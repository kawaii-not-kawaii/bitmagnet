package logging

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestLogLevel_ChangeableAtRuntime is the log live-apply guarantee: applying a
// new logging Config updates the primary core's shared zap.AtomicLevel without
// a restart.
func TestLogLevel_ChangeableAtRuntime(t *testing.T) {
	t.Parallel()

	res := New(Params{Config: Config{Level: "info"}})
	applier := NewLiveApplier(res.Level)

	core := res.Logger.Core()

	if core.Enabled(zapcore.DebugLevel) {
		t.Fatal("debug should be suppressed at level info")
	}

	if !core.Enabled(zapcore.InfoLevel) {
		t.Fatal("info should be enabled at level info")
	}

	// Raise verbosity at runtime: debug becomes visible immediately.
	if _, err := applier.Apply(Config{Level: "debug"}); err != nil {
		t.Fatalf("apply debug level: %v", err)
	}

	if !core.Enabled(zapcore.DebugLevel) {
		t.Error("debug should be emitted after SetLevel(debug), without restart")
	}

	// Lower it: info and debug are suppressed immediately.
	if _, err := applier.Apply(Config{Level: "warn"}); err != nil {
		t.Fatalf("apply warn level: %v", err)
	}

	if core.Enabled(zapcore.InfoLevel) || core.Enabled(zapcore.DebugLevel) {
		t.Error("info/debug should be suppressed after SetLevel(warn)")
	}

	if !core.Enabled(zapcore.WarnLevel) {
		t.Error("warn should remain enabled")
	}
}

// TestLogLevel_FileRotatorKeepsOwnLevel: the secondary (file) core's level is
// fixed and independent — changing the primary atomic level must not affect it.
// The tee'd core reports Enabled as the OR of its children, so with the file
// core at debug, the combined core stays debug-enabled even when the primary
// level is warn.
func TestLogLevel_FileRotatorKeepsOwnLevel(t *testing.T) {
	t.Parallel()

	res := New(Params{Config: Config{
		Level: "warn",
		FileRotator: FileRotatorConfig{
			Enabled: true,
			Level:   "debug",
			Path:    t.TempDir(),
		},
	}})

	core := res.Logger.Core()

	if !core.Enabled(zapcore.DebugLevel) {
		t.Error("file core at debug should keep debug enabled on the tee")
	}

	// Lowering the primary further must not disable the file core's debug.
	res.Level.SetLevel(zapcore.ErrorLevel)

	if !core.Enabled(zapcore.DebugLevel) {
		t.Error("file core level must be independent of the primary atomic level")
	}
}
