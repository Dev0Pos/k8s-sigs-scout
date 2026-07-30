# k8s-sigs-scout

[![CI](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/ci.yml/badge.svg)](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/ci.yml)
[![Release](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/release.yml/badge.svg)](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![GHCR](https://img.shields.io/badge/ghcr.io-k8s--sigs--scout-326ce5)](https://github.com/Dev0Pos/k8s-sigs-scout/pkgs/container/k8s-sigs-scout)

Dashboard for browsing unassigned **good first issue** tasks from the GitHub org [`kubernetes-sigs`](https://github.com/kubernetes-sigs).

## Requirements

- Go **1.26+** (project pins `toolchain go1.26.5`; with `GOTOOLCHAIN=auto` the Go command downloads it if needed)

## Layout

```
cmd/k8s-scout/          # main() entrypoint
internal/
  issue/                # domain model + language hints
  github/               # GitHub Search API client (paginated)
  cache/                # in-memory cache + background refresh
  filter/               # filter / sort / deep-link paths
  logging/              # slog setup + HTTP access log middleware
  server/               # HTTP handlers + embedded templates
```

## Run locally

```bash
go test ./...
go run ./cmd/k8s-scout
```

Open http://localhost:8080 — health: http://localhost:8080/healthz

```bash
PORT=3000 go run ./cmd/k8s-scout
```

Optional `GITHUB_TOKEN` (PAT or fine-grained token) raises Search API rate limits; without it the app stays unauthenticated (~60 req/h per IP). The token is only used by the backend cache refresher — never exposed to the browser or logs.

```bash
GITHUB_TOKEN=ghp_xxx go run ./cmd/k8s-scout
```

Logging uses Go `log/slog` (JSON by default). Tune with:

```bash
LOG_FORMAT=json LOG_LEVEL=info go run ./cmd/k8s-scout   # container-friendly default
LOG_FORMAT=text LOG_LEVEL=debug go run ./cmd/k8s-scout   # local debugging
```

HTTP access logs include method/path/status/duration (not `/healthz`). The token value is never logged.

## CI (GitHub Actions)

On push/PR: tests, build, `golangci-lint`, Docker + Trivy.

On `v*` tags: Trivy gate (**0 vulns**, same as CI) on an amd64 image **before** any GHCR push, then multi-arch (`linux/amd64`, `linux/arm64`) publish to `ghcr.io/dev0pos/k8s-sigs-scout`.

Dependabot opens weekly PRs for GitHub Actions and Docker base images.

## Docker

```bash
docker build -t k8s-scout .
docker run --rm -p 8080:8080 k8s-scout
```

Or from GHCR:

```bash
docker run --rm -p 8080:8080 ghcr.io/dev0pos/k8s-sigs-scout:latest
```

## Docker Compose

```bash
docker compose up --build
```

Uses the local Dockerfile by default and tags the image as `ghcr.io/dev0pos/k8s-sigs-scout:latest`. To run a published image without building:

```bash
docker compose pull
docker compose up
```

Optional port override: `PORT=3000 docker compose up --build`.

Optional GitHub auth (recommended for shared hosts):

```bash
GITHUB_TOKEN=ghp_xxx docker compose up --build
```

## How it works

1. Background refresh pulls open, unassigned `good first issue` items (paginated) into RAM every 15 minutes — browsers never hit GitHub directly.
2. UI is Go `html/template` + Tailwind CDN + HTMX with deep links: `q`, `lang`, `repo`, `sort`, `page` (10 issues per page). Use **Copy URL** to share the current filter.
3. **New since last visit** highlights issues created after your previous session (`localStorage`) and shows a count in the header. Use **Mark seen** to reset.
4. `/healthz` exposes cache status for probes. If a refresh fails but stale data remains, the UI shows a **degraded** warning banner.
