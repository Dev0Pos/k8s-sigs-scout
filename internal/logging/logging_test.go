package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s-scout/internal/logging"
)

func TestParseLevel(t *testing.T) {
	if logging.ParseLevel("debug") != slog.LevelDebug {
		t.Fatal("debug")
	}
	if logging.ParseLevel("") != slog.LevelInfo {
		t.Fatal("default")
	}
	if logging.ParseLevel("WARN") != slog.LevelWarn {
		t.Fatal("warn")
	}
	if logging.ParseLevel("warning") != slog.LevelWarn {
		t.Fatal("warning")
	}
	if logging.ParseLevel("error") != slog.LevelError {
		t.Fatal("error")
	}
}

func TestNewJSON(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(&buf, logging.Options{Format: "json", Level: "info"})
	log.Info("hello", "k", 1)
	var row map[string]any
	if err := json.Unmarshal(buf.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["msg"] != "hello" {
		t.Fatalf("row = %#v", row)
	}
}

func TestNewText(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(&buf, logging.Options{Format: "text", Level: "info"})
	log.Info("hello")
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Fatalf("got %s", out)
	}
	if strings.Contains(strings.TrimSpace(out), `"msg"`) {
		t.Fatal("expected text handler, got json")
	}
}

func TestAccessLogSkipsHealthz(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.New(&buf, logging.Options{Format: "json", Level: "info"}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := logging.AccessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if buf.Len() != 0 {
		t.Fatalf("expected no access log for /healthz, got %s", buf.String())
	}

	buf.Reset()
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?q=helm", nil))
	if !strings.Contains(buf.String(), `"path":"/"`) {
		t.Fatalf("access log = %s", buf.String())
	}
	if strings.Contains(buf.String(), "Authorization") || strings.Contains(buf.String(), "token") {
		t.Fatal("must not log auth material")
	}
}

func TestAccessLogOmitsAuthorizationHeader(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.New(&buf, logging.Options{Format: "json", Level: "info"}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := logging.AccessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/search?q=helm", nil)
	req.Header.Set("Authorization", "Bearer ghs_should_not_appear")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := buf.String()
	if strings.Contains(out, "ghs_should_not_appear") || strings.Contains(out, "Authorization") {
		t.Fatalf("auth material in access log: %s", out)
	}
	if !strings.Contains(out, `"status":404`) {
		t.Fatalf("expected status 404 in access log: %s", out)
	}
	if !strings.Contains(out, `"path":"/search"`) {
		t.Fatalf("expected path in access log: %s", out)
	}
}
