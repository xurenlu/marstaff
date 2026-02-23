# Marstaff Makefile

.PHONY: all build clean test run deps fmt lint docker

# Variables
BINARY_DIR=bin
CMD_DIR=cmd
GO=go
GOFLAGS=-v
LDFLAGS=-w -s

# Build targets
GATEWAY_BINARY=$(BINARY_DIR)/gateway

all: deps build

deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

build: build-gateway

build-gateway:
	@echo "Building Marstaff server..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(GATEWAY_BINARY) $(CMD_DIR)/gateway/main.go

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

run: build-gateway
	@echo "Running Marstaff server..."
	./$(GATEWAY_BINARY) --config configs/config.yaml

docker-build:
	@echo "Building Docker image..."
	docker build -t marstaff:latest -f deployments/docker/Dockerfile .

docker-run:
	@echo "Running Docker container..."
	docker-compose -f deployments/docker/docker-compose.yml up

migrate-up:
	@echo "Running database migrations up..."
	@echo "Please run manually: mysql -u root -p marstaff < migrations/001_init_schema.up.sql"

migrate-single-user:
	@echo "Migrating sessions to default user (single-user mode)..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/migrate ./cmd/migrate/
	./$(BINARY_DIR)/migrate --config configs/config.yaml

migrate-down:
	@echo "Running database migrations down..."
	@echo "Please run manually: mysql -u root -p marstaff < migrations/001_init_schema.down.sql"

help:
	@echo "Available targets:"
	@echo "  all           - Install dependencies and build"
	@echo "  deps          - Install dependencies"
	@echo "  build         - Build server binary"
	@echo "  build-gateway - Build server binary"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linter"
	@echo "  run           - Build and run server"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run with Docker Compose"
	@echo "  migrate-up        - Run database migrations up"
	@echo "  migrate-single-user - Migrate sessions to default user"
	@echo "  migrate-down      - Run database migrations down"
