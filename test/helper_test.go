package test

import (
	"log/slog"
	"testing"
)

func TestInitTestLoggerHonorsLogLevel(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	t.Setenv("LOGLEVEL", "DEBUG")

	initTestLogger()

	if !slog.Default().Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("debug logging should be enabled when LOGLEVEL=DEBUG")
	}
}
