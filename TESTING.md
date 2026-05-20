# TESTING.md — Running the envy Test Suite

## Quick Start

Run all tests from the repository root:

```
PYTHONPATH=src python3 -m unittest discover -s tests
```

`PYTHONPATH=src` ensures the `envy` package under `src/` is importable without a full install.

## What the Test Suite Validates

The tests in `tests/test_cli.py` cover:

- **CLI command dispatch** — each subcommand routes to the correct handler.
- **Config read/write** — setting, getting, and deleting variables persists correctly to the config file.
- **Template rendering** — placeholders in template files are replaced with config values; missing variables raise errors.
- **Edge cases** — empty config, unknown variables, malformed input.

Git sync behaviour may be tested with mocked subprocess calls to avoid requiring a real git remote.

## Prerequisites

- Python 3.8 or later.
- No third-party packages are required to run the tests (stdlib only).
- The repository must be checked out with the `src/` directory present.

## Running a Single Test

```
PYTHONPATH=src python3 -m unittest tests.test_cli.TestClassName.test_method_name
```

## Continuous Integration

If a CI pipeline is configured, it runs the same command:

```
PYTHONPATH=src python3 -m unittest discover -s tests
```

All tests must pass before merging any change.

## Adding New Tests

1. Add test methods to `tests/test_cli.py` (or a new file under `tests/`).
2. Follow the existing naming convention: `test_<behaviour_under_test>`.
3. Use `unittest.mock` to isolate filesystem and subprocess side-effects.
4. Run the full suite to confirm no regressions.
