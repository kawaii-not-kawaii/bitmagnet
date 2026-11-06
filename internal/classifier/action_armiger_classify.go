package classifier

import (
	"math"
	"strconv"
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/armiger"
	"github.com/bitmagnet-io/bitmagnet/internal/classifier/classification"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

const armigerClassifyActionName = "armiger_classify"

type armigerClassifyAction struct{}

func (armigerClassifyAction) name() string {
	return armigerClassifyActionName
}

var armigerClassifyPayloadSpec = payloadLiteral[string]{
	literal: armigerClassifyActionName,
	description: "Record an armiger classifier prediction as tags (shadow mode: " +
		"never changes content type or any other attribute, never fails)",
}

// armigerTags maps a prediction onto its two tags: armiger-<category> and
// armiger-conf-<bucket>, where bucket is floor(confidence*10)*10 clamped to 0-90.
func armigerTags(category string, confidence float64) ([]string, error) {
	if math.IsNaN(confidence) {
		confidence = 0
	}

	bucket := int(math.Floor(max(0, min(confidence, 1))*10)) * 10
	bucket = max(0, min(bucket, 90))

	tags := []string{
		"armiger-" + strings.ReplaceAll(category, "_", "-"),
		"armiger-conf-" + strconv.Itoa(bucket),
	}

	for _, tag := range tags {
		if err := model.ValidateTagName(tag); err != nil {
			return nil, err
		}
	}

	return tags, nil
}

func (armigerClassifyAction) compileAction(ctx compilerContext) (action, error) {
	if _, err := armigerClassifyPayloadSpec.Unmarshal(ctx); err != nil {
		return action{}, ctx.error(err)
	}

	return action{
		run: func(ctx executionContext) (classification.Result, error) {
			cl := ctx.result

			// Only unhinted torrents, and only when armiger is configured.
			if cl.ContentType.Valid || ctx.armiger == nil || !ctx.armiger.Enabled() {
				return cl, nil
			}

			var files []armiger.File
			if len(ctx.torrent.Files) > 0 {
				files = make([]armiger.File, len(ctx.torrent.Files))
				for i, f := range ctx.torrent.Files {
					files[i] = armiger.File{Path: f.Path, Size: f.Size}
				}
			}

			category, confidence, err := ctx.armiger.Classify(
				ctx,
				ctx.torrent.Name,
				ctx.torrent.Size,
				files,
			)
			if err != nil {
				ctx.logger.Warnw("armiger classification failed", "error", err)
				return cl, nil
			}

			tags, err := armigerTags(category, confidence)
			if err != nil {
				ctx.logger.Warnw("armiger classification produced an invalid tag", "error", err)
				return cl, nil
			}

			// Copy the tag action so the caller's result is never mutated.
			updated := classification.NewTagAction()
			if cl.Tags != nil {
				for tag := range cl.Tags.Add {
					updated.Add[tag] = struct{}{}
				}

				for tag := range cl.Tags.Delete {
					updated.Delete[tag] = struct{}{}
				}
			}

			for _, tag := range tags {
				updated.Add[tag] = struct{}{}
			}

			cl.Tags = updated

			return cl, nil
		},
	}, nil
}

func (armigerClassifyAction) JSONSchema() JSONSchema {
	return armigerClassifyPayloadSpec.JSONSchema()
}
