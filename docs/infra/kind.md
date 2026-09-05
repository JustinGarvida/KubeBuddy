# Local KIND Cluster

Guide for the local [KIND](https://kind.sigs.k8s.io/) cluster used for development and the manual verification steps described in [`architecture.md`](../architecture.md#testing--verification-strategy).

Topology: 1 control-plane node + 2 worker nodes (defined in [`infra/kind/kind-config.yaml`](../../infra/kind/kind-config.yaml)), so the Go agent's pod polling can be exercised across multiple nodes.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [`kind`](https://kind.sigs.k8s.io/docs/user-guide/quick-start/#installation) and [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl)

```bash
brew install kind kubectl
```

## Create the cluster

```bash
kind create cluster --config infra/kind/kind-config.yaml
```

## Verify

```bash
kubectl cluster-info --context kind-podsentinel
kubectl get nodes
```

All nodes should show `Ready`.

## Install metrics-server

The Go agent's poller reads pod CPU/memory from the `metrics.k8s.io`
API, which KIND doesn't provide out of the box — it needs
[metrics-server](https://github.com/kubernetes-sigs/metrics-server)
installed.

```bash
kubectl --context kind-podsentinel apply -f infra/kind/metrics-server.yaml
```

[`infra/kind/metrics-server.yaml`](../../infra/kind/metrics-server.yaml)
is the upstream `components.yaml` for a pinned metrics-server release
(`v0.9.0`, matching this repo's convention of pinning infra image
tags), with one change: the container args add
`--kubelet-insecure-tls`. KIND's kubelet serving certificates aren't
signed for the hostnames/IPs metrics-server validates against by
default, so without that flag the `metrics-server` Deployment never
reaches `Ready` on a KIND cluster.

## Verify metrics-server

```bash
kubectl --context kind-podsentinel -n kube-system rollout status deployment/metrics-server
kubectl --context kind-podsentinel top nodes
kubectl --context kind-podsentinel top pods -A
```

The rollout should report `successfully rolled out`, and both `top`
commands should print CPU/memory numbers once metrics-server has
completed its first scrape (can take up to ~1 minute after the
Deployment becomes ready).

## Tear down

```bash
kind delete cluster --name podsentinel
```

## Next steps

Schema, RabbitMQ topology, and Postgres/`client-go` wiring for the Go
agent's ingestion path are tracked separately — see
[`docs/kube-agent.md`](../kube-agent.md).
