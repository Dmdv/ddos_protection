.PHONY: all build test test-race test-coverage test-bench test-integration clean fmt lint vet verify
.PHONY: docker-build docker-build-server docker-build-client docker-build-loadtest docker-up docker-up-all docker-down docker-logs docker-run-client
.PHONY: run-server run-client run-loadtest mod help demo demo-stop

# Detect OS for browser opening
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

# Build targets
all: verify build

build: build-server build-client build-loadtest

build-server:
	@echo "Building server..."
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/server ./cmd/server

build-client:
	@echo "Building client..."
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/client ./cmd/client

build-loadtest:
	@echo "Building loadtest..."
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/loadtest ./cmd/loadtest

# Test targets
test:
	@echo "Running tests..."
	go test -v ./...

test-unit:
	@echo "Running unit tests..."
	go test -v -short ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

test-pow:
	@echo "Running PoW tests..."
	go test -v ./internal/pow/...

# Verification targets (mandatory before deployment)
verify: fmt vet lint test
	@echo "All verification checks passed!"

fmt:
	@echo "Formatting code..."
	gofmt -w -s .

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

# Docker targets
docker-build: docker-build-server docker-build-client

docker-build-server:
	@echo "Building server Docker image..."
	docker build --target server -t wow-server:latest .

docker-build-client:
	@echo "Building client Docker image..."
	docker build --target client -t wow-client:latest .

docker-build-loadtest:
	@echo "Building loadtest Docker image..."
	docker build --target loadtest -t wow-loadtest:latest .

docker-up:
	@echo "Starting services..."
	docker compose up -d server

docker-up-all:
	@echo "Starting services with monitoring..."
	docker compose --profile monitoring up -d

docker-down:
	@echo "Stopping services..."
	docker compose down

docker-logs:
	docker compose logs -f

docker-run-client:
	@echo "Running client against Docker server..."
	docker compose run --rm client

# Clean targets
clean:
	rm -rf bin/ coverage.out coverage.html

# Development helpers
run-server:
	POW_SECRET=dev-secret-exactly-32-bytes!!!!! go run ./cmd/server

run-client:
	go run ./cmd/client

run-loadtest:
	go run ./cmd/loadtest --server localhost:8080

mod:
	go mod tidy
	go mod download

# Integration testing
test-integration: docker-build
	@echo "Running integration tests..."
	@docker compose up -d server
	@echo "Waiting for server to be healthy..."
	@sleep 5
	@docker compose run --rm client && echo "Integration test PASSED" || (docker compose down && exit 1)
	@docker compose down
	@echo "Integration tests completed!"

# Demo targets
demo:
	@echo "=== Word of Wisdom DDoS Protection Demo ==="
	@echo ""
	@echo "Starting server, Prometheus, and load test..."
	@PROMETHEUS_PORT=9092 docker compose --profile demo up --build -d server prometheus
	@echo ""
	@echo "Waiting for server to be healthy..."
	@sleep 10
	@echo ""
	@echo "Opening Prometheus UI..."
	@$(OPEN_CMD) "http://localhost:9092/graph?g0.expr=pow_challenges_issued_total&g0.tab=0" 2>/dev/null || echo "Open http://localhost:9092 in your browser"
	@echo ""
	@echo "Starting load test (20 workers, 60 seconds)..."
	@echo "Watch the metrics update in real-time in your browser!"
	@echo ""
	@PROMETHEUS_PORT=9092 docker compose --profile demo run --rm loadtest
	@echo ""
	@echo "Demo complete! Server and Prometheus are still running."
	@echo "Run 'make demo-stop' to stop all services."

demo-stop:
	@echo "Stopping demo services..."
	@PROMETHEUS_PORT=9092 docker compose --profile demo down
	@echo "Demo services stopped."

# Help target
help:
	@echo "Word of Wisdom Server - Available targets:"
	@echo ""
	@echo "Build:"
	@echo "  make build          - Build server and client binaries"
	@echo "  make build-server   - Build server binary only"
	@echo "  make build-client   - Build client binary only"
	@echo ""
	@echo "Test:"
	@echo "  make test           - Run all tests"
	@echo "  make test-unit      - Run unit tests only"
	@echo "  make test-race      - Run tests with race detector"
	@echo "  make test-coverage  - Generate coverage report"
	@echo "  make test-bench     - Run benchmarks"
	@echo "  make test-integration - Run integration tests with Docker"
	@echo ""
	@echo "Verify:"
	@echo "  make verify         - Run all verification (fmt, vet, lint, test)"
	@echo "  make fmt            - Format code"
	@echo "  make vet            - Run go vet"
	@echo "  make lint           - Run golangci-lint"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build   - Build Docker images"
	@echo "  make docker-up      - Start server"
	@echo "  make docker-up-all  - Start with monitoring (Prometheus, Grafana)"
	@echo "  make docker-down    - Stop all services"
	@echo "  make docker-logs    - Follow container logs"
	@echo "  make docker-run-client - Run client against Docker server"
	@echo ""
	@echo "Demo:"
	@echo "  make demo           - Run full demo (server + Prometheus + load test)"
	@echo "  make demo-stop      - Stop demo services"
	@echo ""
	@echo "Development:"
	@echo "  make run-server     - Run server locally"
	@echo "  make run-client     - Run client locally"
	@echo "  make run-loadtest   - Run load test locally"
	@echo "  make clean          - Remove build artifacts"
