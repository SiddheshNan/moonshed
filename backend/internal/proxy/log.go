package proxy

import (
	"context"
	"log/slog"
	"slices"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// logger sends certmagic's zap logs to slog so they land in the platform log.
var logger = zap.New(slogCore{LevelEnabler: zapcore.InfoLevel})

type slogCore struct {
	zapcore.LevelEnabler
	fields []zapcore.Field
}

func (c slogCore) With(fields []zapcore.Field) zapcore.Core {
	c.fields = append(slices.Clone(c.fields), fields...)
	return c
}

func (c slogCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}

func (c slogCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range append(slices.Clone(c.fields), fields...) {
		f.AddTo(enc)
	}
	attrs := []any{"component", "tls"}
	for k, v := range enc.Fields {
		attrs = append(attrs, k, v)
	}
	level := slog.LevelInfo
	switch {
	case e.Level >= zapcore.ErrorLevel:
		level = slog.LevelError
	case e.Level == zapcore.WarnLevel:
		level = slog.LevelWarn
	}
	slog.Log(context.Background(), level, e.Message, attrs...)
	return nil
}

func (c slogCore) Sync() error { return nil }
