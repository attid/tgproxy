package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: "http://example.invalid",
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if body := strings.TrimSpace(rec.Body.String()); body != "ok" {
		t.Fatalf("expected body ok, got %q", body)
	}
}

func TestRejectsMalformedBotToken(t *testing.T) {
	t.Parallel()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: "http://example.invalid",
	})

	req := httptest.NewRequest(http.MethodGet, "/botbadtoken/getMe", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRejectsForbiddenBotID(t *testing.T) {
	t.Parallel()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: "http://example.invalid",
	})

	req := httptest.NewRequest(http.MethodGet, "/bot999999:secret/getMe", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestProxiesAllowedBotRequest(t *testing.T) {
	t.Parallel()

	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true

		if r.URL.Path != "/bot123456:secret/getMe" {
			t.Fatalf("unexpected upstream path %q", r.URL.Path)
		}

		if r.URL.RawQuery != "offset=42" {
			t.Fatalf("unexpected upstream query %q", r.URL.RawQuery)
		}

		if r.Header.Get("X-Test-Header") != "value" {
			t.Fatalf("expected X-Test-Header to be forwarded")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: upstream.URL,
	})

	req := httptest.NewRequest(http.MethodGet, "/bot123456:secret/getMe?offset=42", nil)
	req.Header.Set("X-Test-Header", "value")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !upstreamCalled {
		t.Fatal("expected upstream to be called")
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rec.Code)
	}

	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestProxiesJSONBody(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		if string(body) != `{"chat_id":1}` {
			t.Fatalf("unexpected body %q", string(body))
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content type %q", ct)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: upstream.URL,
	})

	req := httptest.NewRequest(http.MethodPost, "/bot123456:secret/sendMessage", strings.NewReader(`{"chat_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestProxiesMultipartBody(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Fatalf("unexpected content type %q", r.Header.Get("Content-Type"))
		}

		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		if r.FormValue("chat_id") != "1" {
			t.Fatalf("unexpected chat_id %q", r.FormValue("chat_id"))
		}

		file, _, err := r.FormFile("document")
		if err != nil {
			t.Fatalf("failed to get form file: %v", err)
		}
		defer file.Close()

		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		if string(body) != "hello world" {
			t.Fatalf("unexpected file contents %q", string(body))
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	handler := newServer(Config{
		AllowedBotIDs:   map[string]struct{}{"123456": {}},
		UpstreamBaseURL: upstream.URL,
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", "1"); err != nil {
		t.Fatalf("failed to write chat_id field: %v", err)
	}

	part, err := writer.CreateFormFile("document", "hello.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}

	if _, err := part.Write([]byte("hello world")); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/bot123456:secret/sendDocument", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("ALLOWED_BOT_IDS", "123456, 987654")
	t.Setenv("PORT", "9090")
	t.Setenv("UPSTREAM_BASE_URL", "https://example.invalid")

	cfg, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.ListenAddr != ":9090" {
		t.Fatalf("expected listen addr :9090, got %q", cfg.ListenAddr)
	}

	if cfg.UpstreamBaseURL != "https://example.invalid" {
		t.Fatalf("expected custom upstream, got %q", cfg.UpstreamBaseURL)
	}

	if _, ok := cfg.AllowedBotIDs["123456"]; !ok {
		t.Fatal("expected 123456 in allowlist")
	}

	if _, ok := cfg.AllowedBotIDs["987654"]; !ok {
		t.Fatal("expected 987654 in allowlist")
	}
}

func TestConfigRejectsMissingAllowedBotIDs(t *testing.T) {
	t.Setenv("ALLOWED_BOT_IDS", "")
	t.Setenv("PORT", "")
	t.Setenv("UPSTREAM_BASE_URL", "")

	if _, err := loadConfigFromEnv(); err == nil {
		t.Fatal("expected error for empty ALLOWED_BOT_IDS")
	}
}
