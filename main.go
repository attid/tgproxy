package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddr      = ":8080"
	defaultUpstreamBaseURL = "https://api.telegram.org"
	defaultHealthcheckURL  = "http://127.0.0.1:8080/healthz"
)

type Config struct {
	ListenAddr      string
	AllowedBotIDs   map[string]struct{}
	UpstreamBaseURL string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

func loadConfigFromEnv() (Config, error) {
	allowedBotIDs := parseAllowedBotIDs(os.Getenv("ALLOWED_BOT_IDS"))
	if len(allowedBotIDs) == 0 {
		return Config{}, errors.New("ALLOWED_BOT_IDS must not be empty")
	}

	listenAddr := strings.TrimSpace(os.Getenv("LISTEN_ADDR"))
	if listenAddr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port != "" {
			listenAddr = ":" + port
		} else {
			listenAddr = defaultListenAddr
		}
	}

	upstreamBaseURL := strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL"))
	if upstreamBaseURL == "" {
		upstreamBaseURL = defaultUpstreamBaseURL
	}

	return Config{
		ListenAddr:      listenAddr,
		AllowedBotIDs:   allowedBotIDs,
		UpstreamBaseURL: upstreamBaseURL,
	}, nil
}

func parseAllowedBotIDs(raw string) map[string]struct{} {
	allowedBotIDs := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		botID := strings.TrimSpace(part)
		if botID == "" {
			continue
		}
		allowedBotIDs[botID] = struct{}{}
	}
	return allowedBotIDs
}

func newServer(cfg Config) http.Handler {
	upstreamURL, err := url.Parse(cfg.UpstreamBaseURL)
	if err != nil {
		panic("invalid upstream base URL: " + err.Error())
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstreamURL)
			pr.Out.URL.Path = pr.In.URL.Path
			pr.Out.URL.RawPath = pr.In.URL.RawPath
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, "bad gateway: "+err.Error(), http.StatusBadGateway)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		redactedPath := redactBotPath(r.URL.Path)

		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			log.Printf("request health status=200 method=%s path=%s duration=%s", r.Method, redactedPath, time.Since(start).Round(time.Millisecond))
			return
		}

		botID, ok := extractBotID(r.URL.Path)
		if !ok {
			http.Error(w, "invalid bot path", http.StatusBadRequest)
			log.Printf("request rejected status=400 method=%s path=%s reason=invalid_bot_path duration=%s", r.Method, redactedPath, time.Since(start).Round(time.Millisecond))
			return
		}

		if _, allowed := cfg.AllowedBotIDs[botID]; !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			log.Printf("request rejected status=403 method=%s bot_id=%s path=%s reason=bot_id_not_allowed duration=%s", r.Method, botID, redactedPath, time.Since(start).Round(time.Millisecond))
			return
		}

		recorder := &statusRecorder{ResponseWriter: w}
		proxy.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		log.Printf("request proxied status=%d method=%s bot_id=%s path=%s duration=%s", status, r.Method, botID, redactedPath, time.Since(start).Round(time.Millisecond))
	})
}

func extractBotID(path string) (string, bool) {
	if !strings.HasPrefix(path, "/bot") {
		return "", false
	}

	rest := strings.TrimPrefix(path, "/bot")
	token, methodPath, ok := strings.Cut(rest, "/")
	if !ok || token == "" || methodPath == "" {
		return "", false
	}

	botID, _, ok := strings.Cut(token, ":")
	if !ok || botID == "" {
		return "", false
	}

	for _, r := range botID {
		if r < '0' || r > '9' {
			return "", false
		}
	}

	return botID, true
}

func redactBotPath(path string) string {
	if !strings.HasPrefix(path, "/bot") {
		return path
	}

	rest := strings.TrimPrefix(path, "/bot")
	token, suffix, ok := strings.Cut(rest, "/")
	if !ok || token == "" {
		return "/bot<redacted>"
	}

	botID, secret, hasSecret := strings.Cut(token, ":")
	if hasSecret && botID != "" && secret != "" {
		return "/bot" + botID + ":<redacted>/" + suffix
	}

	return "/bot<redacted>/" + suffix
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(healthcheckURLFromEnv()); err != nil {
			log.Fatal(err)
		}
		return
	}

	cfg, err := loadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newServer(cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s", cfg.ListenAddr)
	log.Fatal(server.ListenAndServe())
}

func healthcheckURLFromEnv() string {
	raw := strings.TrimSpace(os.Getenv("HEALTHCHECK_URL"))
	if raw == "" {
		return defaultHealthcheckURL
	}
	return raw
}

func runHealthcheck(target string) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("healthcheck returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}
