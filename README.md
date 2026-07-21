# k8s-sigs-scout

Dashboard for browsing unassigned **good first issue** tasks from the GitHub org [`kubernetes-sigs`](https://github.com/kubernetes-sigs).

## Requirements

- Go **1.26+** (project pins `toolchain go1.26.5`; with `GOTOOLCHAIN=auto` the Go command downloads it if needed)

## Run locally

```bash
cd k8s-sigs-scout
go test ./...
go run main.go
```

Open http://localhost:8080

Optional port override:

```bash
PORT=3000 go run main.go
```

## CI (GitHub Actions)

On push/PR to `main`/`master`, `.github/workflows/ci.yml` runs:

1. `go test ./...` + `go build`
2. `docker build` + Trivy (exit on any vuln)

## Docker

```bash
cd k8s-sigs-scout
docker build -t k8s-scout .
docker run --rm -p 8080:8080 k8s-scout
```

Port override:

```bash
docker run --rm -p 3000:3000 -e PORT=3000 k8s-scout
```

## How it works

1. On startup a background goroutine fetches open, unassigned `good first issue` items from the GitHub Search API (anonymous, no PAT).
2. Results are stored in an in-memory cache and refreshed every **15 minutes** (`time.Ticker`), so browser traffic never hits GitHub directly and stays within the 60 req/h anonymous limit.
3. `/` renders the dark UI (Go `html/template` + Tailwind CDN + HTMX).
4. Typing in search or changing the language/tag filter fires `hx-get="/search"` with a 200 ms debounce; the server filters the RAM cache and swaps `#results` without a full page reload.
