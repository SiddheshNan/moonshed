package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanicRecovery(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.public("GET /boom", func(w http.ResponseWriter, r *http.Request) error { panic("boom") })
	s.public("GET /stream", func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/event-stream")
		if _, ok := w.(http.Flusher); !ok {
			t.Error("handlers must still see an http.Flusher")
		}
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		panic("mid-stream")
	})
	s.public("GET /ok", func(w http.ResponseWriter, r *http.Request) error { return ok(w, "fine") })
	srv := httptest.NewServer(s)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError || body["error"] != "internal server error" {
		t.Errorf("panic before writing: got %d %v", res.StatusCode, body)
	}

	// After a panic mid-stream the client sees the stream cut off, not a clean end.
	res, err = http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(data), "first") || err == nil {
		t.Errorf("panic mid-stream: got %q, err %v; want the first event then a broken connection", data, err)
	}

	// The server keeps working.
	res, err = http.Get(srv.URL + "/ok")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("after panics: %v %v", res, err)
	}
	res.Body.Close()
}
