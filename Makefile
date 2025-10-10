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
	@echo "Generating protobuf files for ContestManager (Go)..."
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
			contest.proto && \
		mkdir -p ContestManager/api/grpc/agentmanager && \
		protoc --go_out=ContestManager/api/grpc/agentmanager --go_opt=paths=source_relative \
			--go-grpc_out=ContestManager/api/grpc/agentmanager --go-grpc_opt=paths=source_relative \
			agent_manager.proto"
	@echo "Generating protobuf files for AgentManager (Python)..."
	docker run --rm -v $(PWD):/workspace -w /workspace \
		python:3.11-alpine sh -c "\
		apk add --no-cache protobuf protobuf-dev && \
		pip install -r AgentManager/requirements.txt && \
		mkdir -p AgentManager/src/grpc_client && \
		cd AgentManager/src/grpc_client && \
		python -m grpc_tools.protoc --proto_path=/workspace \
			--python_out=. \
			--grpc_python_out=. \
			/workspace/contest.proto && \
		cd /workspace && \
		mkdir -p AgentManager/src/grpc_server && \
		cd AgentManager/src/grpc_server && \
		python -m grpc_tools.protoc --proto_path=/workspace \
			--python_out=. \
			--grpc_python_out=. \
			/workspace/agent_manager.proto && \
		cd /workspace && \
		touch AgentManager/src/grpc_client/__init__.py && \
		touch AgentManager/src/grpc_server/__init__.py"
	@echo "Generating protobuf files for WebApp (TypeScript - Connect-RPC)..."
	docker run --rm -v $(PWD):/workspace -w /workspace \
		node:20-alpine sh -c "\
		apk add --no-cache protobuf protobuf-dev && \
		npm install -g @bufbuild/protoc-gen-es@^1.10.0 @connectrpc/protoc-gen-connect-es && \
		mkdir -p WebApp/src/gen && \
		protoc -I . -I /usr/include \
			--es_out=WebApp/src/gen \
			--es_opt=target=ts,import_extension=none \
			--connect-es_out=WebApp/src/gen \
			--connect-es_opt=target=ts,import_extension=none \
			contest.proto"
	@echo "✅ Protobuf generation complete"

test-contestmanager:
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit integration-test test-worker
	docker compose -f docker-compose.test.yml down -v

clean:
	docker compose down -v
	docker compose -f docker-compose.test.yml down -v
	docker system prune -f

import-problems: PROBLEM_PATH ?= /data/coffee
import-problems:
	docker compose --profile import build import-problems
	docker compose --profile import run --rm import-problems ./import-problems $(PROBLEM_PATH)

delete-problem: PROBLEM_NAME ?= "Make Them Even"
delete-problem:
	docker compose --profile delete build delete-problem
	docker compose --profile delete run --rm delete-problem ./delete-problem "$(PROBLEM_NAME)"

run-project:
	docker compose up --build -d
	docker compose logs -f

test-agents: MODELS ?= gpt-4o,gpt-5-mini
test-agents:
	@echo "Running multi-agent test with models: $(MODELS)"
	@mkdir -p $(PWD)/AgentManager/logs
	docker run --rm \
		-v $(PWD)/AgentManager:/app \
		-v $(PWD)/AgentManager/logs:/app/logs \
		-w /app \
		--network projects_contest-network \
		-e CONTEST_MANAGER_HOST=contestmanager:50051 \
		-e AGENT_MANAGER_HOST=agent-manager:50052 \
		-e MODELS="$(MODELS)" \
		python:3.11-alpine sh -c "\
		pip install -q -r requirements.txt && \
		python3 test_simple_contest.py"
	@echo ""
	@echo "📁 Logs available in: AgentManager/logs/"
	@ls -lh $(PWD)/AgentManager/logs/ 2>/dev/null || true

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

db-reset:
	docker compose down postgres
	docker volume rm projects_postgres_data || true
	docker compose up -d postgres
