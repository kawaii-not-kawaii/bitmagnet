package client

import (
	"context"
	"fmt"

	"github.com/autobrr/go-qbittorrent"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
)

type qBitClient struct {
	CommonClient
}

func (c qBitClient) sendTo(ctx context.Context, content *content) error {
	sendTo, ok := c.config.GetSendTo(gen.ClientIDQBittorrent)
	if !ok {
		return fmt.Errorf("undefined sendTo: %+v", c.config.SendTo)
	}

	qb := qbittorrent.NewClient(qbittorrent.Config{
		Host:     fmt.Sprintf("http://%v:%v/", sendTo.Host, sendTo.Port),
		Username: sendTo.Username,
		Password: sendTo.Password,
		Timeout:  1,
	})

	err := qb.LoginCtx(ctx)
	if err != nil {
		return err
	}

	pref, err := qb.GetAppPreferencesCtx(ctx)
	if err != nil {
		return err
	}

	for _, item := range *content {
		category := c.downloadCategory(item.Content.Type)

		err = qb.AddTorrentFromUrlCtx(
			ctx,
			item.Torrent.MagnetURI(),
			map[string]string{
				"savepath": fmt.Sprintf("%v/%v", pref.SavePath, category),
				"category": category,
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}
