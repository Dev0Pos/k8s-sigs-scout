# Kubernetes deploy (scout + Loki + Grafana)

Self-contained stack in namespace `k8s-scout` (own Loki/Grafana; does not depend on other cluster monitoring).

| Component | Purpose | Access |
|-----------|---------|--------|
| **k8s-scout** | dashboard (`LOG_FORMAT=json`, image tag **pinned** in `scout.yaml`) | NodePort **30808** |
| **Loki** | log store (filesystem, 5Gi `local-path` PVC, samples older than 168h rejected) | ClusterIP `:3100` |
| **Promtail** | ships **scout** pod logs → Loki (DaemonSet) | hostPath `/var/log/pods` |
| **Grafana** | Explore + preloaded dashboard (`grafana/grafana:11.5.2`) | NodePort **30300** (`admin` / `scout`) |

## Install (on the cluster host)

```bash
cd deploy/k8s
chmod +x install.sh
./install.sh
```

`install.sh` applies manifests, waits for Loki / Grafana / scout rollouts (180s each), then waits for Promtail (`|| true` so a slow node agent does not fail the script). It prints the Tailscale IPv4 if `tailscale ip -4` works, otherwise the first node InternalIP.

Or:

```bash
kubectl apply -f namespace.yaml -f loki.yaml -f promtail.yaml -f grafana.yaml -f scout.yaml
```

### Image pin

`scout.yaml` pins `ghcr.io/dev0pos/k8s-sigs-scout:v0.9.0` (`imagePullPolicy: IfNotPresent`). Newer git tags (for example `v0.10.0`) are **not** picked up until you bump that tag and roll the deployment.

That pin lags two changes already on `main`:

- **Language filter:** `v0.9.0` / `v0.10.0` still substring-match repo + labels, so `lang=go` matches the words in `good first issue`. Current `internal/filter` matches `LanguageHints` only (hub README).
- **Runtime image:** those tags are Alpine **3.24** / `USER nobody`. Current `Dockerfile` is `scratch` (`USER 65532:65532`, no shell). After you bump to a scratch-based tag, use logs and `/healthz` instead of `kubectl exec`.

The dashboard HTML loads Tailwind (`cdn.tailwindcss.com`) and HTMX 2.0.4 (`unpkg.com`). Browsers that cannot reach those CDNs get an unstyled page; filter changes need HTMX, but a full load of a `/?q=…&lang=…` URL still works.

```bash
# after editing the image tag in scout.yaml
kubectl -n k8s-scout apply -f scout.yaml
kubectl -n k8s-scout rollout restart deploy/k8s-scout
```

### GitHub token (optional)

Higher Search API rate limits. Secret is optional in the pod spec (`optional: true`); without it the app stays unauthenticated.

```bash
kubectl -n k8s-scout create secret generic k8s-scout-github \
  --from-literal=GITHUB_TOKEN=ghp_xxx \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n k8s-scout rollout restart deploy/k8s-scout
```

## Probes and resources

Scout liveness/readiness are **TCP on container port 8080**, not `GET /healthz`. The process can stay Ready while GitHub is failing (`degraded` / `error` on `/healthz`). That is intentional: a Search outage must not restart the dashboard.

Requests/limits (from the manifests):

| Workload | CPU | Memory |
|----------|-----|--------|
| k8s-scout | 50m / 500m | 64Mi / 256Mi |
| Loki | 50m / 1 | 128Mi / 1Gi |
| Grafana | 50m / 500m | 128Mi / 512Mi |
| Promtail | 25m / 200m | 64Mi / 256Mi |

## URLs

NodePorts are open on the host (`30808` / `30300`), but Tailscale ACLs may block them from your laptop. Easiest access via SSH tunnels:

```bash
ssh -N \
  -L 30808:127.0.0.1:30808 \
  -L 30300:127.0.0.1:30300 \
  root@100.76.137.100
```

Then open:

- Scout: http://127.0.0.1:30808
- Grafana: http://127.0.0.1:30300  (`admin` / `scout`; sign-up disabled)

On the cluster host itself (or LAN): http://192.168.68.103:30808 and `:30300`.

In Grafana → **Explore** → Loki query:

```logql
{namespace="k8s-scout", app="k8s-scout"}
```

Promtail scrapes `/var/log/pods/k8s-scout_k8s-scout-*/k8s-scout/*.log` only (not Grafana/Loki), parses slog JSON, and promotes `level` to a label. Dashboard folder **k8s-scout** → **k8s-scout logs** (`uid: k8s-scout-logs`).

Useful app log lines (JSON `msg`): `listening`, `github api auth` (`enabled` bool only), `cache refreshed`, `cache refresh failed`, `http request`.

## Troubleshooting

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| Scout Ready but UI amber / `/healthz` `degraded` | GitHub Search 403 / rate limit | Create `k8s-scout-github` (see above). TCP probes will still pass |
| Scout `/healthz` 503 | First refresh failed; empty cache | Same token fix. Check logs: `{namespace="k8s-scout", app="k8s-scout"} \|= "cache refresh failed"` |
| Loki PVC Pending | No `local-path` StorageClass | Install a local-path provisioner or change `storageClassName` in `loki.yaml` |
| Promtail rollout timeout | DaemonSet not scheduled / hostPath | `install.sh` ignores this failure. `kubectl -n k8s-scout describe ds/promtail` |
| No logs in Grafana | Promtail path mismatch or scrape lag | Confirm scout pods are `k8s-scout` in namespace `k8s-scout`; wait ~15s (`target_config.sync_period`) |
| `lang=go` matches almost every issue | Pinned `v0.9.0` still uses the old label-substring filter | Expected on this pin. Bump the image to a release that includes the hint-only filter, or ignore `lang` until then |
| `kubectl exec` has no shell after a scratch-based tag | Newer images are `FROM scratch` | `kubectl -n k8s-scout logs deploy/k8s-scout` and `/healthz`. Pinned `v0.9.0` is still Alpine |
| Scout HTML has no CSS / changing filters does nothing | Browser blocked Tailwind/HTMX CDNs | Allow `cdn.tailwindcss.com` and `unpkg.com`, or open a shareable `/?lang=go` URL |

## Uninstall

```bash
kubectl delete namespace k8s-scout
```

That does **not** remove cluster-scoped Promtail RBAC. Also run:

```bash
kubectl delete clusterrolebinding k8s-scout-promtail
kubectl delete clusterrole k8s-scout-promtail
```
