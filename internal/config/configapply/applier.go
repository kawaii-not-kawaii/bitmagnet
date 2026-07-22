package configapply

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"go.uber.org/fx"
)

var (
	ErrSectionNotMutable = errors.New("section is not mutable")
	ErrUnknownSection    = errors.New("unknown section")
)

type Outcome struct {
	Key   string
	Value any
	Live  bool
}

type Applier struct {
	mutex    sync.Mutex
	resolved *concurrency.AtomicValue[config.ResolvedConfig]
	validate *validator.Validate
	live     map[string]LiveApplier
	path     configwrite.TargetPath
}

type Params struct {
	fx.In

	Appliers []LiveApplier `group:"config_live_appliers"`
	Resolved *concurrency.AtomicValue[config.ResolvedConfig]
	Validate *validator.Validate
	Path     configwrite.TargetPath
}

type Result struct {
	fx.Out

	Applier       *Applier
	Changeability Changeability
}

func New(p Params) Result {
	live := make(map[string]LiveApplier, len(p.Appliers))
	for _, applier := range p.Appliers {
		live[applier.Key] = applier
	}

	applier := &Applier{
		resolved: p.Resolved,
		validate: p.Validate,
		live:     live,
		path:     p.Path,
	}

	return Result{Applier: applier, Changeability: applier}
}

var Module = fx.Provide(New)

func (a *Applier) IsLive(sectionKey string) bool {
	_, ok := a.live[sectionKey]
	return ok
}

func (a *Applier) SetSection(key string, raw any) (Outcome, error) {
	if isDeniedSection(key) {
		return Outcome{}, fmt.Errorf("configapply: section %q: %w", key, ErrSectionNotMutable)
	}

	node, ok := a.resolved.Get().NodeMap[key]
	if !ok {
		return Outcome{}, fmt.Errorf("configapply: section %q: %w", key, ErrUnknownSection)
	}

	targetType := node.Type
	if targetType == nil {
		targetType = reflect.TypeOf(node.Value)
	}
	if targetType == nil {
		return Outcome{}, fmt.Errorf("configapply: section %q: missing value type", key)
	}

	decodedValue := reflect.New(targetType)
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      decodedValue.Interface(),
		ErrorUnused: true,
		MatchName: func(mapKey, fieldName string) bool {
			return mapKey == fieldName
		},
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("configapply: decode section %q: %w", key, err)
	}
	if err = decoder.Decode(raw); err != nil {
		return Outcome{}, fmt.Errorf("configapply: decode section %q: %w", key, err)
	}

	decoded := decodedValue.Elem().Interface()
	if decodedValue.Elem().Kind() == reflect.Struct {
		if err = a.validate.Struct(decoded); err != nil {
			return Outcome{}, fmt.Errorf("configapply: validate section %q: %w", key, err)
		}
	}

	liveApplier, isLive := a.live[key]
	a.mutex.Lock()

	var after func()
	if isLive {
		after, err = liveApplier.Apply(decoded)
		if err != nil {
			a.mutex.Unlock()
			return Outcome{}, fmt.Errorf("configapply: apply section %q: %w", key, err)
		}
	}

	writeErr := configwrite.WriteSection(string(a.path), []string{key}, encodeSection(decoded))
	if writeErr == nil {
		resolved := a.resolved.Get()
		nodeMap := make(map[string]config.ResolvedNode, len(resolved.NodeMap))
		for nodeKey, resolvedNode := range resolved.NodeMap {
			nodeMap[nodeKey] = resolvedNode
		}

		node = nodeMap[key]
		node.Value = decoded
		node.ValueRaw = raw
		nodeMap[key] = node
		resolved.NodeMap = nodeMap
		a.resolved.Set(resolved)
	}

	a.mutex.Unlock()
	if after != nil {
		after()
	}

	// A successful live apply cannot be rolled back if persistence fails. Surface
	// the error while leaving the running subsystem on the validated value.
	if writeErr != nil {
		return Outcome{}, fmt.Errorf("configapply: persist section %q: %w", key, writeErr)
	}

	return Outcome{Key: key, Value: decoded, Live: isLive}, nil
}

func isDeniedSection(key string) bool {
	switch key {
	case "postgres", "auth", "http_server", "dht_server":
		return true
	default:
		return false
	}
}
