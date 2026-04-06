# Marstaff Makefile

.PHONY: all build clean test run deps fmt lint docker doctor

# Variables
BINARY_DIR=bin
CMD_DIR=cmd
GO=go
GOFLAGS=-v
# 注入版本号和 git 提交哈希，便于运行时可确认代码版本
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-w -s -X main.GitCommit=$(GIT_COMMIT)

# Build targets
GATEWAY_BINARY=$(BINARY_DIR)/gateway

all: deps build

deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

playwright-install:
	@echo "Installing Playwright sidecar dependencies..."
	cd playwright-sidecar && npm install && npx playwright install

build: build-gateway

build-onboard:
	@echo "Building onboard CLI..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY_DIR)/onboard $(CMD_DIR)/onboard/main.go

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

onboard: build-onboard
	@echo "Running onboard wizard..."
	./$(BINARY_DIR)/onboard

# Update skills from openclaw-master-skills (https://github.com/LeoYeAI/openclaw-master-skills)
# Debug failed video workflow: make debug-pipeline SESSION_ID=your-session-id
debug-pipeline:
	@if [ -z "$(SESSION_ID)" ]; then echo "Usage: make debug-pipeline SESSION_ID=<session_id from chat>"; exit 1; fi; \
	port=$${API_PORT:-15678}; \
	echo "Fetching pipelines for session $(SESSION_ID) from localhost:$$port..."; \
	curl -s "http://localhost:$$port/api/pipelines?session_id=$(SESSION_ID)" | jq '.pipelines[] | {id, name, status, error, steps: [.steps[]? | {step_key, name, status, error}]}' 2>/dev/null || curl -s "http://localhost:$$port/api/pipelines?session_id=$(SESSION_ID)"

short-drama-init:
	@if [ -z "$(SERIES)" ]; then echo "Usage: make short-drama-init SERIES=my_anime [TITLE='My Anime'] [STYLE='cel shading...']"; exit 1; fi
	./scripts/init-short-drama-db.sh "$(SERIES)" "$(TITLE)" "$(STYLE)"

skills-update:
	@echo "Updating skills from openclaw-master-skills..."
	@if [ ! -d /tmp/openclaw-master-skills ]; then \
		git clone --depth 1 git@github.com:LeoYeAI/openclaw-master-skills.git /tmp/openclaw-master-skills; \
	else \
		cd /tmp/openclaw-master-skills && git pull --rebase; \
	fi
	cp -r /tmp/openclaw-master-skills/skills/* ./skills/
	@echo "Skills updated. Builtin skills (calculator, weather) are preserved."

doctor: build-gateway
	@echo "Running gateway doctor (config + DB + paths)..."
	./$(GATEWAY_BINARY) doctor --config configs/config.yaml

help:
	@echo "Available targets:"
	@echo "  all           - Install dependencies and build"
	@echo "  deps          - Install dependencies"
	@echo "  build         - Build server binary"
	@echo "  build-gateway - Build server binary"
	@echo "  build-onboard - Build onboard CLI"
	@echo "  onboard       - Run interactive config wizard"
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
	@echo "  doctor            - Run gateway doctor (health checks)"
	@echo "  skills-update     - Update skills from openclaw-master-skills repo"
	@echo "  short-drama-init  - Init short drama project (SERIES=slug TITLE=... STYLE=...)"
	@echo "  debug-pipeline    - Fetch failed pipeline errors (SESSION_ID=xxx API_PORT=15678)"
