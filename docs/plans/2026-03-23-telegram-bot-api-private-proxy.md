# Telegram Bot API Private Proxy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a minimal Go HTTP proxy for Telegram Bot API that only forwards requests for allowlisted bot IDs and exposes a local health endpoint.

**Architecture:** Use the Go standard library only. A single `http.Handler` validates `/bot<token>/<method>` paths, extracts the numeric bot ID prefix, rejects non-allowlisted IDs, and proxies the request to the configured upstream with streaming request and response bodies. Configuration is loaded from environment variables and the binary runs in a minimal multi-stage Docker image.

**Tech Stack:** Go 1.22, `net/http`, `httputil.ReverseProxy`, Docker multi-stage build

---

### Task 1: Confirm Red Phase From Existing Tests

**Files:**
- Test: `proxy_test.go`

**Step 1: Run the current test suite**

Run: `go test ./...`
Expected: FAIL because `newServer`, `Config`, and `loadConfigFromEnv` are not implemented yet.

### Task 2: Implement Config Loading

**Files:**
- Create: `main.go`
- Modify: `proxy_test.go`

**Step 1: Make `loadConfigFromEnv` satisfy the current config tests**

Implementation notes:
- `ALLOWED_BOT_IDS` is required and split on commas with whitespace trimmed.
- `LISTEN_ADDR` overrides `PORT`.
- `PORT` maps to `:<port>`.
- Default listen address is `:8080`.
- Default upstream base URL is `https://api.telegram.org`.

**Step 2: Run config-focused tests**

Run: `go test ./... -run 'TestConfig'`
Expected: PASS

### Task 3: Implement HTTP Handler

**Files:**
- Create: `main.go`
- Test: `proxy_test.go`

**Step 1: Implement `/healthz`**

Behavior:
- `GET /healthz` returns `200 OK` with body `ok`.

**Step 2: Implement bot path validation**

Behavior:
- Accept only paths matching `/bot<token>/<method>`.
- Extract bot ID from the token prefix before `:`.
- Reject malformed tokens with `400`.
- Reject non-allowlisted IDs with `403`.

**Step 3: Implement transparent reverse proxy behavior**

Behavior:
- Preserve method, query string, status code, and body.
- Stream request and response bodies without manual full buffering.
- Forward headers except hop-by-hop ones.

**Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS

### Task 4: Add Containerization

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Step 1: Add a multi-stage Docker build**

Behavior:
- Build a static Linux binary in a Go builder stage.
- Copy the binary into a minimal final image.
- Run on port `8080`.
- Use a non-root runtime user.

**Step 2: Build the image**

Run: `docker build -t tgproxy .`
Expected: successful image build

### Task 5: Final Verification

**Files:**
- Verify: `main.go`
- Verify: `Dockerfile`
- Verify: `.dockerignore`
- Verify: `proxy_test.go`

**Step 1: Re-run tests**

Run: `go test ./...`
Expected: PASS

**Step 2: Rebuild container**

Run: `docker build -t tgproxy .`
Expected: PASS
