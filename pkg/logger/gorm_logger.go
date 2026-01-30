package logger

import (
	"context"
	"errors"
	"fmt"
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
		With(ctx, "gorm_info", formatMessage(msg, data)).Info("GORM info")
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		With(ctx, "gorm_warn", formatMessage(msg, data)).Warn("GORM warning")
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		With(ctx, "gorm_error", formatMessage(msg, data)).Error("GORM error")
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
		With(ctx,
			"error", err.Error(),
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		).Error("GORM query error")

	case elapsed > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= gormlogger.Warn:
		With(ctx,
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"threshold_ms", float64(l.slowThreshold.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		).Warn("GORM slow query")

	case l.logLevel >= gormlogger.Info:
		With(ctx,
			"duration_ms", float64(elapsed.Nanoseconds())/1e6,
			"rows", rows,
			"sql", sql,
		).Info("GORM query")
	}
}

func formatMessage(msg string, data []interface{}) string {
	if len(data) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, data...)
}
