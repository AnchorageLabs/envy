# api

The `api` module is a Go HTTP API server for the ENVY project.

## Purpose

The API server stores and serves versioned `.env` values, providing a versioned key-value store that the ENVY CLI queries to fetch environment variable values for a given project and environment.

## Responsibilities

- Store environment variable values with versioning
- Expose HTTP endpoints for the CLI to fetch and push versioned values
- Manage access control and authentication
- Support multiple projects and environments

## Usage

> Source code and build instructions will be added as the module is implemented.

## Related

- [`cli/`](../cli/README.md) — the CLI tool that consumes this API
- [`docs/`](../docs/README.md) — canonical specification documents
