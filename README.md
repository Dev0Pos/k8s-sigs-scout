# k8s-sigs-scout

[![CI](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/ci.yml/badge.svg)](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/ci.yml)
[![Release](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/release.yml/badge.svg)](https://github.com/Dev0Pos/k8s-sigs-scout/actions/workflows/release.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![GHCR](https://img.shields.io/badge/ghcr.io-k8s--sigs--scout-326ce5)](https://github.com/Dev0Pos/k8s-sigs-scout/pkgs/container/k8s-sigs-scout)

Dashboard for browsing unassigned **good first issue** tasks from the GitHub org [`kubernetes-sigs`](https://github.com/kubernetes-sigs).

## Requirements

- Go **1.26+** (`go.mod` pins `go 1.26` and `toolchain go1.26.5`; with `GOTOOLCHAIN=auto` the Go command downloads it if needed)
- Docker image builds with `golang:1.27-alpine` (Dependabot) against that same module — CI tests use `go-version-file: go.mod`, not the builder tag

## Environment

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | Listen address (`:` + value) in `cmd/k8s-scout` |
| `GITHUB_TOKEN` | unset | Optional PAT or fine-grained token sent as `Authorization: Bearer`. Raises Search API limits (~60 req/h unauthenticated → ~5000/h). Used only by the cache refresher — never exposed to the browser or logs |
| `LOG_FORMAT` | `json` | `text` for slog text; any other value (including empty) is JSON |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` / `warning`, `error` |

`PORT` in Docker Compose only remaps the **host** port. The container process still listens on `8080` (Dockerfile `ENV PORT=8080`; compose does not pass `PORT` into the container).

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

```bash
GITHUB_TOKEN=ghp_xxx go run ./cmd/k8s-scout
```

```bash
LOG_FORMAT=json LOG_LEVEL=info go run ./cmd/k8s-scout   # container-friendly default
LOG_FORMAT=text LOG_LEVEL=debug go run ./cmd/k8s-scout   # local debugging
```

HTTP access logs include `method`, `path`, `query`, `status`, `bytes`, `duration_ms`, `remote` (not `/healthz`). Authorization headers and the token value are never logged.

## HTTP

| Path | Response |
|------|----------|
| `GET /` | Full HTML dashboard |
| `GET /search` | Same handler as `/` |
| `GET /healthz` | JSON cache probe (skipped by the access log) |

Unknown paths under `/` return 404. `HX-Request: true` returns the `results.html` fragment for HTMX swaps.

Query params (shareable deep-link; **Copy URL** copies `window.location.href`):

| Param | Behavior |
|-------|----------|
| `q` | Case-insensitive substring of title + repository + labels |
| `lang` | Exact match on derived **language hints** (below). Unknown values match nothing — no substring fallback on repo/labels |
| `repo` | Exact repository (`owner/name`) |
| `sort` | `newest` (default, omitted from the URL), `comments`, `repo`, `title`. Any other value is treated as `newest` |
| `page` | UI page, **10** issues per page. Out-of-range values clamp. Non-numeric `page` is treated as `1`. `page=1` is omitted from the URL |

Language hints are tokenized from repository name + labels (split on `/`, `-`, `_`, `.`, `:`, and whitespace) and kept only if they match: `go` (`golang` → `go`), `python`, `javascript`, `typescript`, `rust`, `java`, `docs` (`documentation` → `docs`), `helm`, `yaml`. An issue whose tokens are only `good first issue` has **no** hints. `lang` compares those hints only — a label-text fallback would treat `good` as `go` and `javascript` as `java`.

The filter form is HTMX (`hx-get="/"`, `hx-push-url`, 200ms debounce). The HTML loads Tailwind from `cdn.tailwindcss.com` and HTMX **2.0.4** from `unpkg.com`; deep-link query URLs still render on a full page load if those CDNs are blocked.

Example: `/?q=helm&lang=go&repo=kubernetes-sigs%2Fkind&sort=comments&page=2`

### `GET /healthz`

```json
{
  "status": "ok",
  "issues": 142,
  "updated_at": "2026-09-02T11:00:00Z",
  "age_seconds": 90
}
```

| `status` | HTTP | Meaning |
|----------|------|---------|
| `starting` | 200 | No snapshot yet |
| `ok` | 200 | Snapshot present, last refresh succeeded |
| `degraded` | 200 | Snapshot present, last refresh failed (`error` set; UI amber banner) |
| `error` | 503 | No snapshot and last refresh failed (`error` set; UI red banner) |

Kubernetes probes in `deploy/k8s/scout.yaml` are **TCP on 8080**, not this endpoint — a GitHub outage does not restart the pod.

## How it works

1. `cache.StartRefresher` fetches on process start, then every **15 minutes**. Browsers never call GitHub.
2. Fixed Search query: `org:kubernetes-sigs is:issue is:open label:"good first issue" no:assignee`
3. Pagination: 100 items/page, **max 10 pages** (~1000 results — GitHub Search cap). Sorted `created` desc. HTTP client timeout 30s, `User-Agent: k8s-sigs-scout`.
4. A failed refresh keeps the last good snapshot (`degraded`). A first-fetch failure with an empty cache is `error`.
5. Filters and sort run in memory (`internal/filter`). `lang` matches `LanguageHints` only. Repo dropdown options are the distinct repositories in the **full** cache, not the current filter. Sort ties break on `HTMLURL`.
6. **New since last visit** uses `localStorage` key `k8s-scout:lastVisit`. The header count is **the current page only**. **Mark seen**, tab hide (`visibilitychange`), and page unload all write the timestamp. First visit shows `—`.

## Troubleshooting

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| `/healthz` `degraded` / amber banner | GitHub Search failed (often 403 + `rate-limit-remaining=0`) | Set `GITHUB_TOKEN`. Unauthenticated budget is ~60 req/h; a refresh can use up to 10 Search calls |
| `/healthz` 503 `error` | First refresh failed; RAM is empty | Same as above. UI shows a hard error, not stale data |
| Compose `PORT=3000` but the process still listens on 8080 | Compose maps host `PORT` → container `8080` | Open `http://localhost:3000`. To change the listen port, run the binary with `PORT=…` (not compose) |
| `lang=go` looks empty, or an old build matches the whole catalog | Hints come from tokenized repo+labels only; a substring fallback on `"good first issue"` matches `go` | Use a dropdown token. Upgrade past the hint-only filter if every issue matches `lang=go` |
| Unstyled page / changing filters does nothing | Browser cannot load Tailwind/HTMX CDNs | Allow `cdn.tailwindcss.com` and `unpkg.com`. Typed `/?q=…` URLs still work on full page load |
| `docker exec` has no shell after a local rebuild | Current `Dockerfile` is `scratch` | Use process logs and `/healthz`. GHCR through `v0.10.0` is still Alpine |
| "New since visit" is 0 after leaving the tab | Hide/unload writes `lastVisit` | Use **Mark seen** only when you intend to reset |
| CI Go ≠ Docker builder | CI uses `go.mod`; image builder is `golang:1.27-alpine` | Expected until the module is bumped |

## CI (GitHub Actions)

On push/PR: tests, build, `golangci-lint` **v2.1.6** (`install-mode: goinstall`), Docker + Trivy.

On `v*` tags: Trivy gate (**0 vulns**, same as CI) on an amd64 image **before** any GHCR push, then multi-arch (`linux/amd64`, `linux/arm64`) publish to `ghcr.io/dev0pos/k8s-sigs-scout`.

Dependabot opens weekly PRs (Mondays) for GitHub Actions and Docker base images.

## Kubernetes (Loki + Grafana)

See [`deploy/k8s/README.md`](deploy/k8s/README.md) for a self-contained install (scout + Loki + Promtail + Grafana) with NodePorts for Tailscale access.

## Docker

```bash
docker build -t k8s-scout .
docker run --rm -p 8080:8080 k8s-scout
```

Building from this tree produces `FROM scratch`: statically linked binary (`CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`), CA bundle copied from the `golang:1.27-alpine` builder, `USER 65532:65532`, `PORT=8080`. No shell — `docker exec` cannot open a prompt. GHCR tags through **v0.10.0** (including `:latest` until the next release) are still Alpine **3.24** / `USER nobody`. Dependabot only bumps the **builder** tag.

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

Host port override (container still listens on 8080): `PORT=3000 docker compose up --build`.

Optional GitHub auth (recommended for shared hosts):

```bash
GITHUB_TOKEN=ghp_xxx docker compose up --build
```
