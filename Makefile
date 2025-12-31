.PHONY: all build test test-race test-coverage test-bench test-integration clean fmt lint vet verify
.PHONY: docker-build docker-build-server docker-build-client docker-build-loadtest docker-up docker-up-all docker-down docker-logs docker-run-client
.PHONY: run-server run-client run-loadtest mod help demo demo-stop
.PHONY: bench bench-all bench-pow bench-ratelimit bench-protocol bench-server bench-quotes
.PHONY: profile-cpu profile-mem profile-pow-cpu profile-pow-mem flamegraph flamegraph-cpu flamegraph-mem bench-compare

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

# Benchmark targets
BENCH_DIR := benchmarks
PROFILE_DIR := $(BENCH_DIR)/profiles
FLAMEGRAPH_DIR := $(BENCH_DIR)/flamegraphs
BENCH_COUNT := 5
BENCH_TIME := 3s

$(BENCH_DIR):
	@mkdir -p $(BENCH_DIR)

$(PROFILE_DIR):
	@mkdir -p $(PROFILE_DIR)

$(FLAMEGRAPH_DIR):
	@mkdir -p $(FLAMEGRAPH_DIR)

bench: bench-all  ## Run all benchmarks (alias)

bench-all: $(BENCH_DIR)  ## Run all benchmarks with memory stats
	@echo "Running all benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./... 2>/dev/null | tee $(BENCH_DIR)/bench-all.txt
	@echo "Results saved to $(BENCH_DIR)/bench-all.txt"

bench-pow: $(BENCH_DIR)  ## Run POW package benchmarks
	@echo "Running POW benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./internal/pow/... | tee $(BENCH_DIR)/bench-pow.txt

bench-ratelimit: $(BENCH_DIR)  ## Run rate limiter benchmarks
	@echo "Running rate limiter benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./internal/ratelimit/... | tee $(BENCH_DIR)/bench-ratelimit.txt

bench-protocol: $(BENCH_DIR)  ## Run protocol benchmarks
	@echo "Running protocol benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./internal/protocol/... | tee $(BENCH_DIR)/bench-protocol.txt

bench-server: $(BENCH_DIR)  ## Run server benchmarks
	@echo "Running server benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./internal/server/... | tee $(BENCH_DIR)/bench-server.txt

bench-quotes: $(BENCH_DIR)  ## Run quotes store benchmarks
	@echo "Running quotes benchmarks..."
	go test -bench=. -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) ./internal/quotes/... | tee $(BENCH_DIR)/bench-quotes.txt

# Profiling targets
profile-cpu: $(PROFILE_DIR)  ## Generate CPU profile for all packages
	@echo "Generating CPU profile..."
	go test -bench=. -cpuprofile=$(PROFILE_DIR)/cpu.prof -benchtime=$(BENCH_TIME) ./internal/pow/...
	@echo "CPU profile saved to $(PROFILE_DIR)/cpu.prof"

profile-mem: $(PROFILE_DIR)  ## Generate memory profile for all packages
	@echo "Generating memory profile..."
	go test -bench=. -memprofile=$(PROFILE_DIR)/mem.prof -benchtime=$(BENCH_TIME) ./internal/pow/...
	@echo "Memory profile saved to $(PROFILE_DIR)/mem.prof"

profile-pow-cpu: $(PROFILE_DIR)  ## Generate CPU profile for POW solver
	@echo "Generating POW CPU profile..."
	go test -bench=BenchmarkSolver -cpuprofile=$(PROFILE_DIR)/pow-cpu.prof -benchtime=10s ./internal/pow/...
	@echo "POW CPU profile saved to $(PROFILE_DIR)/pow-cpu.prof"

profile-pow-mem: $(PROFILE_DIR)  ## Generate memory profile for POW
	@echo "Generating POW memory profile..."
	go test -bench=. -memprofile=$(PROFILE_DIR)/pow-mem.prof -benchtime=$(BENCH_TIME) ./internal/pow/...
	@echo "POW memory profile saved to $(PROFILE_DIR)/pow-mem.prof"

profile-ratelimit: $(PROFILE_DIR)  ## Generate CPU profile for rate limiter
	@echo "Generating rate limiter CPU profile..."
	go test -bench=. -cpuprofile=$(PROFILE_DIR)/ratelimit-cpu.prof -benchtime=10s ./internal/ratelimit/...
	@echo "Rate limiter CPU profile saved to $(PROFILE_DIR)/ratelimit-cpu.prof"

# Flamegraph generation (requires go tool pprof)
flamegraph: flamegraph-cpu flamegraph-mem  ## Generate all flamegraphs

flamegraph-cpu: profile-pow-cpu $(FLAMEGRAPH_DIR)  ## Generate CPU flamegraph
	@echo "Generating CPU flamegraph..."
	@if command -v go tool pprof > /dev/null 2>&1; then \
		go tool pprof -svg $(PROFILE_DIR)/pow-cpu.prof > $(FLAMEGRAPH_DIR)/cpu-flamegraph.svg 2>/dev/null || \
		go tool pprof -png $(PROFILE_DIR)/pow-cpu.prof > $(FLAMEGRAPH_DIR)/cpu-flamegraph.png 2>/dev/null || \
		echo "Install graphviz for SVG output: brew install graphviz"; \
	fi
	@echo "CPU flamegraph saved to $(FLAMEGRAPH_DIR)/"

