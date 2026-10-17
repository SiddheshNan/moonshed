// Package logger formats the platform log for people: one line per record, colored on
// the console and plain in the log file.
//
//	[2026-10-18 10:32:01.123] INFO  proxy: certificate obtained host=app.example.com
package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var levels = map[slog.Level]struct{ name, color string }{
	slog.LevelDebug: {"DEBUG", "\x1b[34m"},
	slog.LevelInfo:  {"INFO ", "\x1b[32m"},
	slog.LevelWarn:  {"WARN ", "\x1b[33m"},
	slog.LevelError: {"ERROR", "\x1b[31m"},
}

// ParseLevel accepts debug, info, warn and error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q (debug, info, warn or error)", s)
}

// Init makes this the default slog handler. file gets the same lines without color or
// timestamp; the platform log stream adds its own.
func Init(level slog.Level, file io.Writer) {
	out := &output{console: os.Stderr, color: isTerminal(os.Stderr), file: file}
	slog.SetDefault(slog.New(&handler{out: out, level: level}))
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Std is a standard library logger for http.Server.ErrorLog and similar, logging at warn.
func Std(component string) *log.Logger {
	return slog.NewLogLogger(slog.Default().Handler().WithAttrs([]slog.Attr{slog.String("component", component)}), slog.LevelWarn)
}

type output struct {
	mu      sync.Mutex
	console io.Writer
	color   bool
	file    io.Writer
}

type handler struct {
	out       *output
	level     slog.Level
	component string
	attrs     string
	group     string
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	component := h.component
	var attrs strings.Builder
	attrs.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" && h.group == "" {
			component = a.Value.String()
		} else {
			writeAttr(&attrs, h.group, a)
		}
		return true
	})

	lv := levels[nearest(r.Level)]
	line := r.Message
	if component != "" {
		line = component + ": " + line
	}
	ts := r.Time.Format("2006-01-02 15:04:05.000")

	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	if h.out.color {
		fmt.Fprintf(h.out.console, "\x1b[90m[%s]\x1b[0m %s%s\x1b[0m %s\x1b[90m%s\x1b[0m\n", ts, lv.color, lv.name, line, attrs.String())
	} else {
		fmt.Fprintf(h.out.console, "[%s] %s %s%s\n", ts, lv.name, line, attrs.String())
	}
	if h.out.file != nil {
		fmt.Fprintf(h.out.file, "%s %s%s\n", lv.name, line, attrs.String())
	}
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	var b strings.Builder
	b.WriteString(h.attrs)
	for _, a := range attrs {
		if a.Key == "component" && h.group == "" {
			c.component = a.Value.String()
			continue
		}
		writeAttr(&b, h.group, a)
	}
	c.attrs = b.String()
	return &c
}

func (h *handler) WithGroup(name string) slog.Handler {
	c := *h
	c.group = h.group + name + "."
	return &c
}

func nearest(l slog.Level) slog.Level {
	switch {
	case l >= slog.LevelError:
		return slog.LevelError
	case l >= slog.LevelWarn:
		return slog.LevelWarn
	case l >= slog.LevelInfo:
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

func writeAttr(b *strings.Builder, group string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			writeAttr(b, group+a.Key+".", g)
		}
		return
	}
	b.WriteByte(' ')
	b.WriteString(group + a.Key)
	b.WriteByte('=')
	b.WriteString(quote(value(a.Value)))
}

func value(v slog.Value) string {
	switch v.Kind() {
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', -1, 64)
	}
	return v.String()
}

func quote(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\"=") {
		return strconv.Quote(s)
	}
	return s
}
