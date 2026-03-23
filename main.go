package main

import (
	"errors"
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
)

type Config struct {
	ListenAddr      string
	AllowedBotIDs   map[string]struct{}
	UpstreamBaseURL string
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
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		botID, ok := extractBotID(r.URL.Path)
		if !ok {
			http.Error(w, "invalid bot path", http.StatusBadRequest)
			return
		}

		if _, allowed := cfg.AllowedBotIDs[botID]; !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		proxy.ServeHTTP(w, r)
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

func main() {
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