flamegraph-mem: profile-pow-mem $(FLAMEGRAPH_DIR)  ## Generate memory flamegraph
	@echo "Generating memory flamegraph..."
	@if command -v go tool pprof > /dev/null 2>&1; then \
		go tool pprof -svg -alloc_space $(PROFILE_DIR)/pow-mem.prof > $(FLAMEGRAPH_DIR)/mem-flamegraph.svg 2>/dev/null || \
		go tool pprof -png -alloc_space $(PROFILE_DIR)/pow-mem.prof > $(FLAMEGRAPH_DIR)/mem-flamegraph.png 2>/dev/null || \
		echo "Install graphviz for SVG output: brew install graphviz"; \
	fi
	@echo "Memory flamegraph saved to $(FLAMEGRAPH_DIR)/"

flamegraph-view-cpu: profile-pow-cpu  ## Open interactive CPU flamegraph in browser
	@echo "Opening interactive CPU profile viewer..."
	go tool pprof -http=:8080 $(PROFILE_DIR)/pow-cpu.prof

flamegraph-view-mem: profile-pow-mem  ## Open interactive memory flamegraph in browser
	@echo "Opening interactive memory profile viewer..."
	go tool pprof -http=:8080 $(PROFILE_DIR)/pow-mem.prof

# Benchmark comparison (requires benchstat: go install golang.org/x/perf/cmd/benchstat@latest)
bench-compare: $(BENCH_DIR)  ## Compare benchmarks (run bench-all first, then make changes, then run this)
	@if [ -f $(BENCH_DIR)/bench-all-old.txt ]; then \
		echo "Comparing benchmarks..."; \
		benchstat $(BENCH_DIR)/bench-all-old.txt $(BENCH_DIR)/bench-all.txt; \
	else \
		echo "No previous benchmark found. Run 'make bench-save' first."; \
	fi

bench-save:  ## Save current benchmark as baseline for comparison
	@if [ -f $(BENCH_DIR)/bench-all.txt ]; then \
		cp $(BENCH_DIR)/bench-all.txt $(BENCH_DIR)/bench-all-old.txt; \
		echo "Benchmark saved as baseline."; \
	else \
		echo "No benchmark results found. Run 'make bench-all' first."; \
	fi

# Stress testing
stress-1m:  ## Run 1-minute stress test
	@echo "Running 1-minute stress test..."
	go run ./cmd/loadtest --server localhost:8080 --workers 50 --duration 1m

stress-5m:  ## Run 5-minute stress test
	@echo "Running 5-minute stress test..."
	go run ./cmd/loadtest --server localhost:8080 --workers 100 --duration 5m

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
	rm -rf bin/ coverage.out coverage.html benchmarks/

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
	@echo "Opening Prometheus UI with metrics dashboard..."
	@$(OPEN_CMD) "http://localhost:9092/graph?g0.expr=rate(pow_challenges_issued_total[30s])&g0.tab=0&g0.range_input=5m&g1.expr=rate(pow_challenges_solved_total[30s])&g1.tab=0&g1.range_input=5m&g2.expr=rate(rate_limit_hits_total[30s])&g2.tab=0&g2.range_input=5m&g3.expr=tcp_connections_active&g3.tab=0&g3.range_input=5m" 2>/dev/null || echo "Open http://localhost:9092 in your browser"
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
	@echo "  make test-bench     - Run benchmarks (quick)"
	@echo "  make test-integration - Run integration tests with Docker"
	@echo ""
	@echo "Benchmarks:"
	@echo "  make bench          - Run all benchmarks with memory stats"
	@echo "  make bench-pow      - Run POW package benchmarks"
	@echo "  make bench-ratelimit - Run rate limiter benchmarks"
	@echo "  make bench-protocol - Run protocol benchmarks"
	@echo "  make bench-server   - Run server benchmarks"
	@echo "  make bench-quotes   - Run quotes store benchmarks"
	@echo "  make bench-save     - Save benchmark as baseline"
	@echo "  make bench-compare  - Compare with baseline (requires benchstat)"
	@echo ""
	@echo "Profiling:"
	@echo "  make profile-cpu    - Generate CPU profile"
	@echo "  make profile-mem    - Generate memory profile"
	@echo "  make profile-pow-cpu - Generate POW solver CPU profile"
	@echo "  make profile-pow-mem - Generate POW memory profile"
	@echo ""
	@echo "Flamegraphs:"
	@echo "  make flamegraph     - Generate CPU and memory flamegraphs"
	@echo "  make flamegraph-cpu - Generate CPU flamegraph (SVG)"
	@echo "  make flamegraph-mem - Generate memory flamegraph (SVG)"
	@echo "  make flamegraph-view-cpu - Interactive CPU profile viewer"
	@echo "  make flamegraph-view-mem - Interactive memory profile viewer"
	@echo ""
	@echo "Stress Testing:"
	@echo "  make stress-1m      - Run 1-minute stress test (50 workers)"
	@echo "  make stress-5m      - Run 5-minute stress test (100 workers)"
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
