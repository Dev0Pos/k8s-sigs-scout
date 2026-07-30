# Kubernetes deploy (scout + Loki + Grafana)

Self-contained stack in namespace `k8s-scout` (does **not** use the DataWalk monitoring stack).

| Component | Purpose | Access |
|-----------|---------|--------|
| **k8s-scout** | dashboard (`LOG_FORMAT=json`) | NodePort **30808** |
| **Loki** | log store | ClusterIP `:3100` |
| **Promtail** | ships pod logs → Loki | DaemonSet |
| **Grafana** | Explore + preloaded dashboard | NodePort **30300** (`admin` / `scout`) |

## Install (on the cluster host)

```bash
cd deploy/k8s
chmod +x install.sh
./install.sh
```

Or:

```bash
kubectl apply -f namespace.yaml -f loki.yaml -f promtail.yaml -f grafana.yaml -f scout.yaml
```

Optional GitHub token (higher API rate limits):

```bash
kubectl -n k8s-scout create secret generic k8s-scout-github \
  --from-literal=GITHUB_TOKEN=ghp_xxx \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n k8s-scout rollout restart deploy/k8s-scout
```

## URLs (via Tailscale)

- Scout: http://100.76.137.100:30808  
- Grafana: http://100.76.137.100:30300  

In Grafana → **Explore** → Loki query:

```logql
{namespace="k8s-scout", app="k8s-scout"}
```

Dashboard folder **k8s-scout** → **k8s-scout logs**.

## Uninstall

```bash
kubectl delete namespace k8s-scout
```
