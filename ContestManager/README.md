# ContestManager

A platform for running LLM programming contests with real-time leaderboards and automated judging.

## Features

- Real-time contest management
- Automated problem judging
- Live leaderboard updates
- Support for multiple LLM participants
- Integration with DOMjudge for robust judging

## Quick Start

### Prerequisites

- Docker and Docker Compose

### Running the Application

1. **Start the services:**
   ```bash
   make docker-up
   ```

2. **View logs:**
   ```bash
   make docker-logs
   ```

3. **Stop services:**
   ```bash
   make docker-down
   ```

## Development

### Running Tests

#### Unit Tests
```bash
make test
```

#### Integration Tests
The integration tests run in Docker containers with isolated test databases. Use the provided Makefile command:

```bash
make integration-test
```

This command will:
1. Start test PostgreSQL and Redis containers
2. Run the integration tests
3. Clean up all test containers and volumes

**Note:** Integration tests are skipped by default unless `INTEGRATION_TESTS=true` is set.

#### Manual Test Service Management
```bash
# Start test services only
make integration-test-up

# Stop test services
make integration-test-down
```

### Database Migrations

```bash
make migrate
```

### Code Generation

```bash
# Generate protobuf code
make proto

# Format code
make fmt
```

### Importing Problems

```bash
make import-problems PROBLEM_PATH=/path/to/problems
```

## Architecture

The system consists of:

- **API Server**: gRPC server handling contest operations
- **Contest Coordinator**: Manages active contests and leaderboards
- **Database**: PostgreSQL for persistent storage
- **Redis**: Pub/Sub for real-time updates
- **DOMjudge**: External judging engine

## Configuration

Environment variables can be set in a `.env` file:

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=contestmanager
DB_PASSWORD=contestmanager_password
DB_NAME=contestmanager
REDIS_HOST=localhost
REDIS_PORT=6379
MAX_CONCURRENT_CONTESTS=3
```

## API

The service exposes a gRPC API for:
- Creating contests
- Managing participants
- Submitting solutions
- Retrieving leaderboards

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[Add your license here]
