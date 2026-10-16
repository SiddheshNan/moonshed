package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"moonshed/internal/manifest"
)

var client = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{DisableKeepAlives: true},
}

// Probe runs one health check against the app's local port and returns its latency.
func Probe(ctx context.Context, port int, h *manifest.Health) (time.Duration, error) {
	if port <= 0 {
		return 0, errors.New("health check needs the app to have a port")
	}
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	start := time.Now()

	if h.HTTP == "" {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return 0, err
		}
		conn.Close()
		return time.Since(start), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+h.HTTP, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("%s returned HTTP %d", h.HTTP, resp.StatusCode)
	}
	return time.Since(start), nil
}
