# k8s-sigs-scout

Dashboard for browsing unassigned **good first issue** tasks from the GitHub org [`kubernetes-sigs`](https://github.com/kubernetes-sigs).

## Requirements

- Go **1.26+** (project pins `toolchain go1.26.5`; with `GOTOOLCHAIN=auto` the Go command downloads it if needed)

## Run locally

```bash
cd k8s-sigs-scout
go test ./...
go run .
```

Open http://localhost:8080

Optional port override:

```bash
PORT=3000 go run .
```

Health check: http://localhost:8080/healthz

## CI (GitHub Actions)

On push/PR to `main`/`master`, `.github/workflows/ci.yml` runs:

1. `go test ./...` + `go build`
2. `docker build` + Trivy (exit on any vuln)

On version tags (`v*`), `.github/workflows/release.yml` builds and pushes to GHCR:

`ghcr.io/dev0pos/k8s-sigs-scout:<tag>` and `:latest`

## Docker

```bash
cd k8s-sigs-scout
docker build -t k8s-scout .
docker run --rm -p 8080:8080 k8s-scout
```

Or from GHCR (after a tagged release):

```bash
docker run --rm -p 8080:8080 ghcr.io/dev0pos/k8s-sigs-scout:latest
```

## How it works

1. On startup a background goroutine fetches open, unassigned `good first issue` items from the GitHub Search API (anonymous, no PAT), **paginating** through result pages into the in-memory cache.
2. The cache refreshes every **15 minutes** (`time.Ticker`), so browser traffic never hits GitHub directly and stays within the 60 req/h anonymous limit.
3. `/` renders the dark UI (Go `html/template` + Tailwind CDN + HTMX). Deep links support `q`, `lang`, `repo`, and `sort` — e.g. `/?repo=kubernetes-sigs/kind&sort=comments`.
4. Filtering/sorting always runs against the RAM cache and swaps `#results` without a full page reload.
5. `/healthz` reports cache status for probes.
