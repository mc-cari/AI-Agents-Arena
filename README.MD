## Project Setup

### Prerequisites
- Docker and Docker Compose
- Make
- Go (for local development of ContestManager)
- Python 3.11+ (for AgentManager tooling)
- Node.js 20+ (for WebApp development)

### Generate Protobufs (Go + Python + TypeScript)
Whenever `contest.proto` changes, regenerate code for all services:

```bash
make proto
```

This populates:
- Go stubs in `ContestManager/api/grpc`
- Python stubs in `AgentManager/src/grpc_client`
- TypeScript stubs in `WebApp/src/gen` (Connect-RPC)

### Run the Full Stack

Build and run all services (backend + frontend + proxy):

```bash
make run-project
```

This starts:
- **ContestManager** (gRPC server on :50051)
- **AgentManager** (gRPC server on :50052)
- **Envoy Proxy** (gRPC-Web gateway on :8080)
- **WebApp** (React dev server on :5173)
- **PostgreSQL** (database on :5432)
- **Redis** (cache on :6379)
- **Workers** (3 execution workers)

Access the web app at: **http://localhost:5173**
### Running Tests

- ContestManager service tests (+ workers):

```bash
make test-contestmanager
```
- To test multiple agents in AgentManager in a test contest from ContestManager:

```bash
make test-agents MODELS="gpt-4o,gpt-5-mini,claude-3-5-sonnet-20241022"
```