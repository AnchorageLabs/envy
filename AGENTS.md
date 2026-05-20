# AGENTS.md — Guidance for Automated Agents

This file provides repository-specific instructions for automated coding agents working inside the `envy` repository.

## Repository Purpose

`envy` is a CLI tool that manages per-project environment variable files, renders templates, and optionally syncs configuration via git. See `README.md` for the user-facing overview.

## Entry Points

| Artifact | Path |
|---|---|
| CLI entry point | `src/envy/cli.py` |
| Package root | `src/envy/` |
| Test suite | `tests/` |

## File Ownership Boundaries

- **`src/envy/`** — Core implementation. Changes here affect CLI behaviour and must be accompanied by tests.
- **`tests/`** — Unit and integration tests. Every new feature or bug-fix must have corresponding test coverage.
- **`AGENTS.md`, `ARCHITECTURE.md`, `CONTRIBUTING.md`, `TESTING.md`** — Documentation only. Safe to update without touching source.
- **`README.md`** — User-facing documentation. Keep consistent with CLI behaviour.
- **`setup.py` / `pyproject.toml` / `setup.cfg`** — Packaging metadata. Edit only when adding dependencies or changing entry points.

## What Not to Change

- Do **not** alter existing CLI command names, flags, or output formats without a corresponding plan that updates tests and README.
- Do **not** add private orchestrator implementation details to any file in this repository.
- Do **not** commit secrets, tokens, or credentials.

## How to Run Tests

```
PYTHONPATH=src python3 -m unittest discover -s tests
```

All tests must pass before any change is considered complete. See `TESTING.md` for full details.

## Preferred Workflow for Agents

1. Read `ARCHITECTURE.md` to understand the system before planning edits.
2. Read the relevant source file(s) in full before modifying them.
3. Make the smallest safe change that satisfies the plan.
4. Run the test command above and confirm zero failures.
5. Update documentation files if behaviour changes.
