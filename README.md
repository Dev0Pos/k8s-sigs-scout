# k8s-sigs-scout

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

## CI (GitHub Actions)

On push/PR: tests, build, `golangci-lint`, Docker + Trivy.

On `v*` tags: build/push to GHCR (`ghcr.io/dev0pos/k8s-sigs-scout`).

## Docker

```bash
docker build -t k8s-scout .
docker run --rm -p 8080:8080 k8s-scout
```

Or from GHCR:

```bash
docker run --rm -p 8080:8080 ghcr.io/dev0pos/k8s-sigs-scout:latest
```

## How it works

1. Background refresh pulls open, unassigned `good first issue` items (paginated) into RAM every 15 minutes — browsers never hit GitHub directly.
2. UI is Go `html/template` + Tailwind CDN + HTMX with deep links: `q`, `lang`, `repo`, `sort`, `page` (10 issues per page).
3. `/healthz` exposes cache status for probes.
