.PHONY: build test clean run help

# Variables
BINARY_NAME=generator
MAIN_PATH=./cmd
BUILD_DIR=./build
DOCKER_IMAGE=ghcr.io/echomessenger/generator:latest
PLATFORM  ?= linux/amd64

# Default target
help:
	@echo "Available targets:"
	@echo "  make build          - Build generator binary"
	@echo "  make test           - Run unit tests"
	@echo "  make test-integration - Run integration tests (requires local Tinode server)"
	@echo "  make run            - Run generator with config.yaml"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run generator in Docker"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make install-deps   - Download dependencies"

# Install dependencies
install-deps:
	go mod download
	go mod tidy

# Build
build: install-deps
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) -v $(MAIN_PATH)

# Development run
run: build
	$(BUILD_DIR)/$(BINARY_NAME) -config config.yaml

# Test
test: install-deps
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-integration: build
	go test -v -race -tags=integration ./...

# Code quality
fmt:
	go fmt ./...

lint:
	go vet ./...

# Docker
docker-build:
	docker build \
	    --platform $(PLATFORM) \
		--tag $(DOCKER_IMAGE) \
		.

docker-run: docker-build
	docker run --rm \
		-v $(PWD)/config.yaml:/app/config.yaml \
		--network host \
		$(DOCKER_IMAGE)

docker-push: docker-build
	docker push $(DOCKER_IMAGE)

# Cleanup
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	go clean

# Dev setup (one-time)
dev-setup: install-deps
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

.DEFAULT_GOAL := help
