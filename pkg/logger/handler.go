package logger

import (
	"context"
	"io"
	"log/slog"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
)

// Component is the logging component identity for HyperFleet API.
const Component = "api"

// HandlerConfig configures the API-owned extensions to the shared handler.
// An empty Hostname uses hyperfleet-logger's OS hostname discovery.
type HandlerConfig struct {
	Output   io.Writer
	Hostname string
	Level    slog.Level
	Format   hfl.Format
}

// NewHandler builds a shared HyperFleet logging handler with API correlation
// fields and the API stack-trace policy.
func NewHandler(version string, cfg HandlerConfig) slog.Handler {
	opts := []hfl.Option{
		hfl.WithLevel(cfg.Level),
		hfl.WithFormat(cfg.Format),
		hfl.WithOutput(cfg.Output),
		hfl.WithContextFields(ContextFields()...),
		hfl.WithSanitize(),
	}
	// Preserve the previous handlers: JSON captures every ERROR stack;
	// text does not automatically capture stacks.
	if cfg.Format == hfl.FormatJSON {
		opts = append(opts, hfl.WithStackTrace(func(context.Context, slog.Record) bool { return true }))
	}
	if cfg.Hostname != "" {
		opts = append(opts, hfl.WithHostname(cfg.Hostname))
	}
	return hfl.NewHandler(Component, version, opts...)
}

// NewLogger returns an isolated logger. Callers that require process-wide
// logging should install it with slog.SetDefault in their composition root.
func NewLogger(version string, cfg HandlerConfig) *slog.Logger {
	return slog.New(NewHandler(version, cfg))
}
