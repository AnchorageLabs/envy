# docs

This directory contains the canonical specification documents for the ENVY project.

## Contents

| File | Description |
|------|-------------|
| `MODEL.md` | Data model specification: projects, environments, variables, versions |
| `FILES.md` | File format specification: `.envy/schema.json`, `.envy/lock.json`, `.env` output |

> These spec files may be added as the project evolves. This README is a brief placeholder.

## Purpose

These documents define the authoritative contracts that both the `cli` and `api` modules implement. Any ambiguity in behaviour should be resolved by referring to the specifications here.

## Related

- [`cli/`](../cli/README.md) — CLI module
- [`api/`](../api/README.md) — API server module
