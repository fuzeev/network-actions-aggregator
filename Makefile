APP_NAME=network-actions-aggregator
MODULE_PATH=network-actions-aggregator

GO=go

.PHONY: help init tidy fmt lint vet test race compose-up compose-down compose-logs run-api run-aggregator run-ingestor run-loadgen

help:
	@echo "Available targets:"
	@echo "  init           - go mod init + tidy (idempotent)"
	@echo "  tidy           - go mod tidy"
	@echo "  fmt            - go fmt ./..."
	@echo "  lint           - run golangci-lint if installed"
	@echo "  vet            - go vet ./..."
	@echo "  test           - go test ./..."
	@echo "  race           - go test -race ./..."
	@echo "  compose-up     - docker compose up (in deploy/)"
	@echo "  compose-down   - docker compose down -v (in deploy/)"
	@echo "  compose-logs   - docker compose logs -f (in deploy/)"
	@echo "  run-api        - run cmd/api"
	@echo "  run-aggregator - run cmd/aggregator"
	@echo "  run-ingestor   - run cmd/ingestor"
	@echo "  run-loadgen    - run cmd/loadgen"

init:
	@if [ ! -f go.mod ]; then $(GO) mod init $(MODULE_PATH); fi
	$(GO) mod tidy

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint not installed (skip)"; fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down -v

compose-logs:
	docker compose -f deploy/docker-compose.yml logs -f

run-api:
	$(GO) run ./cmd/api

run-aggregator:
	$(GO) run ./cmd/aggregator

run-ingestor:
	$(GO) run ./cmd/ingestor

run-loadgen:
	$(GO) run ./cmd/loadgen
