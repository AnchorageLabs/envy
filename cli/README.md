# cli

The `cli` module is a Go command-line tool for the ENVY project.

## Purpose

The CLI reads `.envy/schema.json` to understand the expected environment variable schema for a project, fetches the corresponding versioned values from the ENVY API backend, and writes the resolved values to `.env` files on disk.

## Responsibilities

- Parse and validate `.envy/schema.json`
- Authenticate with and query the ENVY API server
- Resolve versioned environment variable values
- Write output to `.env` (or environment-specific variants)
- Manage `.envy/lock.json` to record the exact versions fetched

## Usage

> Source code and build instructions will be added as the module is implemented.

## Related

- [`api/`](../api/README.md) — the HTTP API backend this CLI communicates with
- [`docs/`](../docs/README.md) — canonical specification documents
