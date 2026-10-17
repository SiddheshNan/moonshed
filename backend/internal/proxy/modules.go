package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"moonshed/internal/requestlog"
)

// current is the Proxy the modules report to. Caddy builds modules itself, so
// they can't be handed one.
var current atomic.Pointer[Proxy]

func init() {
	caddy.RegisterModule(Track{})
	caddy.RegisterModule(Panel{})
	caddy.RegisterModule(LogWriter{})
	httpcaddyfile.RegisterHandlerDirective("moonshed_track", func(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
		h.Next()
		if !h.NextArg() {
			return nil, h.ArgErr()
		}
		return Track{AppID: h.Val()}, nil
	})
	httpcaddyfile.RegisterDirectiveOrder("moonshed_track", httpcaddyfile.Before, "tracing")
	httpcaddyfile.RegisterHandlerDirective("moonshed_panel", func(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
		h.Next()
		return Panel{}, nil
	})
	httpcaddyfile.RegisterDirectiveOrder("moonshed_panel", httpcaddyfile.Before, "reverse_proxy")
}

// Track counts traffic per host and records requests for the app's request log.
type Track struct {
	AppID string `json:"app_id"`
}

func (Track) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "http.handlers.moonshed_track", New: func() caddy.Module { return new(Track) }}
}

func (t Track) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	p := current.Load()
	if p == nil {
		return next.ServeHTTP(w, r)
	}
	start := time.Now()
	rec := caddyhttp.NewResponseRecorder(w, nil, nil)
	err := next.ServeHTTP(rec, r)
	took := time.Since(start)

	status := rec.Status()
	if err != nil {
		// The error handler writes this response after we return.
		var he caddyhttp.HandlerError
		if errors.As(err, &he) && he.StatusCode != 0 {
			status = he.StatusCode
		} else {
			status = http.StatusInternalServerError
		}
	}
	if status == 0 {
		status = http.StatusOK
	}
	host := normalizeHost(r.Host)
	p.count(host, status, took)
	if p.requests.Recording(t.AppID) {
		ip, _ := caddyhttp.GetVar(r.Context(), caddyhttp.ClientIPVarKey).(string)
		if ip == "" {
			ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		p.requests.Record(requestlog.Entry{
			AppID:      t.AppID,
			Time:       start.UnixMilli(),
			Method:     r.Method,
			Host:       host,
			Path:       r.URL.Path,
			Query:      r.URL.RawQuery,
			Status:     status,
			DurationMS: float64(took.Microseconds()) / 1000,
			Bytes:      int64(rec.Size()),
			IP:         ip,
			UserAgent:  r.UserAgent(),
			Referer:    r.Referer(),
		})
	}
	return err
}

// Panel serves the Moonshed panel on its domain.
type Panel struct{}

func (Panel) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "http.handlers.moonshed_panel", New: func() caddy.Module { return new(Panel) }}
}

func (Panel) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	p := current.Load()
	if p == nil {
		return caddyhttp.Error(http.StatusServiceUnavailable, errors.New("panel not ready"))
	}
	p.panel.ServeHTTP(w, r)
	return nil
}

// LogWriter sends Caddy's logs (JSON lines) to the platform log.
type LogWriter struct{}

func (LogWriter) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "caddy.logging.writers.moonshed", New: func() caddy.Module { return new(LogWriter) }}
}

func (LogWriter) String() string    { return "moonshed" }
func (LogWriter) WriterKey() string { return "moonshed" }

func (LogWriter) OpenWriter() (io.WriteCloser, error) { return slogWriter{}, nil }

func (*LogWriter) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next()
	return nil
}

type slogWriter struct{}

func (slogWriter) Close() error { return nil }

func (slogWriter) Write(b []byte) (int, error) {
	for line := range bytes.SplitSeq(bytes.TrimSpace(b), []byte("\n")) {
		var fields map[string]any
		if json.Unmarshal(line, &fields) != nil {
			slog.Info(string(line), "component", "proxy")
			continue
		}
		msg, _ := fields["msg"].(string)
		lvl, _ := fields["level"].(string)
		attrs := []any{"component", "proxy"}
		for k, v := range fields {
			switch k {
			case "msg", "level", "ts":
			case "logger":
				attrs = append(attrs, "caddy", v)
			default:
				attrs = append(attrs, k, v)
			}
		}
		level := slog.LevelInfo
		switch strings.ToLower(lvl) {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error", "panic", "fatal":
			level = slog.LevelError
		}
		slog.Log(context.Background(), level, msg, attrs...)
	}
	return len(b), nil
}

var (
	_ caddyhttp.MiddlewareHandler = Track{}
	_ caddyhttp.MiddlewareHandler = Panel{}
	_ caddy.WriterOpener          = LogWriter{}
	_ caddyfile.Unmarshaler       = (*LogWriter)(nil)
)
