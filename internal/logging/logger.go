package logging

import (
	"context"
	"os"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Params struct {
	fx.In
	Config Config
}

type Result struct {
	fx.Out
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
	// Level is the primary core's level enabler. It is atomic so the log level
	// can be changed at runtime (a future config mutation calls SetLevel on
	// it); a bare zapcore.Level here would fix the threshold for the process
	// lifetime. The file-rotator core keeps its own independent fixed level.
	Level   zap.AtomicLevel
	AppHook fx.Hook `group:"app_hooks"`
}

func New(params Params) Result {
	var appHook fx.Hook

	var encoder zapcore.Encoder
	if params.Config.JSON {
		encoder = zapcore.NewJSONEncoder(jsonEncoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(consoleEncoderConfig)
	}

	writeSyncer := zapcore.AddSync(os.Stdout)

	opts := []zap.Option{
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.AddCaller(),
	}
	if params.Config.Development {
		opts = append(opts, zap.Development())
	}

	level := zap.NewAtomicLevelAt(levelToZapLevel(params.Config.Level))

	core := zapcore.NewCore(
		encoder,
		writeSyncer,
		level,
	)

	if params.Config.FileRotator.Enabled {
		fWriteSyncer := newFileRotator(params.Config.FileRotator)
		core = zapcore.NewTee(
			core,
			zapcore.NewCore(
				zapcore.NewJSONEncoder(jsonEncoderConfig),
				fWriteSyncer,
				levelToZapLevel(params.Config.FileRotator.Level),
			),
		)
		appHook = fx.Hook{
			OnStop: func(context.Context) error {
				return fWriteSyncer.Close()
			},
		}
	}

	l := zap.New(core, opts...)

	return Result{
		Logger:  l,
		Sugar:   l.Sugar(),
		Level:   level,
		AppHook: appHook,
	}
}
