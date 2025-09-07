.PHONY: help build up down logs test clean import-problems delete-problem proto proto-docker

help:
	@echo "Available commands:"
	@echo "  logs            - View logs for all services"
	@echo "  test            - Run integration tests"
	@echo "  clean           - Clean up containers and volumes"
	@echo "  proto           - Generate protobuf files for Go and Python (local)"
	@echo "  proto-docker    - Generate protobuf files using Docker containers"
	@echo "  import-problems - Import problems from ContestManager/data"
	@echo "  delete-problem  - Delete a problem from the database"
	@echo "  contestmanager  - ContestManager specific commands"
	@echo "  agentmanager    - AgentManager specific commands"

logs:
	docker compose logs -f


proto:
	docker run --rm -v $(PWD):/workspace -w /workspace \
		-e GOPATH=/go \
		-v $(shell go env GOPATH):/go \
		--network host \
		golang:1.23-alpine sh -c "\
		apk add --no-cache protobuf protobuf-dev && \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2 && \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0 && \
		mkdir -p ContestManager/api/grpc && \
		protoc --go_out=ContestManager/api/grpc --go_opt=paths=source_relative \
			--go-grpc_out=ContestManager/api/grpc --go-grpc_opt=paths=source_relative \
			contest.proto"
	docker run --rm -v $(PWD):/workspace -w /workspace \
		python:3.11-alpine sh -c "\
		apk add --no-cache protobuf protobuf-dev && \
		pip install -r AgentManager/requirements.txt && \
		mkdir -p AgentManager/src/grpc_client && \
		python -m grpc_tools.protoc --proto_path=. \
			--python_out=AgentManager/src/grpc_client \
			--grpc_python_out=AgentManager/src/grpc_client \
			contest.proto && \
		touch AgentManager/src/grpc_client/__init__.py"

test:
	docker compose -f docker-compose.test.yml --profile test up --build --abort-on-container-exit
	docker compose -f docker-compose.test.yml down -v

test-contestmanager:
	docker compose -f docker-compose.test.yml --profile test up --build --abort-on-container-exit integration-test test-worker
	docker compose -f docker-compose.test.yml down -v

test-agentmanager:
	docker compose -f docker-compose.test.yml --profile test up --build --abort-on-container-exit contestmanager-test test-worker
	docker compose -f docker-compose.test.yml down -v

test-worker:
	docker compose -f docker-compose.test.yml --profile test up --build --abort-on-container-exit test-worker
	docker compose -f docker-compose.test.yml down -v

clean:
	docker compose down -v
	docker compose -f docker-compose.test.yml down -v
	docker system prune -f

import-problems: PROBLEM_PATH ?= /data/coffee
import-problems:
	docker compose --profile import run --rm import-problems ./import-problems $(PROBLEM_PATH)

delete-problem: PROBLEM_NAME ?= "Make Them Even"
delete-problem:
	docker compose --profile delete run --rm delete-problem ./delete-problem $(PROBLEM_NAME)

contestmanager:
	@echo "ContestManager commands:"
	@echo "  make -C ContestManager build"
	@echo "  make -C ContestManager run"
	@echo "  make -C ContestManager test"

agentmanager:
	@echo "AgentManager commands:"
	@echo "  make -C AgentManager build"
	@echo "  make -C AgentManager run"
	@echo "  make -C AgentManager test"

dev-up:
	docker compose up -d postgres redis
	@echo "Database services started. You can now run individual services locally."

dev-down:
	docker compose down postgres redis

workers-up:
	docker compose up -d worker --scale worker=3


db-reset:
	docker compose down postgres
	docker volume rm projects_postgres_data || true
	docker compose up -d postgres
