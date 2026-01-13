package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

// OCMLoggerBridge bridges slog to OCM SDK's Logger interface
type OCMLoggerBridge struct{}

// NewOCMLoggerBridge creates an OCM SDK logger bridge
func NewOCMLoggerBridge() sdk.Logger {
	return &OCMLoggerBridge{}
}

// Debug implements sdk.Logger
func (b *OCMLoggerBridge) Debug(ctx context.Context, format string, args ...interface{}) {
	GetLogger().DebugContext(ctx, fmt.Sprintf(format, args...))
}

// Info implements sdk.Logger
func (b *OCMLoggerBridge) Info(ctx context.Context, format string, args ...interface{}) {
	GetLogger().InfoContext(ctx, fmt.Sprintf(format, args...))
}

// Warn implements sdk.Logger
func (b *OCMLoggerBridge) Warn(ctx context.Context, format string, args ...interface{}) {
	GetLogger().WarnContext(ctx, fmt.Sprintf(format, args...))
}

// Error implements sdk.Logger
func (b *OCMLoggerBridge) Error(ctx context.Context, format string, args ...interface{}) {
	GetLogger().ErrorContext(ctx, fmt.Sprintf(format, args...))
}

// Fatal implements sdk.Logger
func (b *OCMLoggerBridge) Fatal(ctx context.Context, format string, args ...interface{}) {
	GetLogger().ErrorContext(ctx, "FATAL: "+fmt.Sprintf(format, args...))
	os.Exit(1)
}

// DebugEnabled implements sdk.Logger
func (b *OCMLoggerBridge) DebugEnabled() bool {
	return GetLogger().Enabled(context.Background(), slog.LevelDebug)
}

// InfoEnabled implements sdk.Logger
func (b *OCMLoggerBridge) InfoEnabled() bool {
	return GetLogger().Enabled(context.Background(), slog.LevelInfo)
}

// WarnEnabled implements sdk.Logger
func (b *OCMLoggerBridge) WarnEnabled() bool {
	return GetLogger().Enabled(context.Background(), slog.LevelWarn)
}

// ErrorEnabled implements sdk.Logger
func (b *OCMLoggerBridge) ErrorEnabled() bool {
	return GetLogger().Enabled(context.Background(), slog.LevelError)
}
