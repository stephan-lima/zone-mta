# ZoneMTA-Go Makefile

# Variables
BINARY_NAME=zone-mta
BINARY_PATH=./cmd/zone-mta
CONFIG_PATH=config/default.yaml
DOCKER_IMAGE=zone-mta-go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) $(BINARY_PATH)

# Build for multiple platforms
.PHONY: build-all
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 $(BINARY_PATH)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64 $(BINARY_PATH)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 $(BINARY_PATH)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 $(BINARY_PATH)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe $(BINARY_PATH)

# Run the application
.PHONY: run
run:
	@echo "Running $(BINARY_NAME)..."
	go run $(BINARY_PATH) -config $(CONFIG_PATH)

# Run with development config
.PHONY: dev
dev:
	@echo "Running in development mode..."
	go run $(BINARY_PATH) -config config/dev.yaml

# Test all packages
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Lint the code
.PHONY: lint
lint:
	@echo "Linting code..."
	golangci-lint run

# Format the code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

# Update dependencies
.PHONY: deps
deps:
	@echo "Updating dependencies..."
	go mod tidy
	go mod download

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)*
	rm -f coverage.out coverage.html
	go clean

# Docker build
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest

# Docker run
.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -p 2525:2525 -p 12080:12080 -p 9090:9090 $(DOCKER_IMAGE):latest

# Docker compose up
.PHONY: compose-up
compose-up:
	@echo "Starting Docker Compose services..."
	docker-compose up -d

# Docker compose up with admin tools
.PHONY: compose-up-admin
compose-up-admin:
	@echo "Starting Docker Compose services with admin tools..."
	docker-compose --profile admin up -d

# Docker compose down
.PHONY: compose-down
compose-down:
	@echo "Stopping Docker Compose services..."
	docker-compose down

# Docker compose logs
.PHONY: compose-logs
compose-logs:
	@echo "Showing Docker Compose logs..."
	docker-compose logs -f zone-mta-go

# Install development tools
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

# Create development config
.PHONY: dev-config
dev-config:
	@echo "Creating development configuration..."
	@if [ ! -f config/dev.yaml ]; then \
		cp config/default.yaml config/dev.yaml; \
		echo "Created config/dev.yaml - please edit as needed"; \
	else \
		echo "config/dev.yaml already exists"; \
	fi

# Show configuration
.PHONY: show-config
show-config:
	@echo "Showing configuration..."
	go run $(BINARY_PATH) -show-config -config $(CONFIG_PATH)

# Show version
.PHONY: version
version:
	@echo "Version: $(VERSION)"

# Database setup (requires MongoDB and Redis to be running)
.PHONY: db-setup
db-setup:
	@echo "Setting up databases..."
	@echo "Checking MongoDB connection..."
	@mongosh --eval "db.adminCommand('ping')" > /dev/null || (echo "MongoDB not available" && exit 1)
	@echo "Checking Redis connection..."
	@redis-cli ping > /dev/null || (echo "Redis not available" && exit 1)
	@echo "Databases are ready"

# Send test email via API
.PHONY: test-send
test-send:
	@echo "Sending test email via API..."
	curl -X POST http://localhost:12080/api/v1/queue/send \
		-H "Content-Type: application/json" \
		-d '{"from":"test@example.com","to":["recipient@example.com"],"subject":"Test Email","message":"From: test@example.com\r\nTo: recipient@example.com\r\nSubject: Test Email\r\n\r\nHello World!"}'

# Check queue stats
.PHONY: check-queue
check-queue:
	@echo "Checking queue statistics..."
	curl -s http://localhost:12080/api/v1/queue/stats | jq .

# Health check
.PHONY: health
health:
	@echo "Checking application health..."
	curl -s http://localhost:12080/health | jq .

# Run plugin example
.PHONY: plugin-example
plugin-example:
	@echo "Running plugin development example..."
	cd examples/plugin-example && go run main.go

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  build-all     - Build for multiple platforms"
	@echo "  run           - Run the application"
	@echo "  dev           - Run in development mode"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  bench         - Run benchmarks"
	@echo "  lint          - Lint the code"
	@echo "  fmt           - Format the code"
	@echo "  deps          - Update dependencies"
	@echo "  clean         - Clean build artifacts"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  compose-up    - Start Docker Compose services"
	@echo "  compose-down  - Stop Docker Compose services"
	@echo "  install-tools - Install development tools"
	@echo "  dev-config    - Create development configuration"
	@echo "  show-config   - Show configuration"
	@echo "  db-setup      - Setup databases"
	@echo "  test-send     - Send test email via API"
	@echo "  check-queue   - Check queue statistics"
	@echo "  health        - Check application health"
	@echo "  plugin-example- Run plugin development example"
	@echo "  help          - Show this help"