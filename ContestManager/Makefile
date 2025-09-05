# ContestManager Makefile

.PHONY: build run test clean docker-build docker-up docker-down migrate integration-test integration-test-up integration-test-down build-worker run-worker docker-up-workers

GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

import-problems: PROBLEM_PATH ?= /data/coffee
import-problems:
	docker compose --profile import run --rm import-problems ./import-problems $(PROBLEM_PATH)

delete-problem: PROBLEM_NAME ?= "Make Them Even"
delete-problem:
	docker compose --profile delete run --rm delete-problem ./delete-problem $(PROBLEM_NAME)


clean:
	$(GOCLEAN)
	rm -rf bin/

deps:
	$(GOMOD) download
	$(GOMOD) tidy

build:
	mkdir -p bin
	$(GOBUILD) -o bin/server ./cmd/server
	$(GOBUILD) -o bin/import-problems ./cmd/import-problems
	$(GOBUILD) -o bin/delete-problem ./cmd/delete-problem

run: build
	cd bin && ./server

build-worker:
	mkdir -p bin
	$(GOBUILD) -o bin/worker ./cmd/worker

run-worker: build-worker
	cd bin && ./worker

docker-build:
	docker build -t contestmanager .

docker-up:
	docker compose up -d

docker-up-workers:
	docker compose up -d worker --scale worker=3

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f contestmanager

integration-test-up:
	docker compose -f docker-compose.test.yml up -d postgres-test redis-test

integration-test-down:
	docker compose -f docker-compose.test.yml down -v

integration-test: integration-test-up
	@echo "Starting integration tests..."
	@echo "Waiting for services to be ready..."
	@sleep 1
	@echo "Building test containers..."
	docker compose -f docker-compose.test.yml build integration-test test-worker
	@echo "Starting test worker..."
	docker compose -f docker-compose.test.yml --profile test up -d test-worker
	@echo "Waiting for worker to be ready..."
	@sleep 1
	@echo "Running integration tests..."
	docker compose -f docker-compose.test.yml run --rm integration-test
	@echo "Integration tests completed. Cleaning up..."
	@make integration-test-down



PROTOC_GEN_GO := $(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc

proto: $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	@echo "Checking protoc installation..."
	@which protoc > /dev/null || (echo "Error: protoc not found. Please install it:" && echo "  Linux: sudo apt-get install -y protobuf-compiler" && echo "  Mac: brew install protobuf" && exit 1)
	@echo "Generating Go code from proto..."
	@mkdir -p api/grpc
	cd proto && PATH=$(shell go env GOPATH)/bin:$(PATH) protoc \
		--go_out=../api/grpc \
		--go_opt=paths=source_relative \
		--go-grpc_out=../api/grpc \
		--go-grpc_opt=paths=source_relative \
		contest.proto
	@echo "Proto files generated successfully in api/grpc/"

$(PROTOC_GEN_GO): $(shell which go)
	@echo "Installing protoc-gen-go..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

$(PROTOC_GEN_GO_GRPC): $(shell which go)
	@echo "Installing protoc-gen-go-grpc..."
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

fmt:
	$(GOCMD) fmt ./...

help:
	@echo "Available commands:"
	@echo "  build       - Build the application"
	@echo "  integration-test-with-worker - Run integration tests with worker system"
	@echo "  run         - Run the server"
	@echo "  migrate     - Run database migration"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  deps        - Download dependencies"
	@echo "  docker-build- Build Docker image"
	@echo "  docker-up   - Start services with Docker Compose"
	@echo "  docker-down - Stop Docker Compose services"
	@echo "  docker-logs - View application logs"
	@echo "  integration-test - Run integration tests with Docker"
	@echo "  integration-test-up - Start test services"
	@echo "  integration-test-down - Stop test services"
	@echo "  import-problems - Import problems from a directory"
	@echo "  proto       - Generate protobuf code"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo "  help        - Show this help message"
