## Project Setup

### Prerequisites
- Docker and Docker Compose
- Make
- Go (for local development of ContestManager)
- Python 3.11+ (for AgentManager tooling)

### Generate Protobufs (Go + Python)
Whenever `contest.proto` changes, regenerate code for both services:

```bash
make proto
```

This populates:
- Go stubs in `ContestManager/api/grpc`
- Python stubs in `AgentManager/src/grpc_client`

Run the following command to build and run the project:

```bash
make run-project
```


### Running Tests


- ContestManager service tests (+ workers):

```bash
make test-contestmanager
```

- To test multiple agents in AgentManager in a test contest from ContestManager:

```bash
make test-agents MODELS="gpt-4o,gpt-5-mini,claude-3-5-sonnet-20241022"
```

