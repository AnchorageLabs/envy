# AGENTS.md — ENVY

This file is for automated agents (coders, planners, reviewers) working on the
ENVY repository. It supplements the project README with agent-specific
conventions.

## Repository purpose

ENVY is a version control system for environment variables. It separates schema
(what variables exist) from values (the actual secrets). The CLI manages local
project state under `.envy/`, and the API stores versioned values in a Postgres
database.

## Module layout

Two Go modules, both using Go 1.22:

- `cli/` — the `envy` CLI binary (cmd/envy/main.go). Cobra-based command tree.
- `api/` — the `envyd` API server binary (cmd/envyd/main.go). Chi-based HTTP
  server backed by pgxpool.

## Key files agents should read before editing

| File | Purpose |
|---|---|
| `README.md` | Project overview and core idea |
| `docs/MODEL.md` | Canonical data model (entities, fields, relationships) |
| `docs/FILES.md` | On-disk file shapes (.envy/config.json, schema.json, lock.json, .env.local) |
| `MAP.md` | Cartographer-generated module map and invariants |
| `Makefile` | Build, vet, and local-run targets |

## Agent-specific conventions

- **No unit tests exist yet.** There are no `_test.go` files in either module.
  Validate changes with `make build` (builds both binaries) and `make vet` (go
  vet on both modules).
- **Do not add test infrastructure** (test runners, CI configs, coverage tools)
  unless the issue explicitly requests it.
- **The `.envy/` directory** holds committable project config
  (`.envy/config.json`, `.envy/schema.json`, `.envy/lock.json`).
- **`.env.local`** is gitignored and must never be committed.
- **Credentials** are stored in `~/.envy/credentials` (JSON map of API URL to
  bearer token, permissions 0600).
- **Do not reference private Anchorage orchestrator internals** in any doc or
  comment. This is a public repository.

## Go module references

- `github.com/spf13/cobra` — CLI framework
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/jackc/pgx/v5` — Postgres driver and pool
- `golang.org/x/crypto` — bcrypt for password hashing
- `golang.org/x/term` — terminal utilities for password prompts and color