.PHONY: build run test test-integration vet tidy fmt docker-build infra-up infra-down kind-up kind-down

BINARY := agent
COMPOSE := docker compose -f ../infra/docker-compose.yml

## Build the agent binary into ./bin
build:
	go build -o bin/$(BINARY) ./cmd/agent

## Run the agent locally (uses env vars from your shell / .env)
run:
	go run ./cmd/agent

## Run unit tests (store tests skip if timescaledb isn't reachable)
test:
	go test ./...

## Run the end-to-end integration test against a real KIND cluster + timescaledb
test-integration:
	go test -tags=integration ./internal/integration/...

## Vet the module
vet:
	go vet ./...

## Tidy go.mod/go.sum
tidy:
	go mod tidy

## Format all Go source
fmt:
	gofmt -l -w .

## Build the agent's Docker image
docker-build:
	docker build -t podsentinel-agent .

## Start the local timescaledb (+ rabbitmq) containers
infra-up:
	$(COMPOSE) up -d

## Stop the local infra containers (keeps volumes)
infra-down:
	$(COMPOSE) down

## Create the local KIND cluster and install metrics-server
kind-up:
	kind create cluster --config ../infra/kind/kind-config.yaml
	kubectl --context kind-podsentinel apply -f ../infra/kind/metrics-server.yaml
	kubectl --context kind-podsentinel -n kube-system rollout status deployment/metrics-server

## Tear down the local KIND cluster
kind-down:
	kind delete cluster --name podsentinel
