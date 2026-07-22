package configfx

import (
	"os"
	"strings"

	"github.com/adrg/xdg"
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/config"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configresolver"
	"github.com/bitmagnet-io/bitmagnet/internal/config/configwrite"
	"github.com/go-playground/validator/v10"
	"go.uber.org/fx"
)

// WritePath is the highest-priority writable YAML source used by the default resolver stack.
type WritePath string

const defaultWritePath WritePath = "./config.yml"

func New() fx.Option {
	osEnv := ReadOsEnv()

	//nolint:prealloc
	var options []fx.Option

	var extraConfigFiles []string
	if osEnv[extraFilesKey] != "" {
		extraConfigFiles = strings.Split(osEnv[extraFilesKey], ",")
	}

	for i, file := range extraConfigFiles {
		options = append(options,
			fx.Provide(
				fx.Annotated{
					Group: "config_resolvers",
					Target: func(val *validator.Validate) (configresolver.Resolver, error) {
						return configresolver.NewFromYamlFile(
							file,
							false,
							val,
							configresolver.WithPriority(-i),
						)
					},
				},
			))
	}

	options = append(options,
		fx.Provide(config.New),
		// Also provide the whole resolved config behind an AtomicValue, seeded
		// with the startup snapshot. The settings read query calls Get() at
		// request time and a runtime config mutation calls Set with a rebuilt
		// snapshot on the same instance, so mutations are immediately visible
		// to reads. Readers must not mutate the snapshot's NodeMap in place —
		// writers replace the whole value.
		fx.Provide(func(r config.ResolvedConfig) *concurrency.AtomicValue[config.ResolvedConfig] {
			av := &concurrency.AtomicValue[config.ResolvedConfig]{}
			av.Set(r)

			return av
		}),
		// The single file runtime config mutations persist to. Mirrors the
		// read search order among writable locations: an existing ./config.yml
		// wins, else an existing XDG config file; with neither present,
		// ./config.yml is designated and created on first write.
		fx.Provide(func() configwrite.TargetPath {
			return configwrite.TargetPath(resolveWriteTarget())
		}),
		fx.Provide(fx.Annotated{
			Group: "config_resolvers",
			Target: func() (configresolver.Resolver, error) {
				return configresolver.NewEnv(
					osEnv,
					configresolver.WithPriority(-len(extraConfigFiles)),
				), nil
			},
		}),
		fx.Provide(
			fx.Annotated{
				Group: "config_resolvers",
				Target: func(val *validator.Validate) (configresolver.Resolver, error) {
					return configresolver.NewFromYamlFile(
						string(defaultWritePath),
						true,
						val,
						configresolver.WithPriority(10),
					)
				},
			},
		),
	)
	if configFilePath, err := xdg.ConfigFile("bitmagnet/config.yml"); err == nil {
		options = append(options,
			fx.Provide(
				fx.Annotated{
					Group: "config_resolvers",
					Target: func(val *validator.Validate) (configresolver.Resolver, error) {
						return configresolver.NewFromYamlFile(
							configFilePath,
							true,
							val,
							configresolver.WithPriority(20),
						)
					},
				},
			),
		)
	}

	return fx.Module(
		"config",
		fx.Options(options...),
	)
}

// resolveWriteTarget picks the file runtime config mutations persist to. An
// existing ./config.yml (the highest-priority file location the resolver
// reads) wins; else an existing XDG config file; else ./config.yml, which
// does not exist yet and is created on first write.
func resolveWriteTarget() string {
	const localPath = "./config.yml"
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	if path, err := xdg.SearchConfigFile("bitmagnet/config.yml"); err == nil {
		return path
	}

	return localPath
}

func ReadOsEnv() map[string]string {
	rawEnv := os.Environ()
	env := make(map[string]string, len(rawEnv))

	for _, rawEnvEntry := range rawEnv {
		parts := strings.SplitN(rawEnvEntry, "=", 2)
		env[parts[0]] = parts[1]
	}

	return env
}

const extraFilesKey = "EXTRA_CONFIG_FILES"
