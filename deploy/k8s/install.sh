#!/usr/bin/env bash
# Install scout + Loki + Promtail + Grafana into namespace k8s-scout.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"

kubectl apply -f "${ROOT}/namespace.yaml"
kubectl apply -f "${ROOT}/loki.yaml"
kubectl apply -f "${ROOT}/promtail.yaml"
kubectl apply -f "${ROOT}/grafana.yaml"
kubectl apply -f "${ROOT}/scout.yaml"

echo "Waiting for workloads..."
kubectl -n k8s-scout rollout status deploy/loki --timeout=180s
kubectl -n k8s-scout rollout status deploy/grafana --timeout=180s
kubectl -n k8s-scout rollout status deploy/k8s-scout --timeout=180s
kubectl -n k8s-scout rollout status daemonset/promtail --timeout=180s || true

NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
TS_IP="$(tailscale ip -4 2>/dev/null || true)"

echo
echo "Deployed namespace k8s-scout"
echo "  Scout:   http://${TS_IP:-$NODE_IP}:30808   (NodePort)"
echo "  Grafana: http://${TS_IP:-$NODE_IP}:30300   (admin / scout)"
echo "  Explore: {namespace=\"k8s-scout\", app=\"k8s-scout\"}"
echo
kubectl -n k8s-scout get pods,svc,pvc
