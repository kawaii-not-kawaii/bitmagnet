# Development Guide

## Prerequisites

- Go 1.23.6+
- Node.js 22+
- PostgreSQL 16
- Task runner (go-task)
- Nix (optional, for dev shell via flake.nix)
- Protobuf compiler + protoc-gen-go (for protobuf generation)
- Chromium (for Angular tests in CI)

## Quick Start

### 1. Clone and enter the repository

```bash
git clone https://github.com/bitmagnet-io/bitmagnet.git
cd bitmagnet
```

### 2. Set up PostgreSQL

```bash
# Create database
createdb bitmagnet

# Or run via Docker
docker run -d --name bitmagnet-pg -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=bitmagnet -p 5432:5432 postgres:16-alpine
```

### 3. Set up environment

```bash
# Copy example env file
cp .env.example .env  # if exists, or set:
export POSTGRES_PASSWORD=postgres
```

### 4. Install dependencies and run migrations

```bash
task install-webui   # npm ci in webui/
task migrate         # goose up
```

### 5. Run the backend

```bash
# Run all workers (HTTP server, DHT crawler, queue server)
go run . worker run --all
```

### 6. Run the frontend (separate terminal)

```bash
cd webui
npm start  # Angular dev server on :3334, proxied to backend :3333
```

## Build Commands

| Command                     | Description                      |
| --------------------------- | -------------------------------- |
| `task build`                | Build Go binary with git tag     |
| `task build-webui`          | Build Angular app for production |
| `task build-docsite`        | Build Jekyll documentation site  |
| `task install-webui`        | Install npm dependencies         |
| `go run . worker run --all` | Run all workers                  |

## Code Generation

| Command                      | Description                      |
| ---------------------------- | -------------------------------- |
| `task gen-go`                | go generate ./...                |
| `task gen-gorm`              | Generate GORM DAOs               |
| `task gen-gql-enums`         | Generate GraphQL enum stringers  |
| `task gen-gql`               | Generate gqlgen code from schema |
| `task gen-protoc`            | Compile protobuf definitions     |
| `task gen-mockery`           | Generate mock implementations    |
| `task gen-classifier-schema` | Generate classifier JSON schema  |
| `task gen-webui-graphql`     | Generate Angular GraphQL types   |

## Testing

| Command           | Description               |
| ----------------- | ------------------------- |
| `task test`       | Run all tests             |
| `task test-go`    | go test ./...             |
| `task test-webui` | ng test (Karma + Jasmine) |

## Linting

| Command                           | Description               |
| --------------------------------- | ------------------------- |
| `task lint`                       | Run all linters           |
| `task lint-webui`                 | ESLint for Angular        |
| `task lint-prettier`              | Prettier formatting check |
| `golangci-lint run --timeout=10m` | Go linting                |

## Project Structure

- `internal/` - Go backend code organized by subsystem
- `webui/` - Angular frontend
- `graphql/` - GraphQL schema definitions
- `migrations/` - SQL migrations (goose)
- `bitmagnet.io/` - Jekyll documentation site

## CLI Commands

Available CLI commands:

```bash
# Classifier operations
go run . classifier show        # Show classification rules
go run . classifier schema      # Generate JSON schema

# Configuration
go run . config show            # Display resolved config tree

# Processing
go run . process --help         # Single torrent processing

# Reprocessing
go run . reprocess --help       # Batch reprocess torrents

# Worker management
go run . worker run --all       # Run all workers
go run . worker list            # List available workers
```

## Development Utilities

Located in `internal/dev/`:

- Database migration management
- GORM code generation
- Development-specific commands

## Code Conventions

- **Go:** Follow standard Go conventions with golangci-lint enforcement (~60 linters)
- **TypeScript:** ESLint with Prettier formatting
- **GraphQL:** Schema-driven development with code generation
- **Commits:** Conventional commits (feat:, fix:, etc.)
- **Line length:** 120 characters (Go and TypeScript)

## Environment Variables

Key configuration via environment variables (prefixed with subsystem\_):

- `POSTGRES_PASSWORD` - Database password
- `HTTP_SERVER_LOCAL_ADDRESS` - HTTP listen address (default :3333)
- `TMDB_API_KEY` - TMDB API key for content enrichment
- `LOG_FILE_ROTATOR_ENABLED` - Enable file logging
- `EXTRA_CONFIG_FILES` - Additional YAML config files
