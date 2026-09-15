package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

type GormLogger struct {
	logLevel      gormlogger.LogLevel
	slowThreshold time.Duration
}

func NewGormLogger(logLevel gormlogger.LogLevel, slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		logLevel:      logLevel,
		slowThreshold: slowThreshold,
	}
}

func (l *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return &GormLogger{
		logLevel:      level,
		slowThreshold: l.slowThreshold,
	}
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Info {
		slog.InfoContext(ctx, "GORM info", "gorm_info", formatMessage(msg, data))
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		slog.WarnContext(ctx, "GORM warning", "gorm_warn", formatMessage(msg, data))
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		slog.ErrorContext(ctx, "GORM error", "gorm_error", formatMessage(msg, data))
	}
}

func (l *GormLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && l.logLevel >= gormlogger.Error && !errors.Is(err, gormlogger.ErrRecordNotFound):
		slog.ErrorContext(ctx, "GORM query error",
			"error", err,
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		)

	case elapsed > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= gormlogger.Warn:
		slog.WarnContext(ctx, "GORM slow query",
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"threshold_ms", float64(l.slowThreshold.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		)

	case l.logLevel >= gormlogger.Info:
		slog.InfoContext(ctx, "GORM query",
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		)
	}
}

func formatMessage(msg string, data []interface{}) string {
	if len(data) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, data...)
}
