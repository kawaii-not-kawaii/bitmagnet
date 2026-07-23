package classifier

import (
	"encoding/json"
	"regexp"

	"github.com/bitmagnet-io/bitmagnet/internal/llm"
	"github.com/bitmagnet-io/bitmagnet/internal/llm/llmobs"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/tmdb"
	"go.uber.org/zap"
)

type dependencies struct {
	search     LocalSearch
	tmdbClient tmdb.Client
	// llmProviders returns the providers current at call time (backed by
	// llm.Registry.All), so runtime config updates are observed. May be nil
	// when no registry is wired (e.g. tests without LLM).
	llmProviders func() map[string]llm.Provider
	// llmEnabled reads the registry's current enabled state.
	llmEnabled func() bool
	recorder   *llmobs.Recorder
	_logger    *zap.SugaredLogger
	logger     *zap.SugaredLogger
}

func (dependencies) CleanObj(o interface{}) map[string]any {
	isEmptyString := regexp.MustCompile(`^(?:[0\s]*|0001-01-01T00:00:00Z)$`)

	var m map[string]any

	// Best-effort marshal/unmarshal round-trip to convert the struct into a
	// generic map so we can iterate and strip zero-valued fields. If either
	// step fails, m stays nil and the function returns an empty map — the
	// caller treats nil and empty identically.
	jhint, _ := json.Marshal(o)   //nolint:errchkjson // o is a caller-supplied model struct; failure leaves m nil
	_ = json.Unmarshal(jhint, &m) // cannot fail in practice: input is fresh output of Marshal

	for k, v := range m {
		if s, sOk := v.(string); sOk && isEmptyString.MatchString(s) {
			delete(m, k)
			continue
		}

		if a, aOk := v.([]any); aOk && len(a) == 0 {
			delete(m, k)
			continue
		}

		if f, fOk := v.(float64); fOk && f == 0.0 {
			delete(m, k)
			continue
		}

		d, dOk := v.(model.Date)
		if dOk && (d.Year == 0 || d.Month == 0 || d.Day == 0) {
			delete(m, k)
			continue
		}

		if v == nil {
			delete(m, k)
			continue
		}
	}

	return m
}
