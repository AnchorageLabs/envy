# Contributing to ENVY

## Prerequisites

- Go 1.22 or later
- Docker (for running Postgres locally)
- `make`

## Quick start

```sh
# Clone the repository
git clone https://github.com/AnchorageLabs/envy.git
cd envy

# Build both binaries
make build

# Run static analysis
make vet
```

## Local API setup

The API requires a Postgres database. A Docker-based setup is provided:

```sh
# Start Postgres on port 5433
make db

# Create a .env file with required configuration
cat > .env <<EOF
ENVY_DB_URL=postgres://envy:envy@localhost:5433/envy?sslmode=disable
ENVY_MASTER_KEY_B64=$(head -c 32 /dev/urandom | base64)
ENVY_ADDR=:8080
EOF

# Build and run the API server
make run-api
```

The API serves on `:8080` by default. Verify with:

```sh
curl http://localhost:8080/health
```

## Basic CLI workflow

```sh
# Initialize a project (creates .envy/ directory)
./bin/envy init --api-url http://localhost:8080 --project my-project --environment development

# Add a variable to the local schema
./bin/envy add DATABASE_URL --type string --required --secret

# Login to the API
./bin/envy login

# Propose the schema change
./bin/envy propose "Add DATABASE_URL"

# Approve the proposal (publishes a new stable version)
./bin/envy approve 1

# Pull values into .env.local
./bin/envy pull

# Run a command with environment variables injected
./bin/envy run -- env
```

## Project conventions

- **Two Go modules**: `cli/` and `api/`, both Go 1.22. Run `make build` to build
  both.
- **No tests yet**: validate with `make vet` and `make build`.
- **`.envy/` files are committable**: `.envy/config.json`, `.envy/schema.json`,
  `.envy/lock.json`.
- **`.env.local` is gitignored**: never commit local values or secrets.
- **Credentials** are stored in `~/.envy/credentials` (JSON, permissions 0600).
- **Keep docs consistent**: when changing CLI commands, API routes, config
  shapes, or the data model, update `docs/MODEL.md`, `docs/FILES.md`, and the
  root-level docs accordingly.

## Code review expectations

- All changes must compile (`make build`).
- All changes must pass `go vet` (`make vet`).
- New CLI commands should follow the existing Cobra pattern (options struct,
  `newXxxCommand` factory, `runXxx` implementation).
- New API handlers should follow the existing Chi pattern (method on `Server`,
  JSON request/response helpers).
- Do not add private Anchorage orchestrator implementation details to this
  public repository.