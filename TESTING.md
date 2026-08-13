# ENVY Testing

## Current validation commands

ENVY has no unit or integration tests yet. The following commands verify that
the code compiles and passes static analysis:

```sh
# Build both binaries (cli + api)
make build

# Run go vet on both modules
make vet
```

## What each command validates

| Command | What it checks |
|---|---|
| `make build` | Both `cli/cmd/envy` and `api/cmd/envyd` compile without errors. Output goes to `bin/envy` and `bin/envyd`. |
| `make vet` | `go vet ./...` on both modules — reports suspicious constructs, unused code, and common mistakes. |

## Test coverage

There are no `_test.go` files in either module. This is intentional for the MVP
phase. When tests are added, they should follow Go conventions:

- Unit tests alongside the code they test (e.g. `cli/internal/cmd/add_test.go`)
- Integration tests in a top-level `tests/` directory or as a separate Go
  package

## Running the API locally

For manual validation of the API:

```sh
# Start a containerized Postgres on port 5433
make db

# Create a .env file with required variables
cat > .env <<EOF
ENVY_DB_URL=postgres://envy:envy@localhost:5433/envy?sslmode=disable
ENVY_MASTER_KEY_B64=$(head -c 32 /dev/urandom | base64)
ENVY_ADDR=:8080
EOF

# Build and run the API server
make run-api
```

The API serves on `:8080` by default. Health check: `curl http://localhost:8080/health`.

## CLI workflow for manual testing

```sh
# Build the CLI
make build

# Initialize a project
./bin/envy init --api-url http://localhost:8080 --project my-project --environment development

# Add a variable to the schema
./bin/envy add DATABASE_URL --type string --required --secret

# Login (requires a registered user on the API)
./bin/envy login

# Propose changes
./bin/envy propose "Add DATABASE_URL"

# Approve the proposal
./bin/envy approve 1

# Pull values into .env.local
./bin/envy pull

# Run a command with injected variables
./bin/envy run -- env
```