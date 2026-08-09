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

## Tear down

```bash
kind delete cluster --name podsentinel
```

## Next steps

`metrics-server` isn't installed by this config yet — tracked as a follow-up (see issue #6).
