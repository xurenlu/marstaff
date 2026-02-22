# Marstaff Makefile

.PHONY: all build clean test run-gateway run-agent deps fmt lint docker

# Variables
BINARY_DIR=bin
CMD_DIR=cmd
GO=go
GOFLAGS=-v
LDFLAGS=-w -s

# Build targets
GATEWAY_BINARY=$(BINARY_DIR)/gateway
AGENT_BINARY=$(BINARY_DIR)/agent

all: deps build

deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

build: build-gateway build-agent

build-gateway:
	@echo "Building gateway..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(GATEWAY_BINARY) $(CMD_DIR)/gateway/main.go

build-agent:
	@echo "Building agent..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(AGENT_BINARY) $(CMD_DIR)/agent/main.go

clean:
	@echo "Cleaning..."
	rm -rf $(BINARY_DIR)

test:
	@echo "Running tests..."
	$(GO) test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

lint:
	@echo "Linting code..."
	golangci-lint run ./...

run-gateway: build-gateway
	@echo "Running gateway..."
	./$(GATEWAY_BINARY) --config configs/config.yaml

run-agent: build-agent
	@echo "Running agent..."
	./$(AGENT_BINARY) --config configs/config.yaml

docker-build:
	@echo "Building Docker image..."
	docker build -t marstaff:latest -f deployments/docker/Dockerfile .

docker-run:
	@echo "Running Docker container..."
	docker-compose -f deployments/docker/docker-compose.yml up

migrate-up:
	@echo "Running database migrations up..."
	@echo "Please run manually: mysql -u root -p marstaff < migrations/001_init_schema.up.sql"

migrate-down:
	@echo "Running database migrations down..."
	@echo "Please run manually: mysql -u root -p marstaff < migrations/001_init_schema.down.sql"

help:
	@echo "Available targets:"
	@echo "  all           - Install dependencies and build"
	@echo "  deps          - Install dependencies"
	@echo "  build         - Build all binaries"
	@echo "  build-gateway - Build gateway binary"
	@echo "  build-agent   - Build agent binary"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linter"
	@echo "  run-gateway   - Build and run gateway"
	@echo "  run-agent     - Build and run agent"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run with Docker Compose"
	@echo "  migrate-up    - Run database migrations up"
	@echo "  migrate-down  - Run database migrations down"
