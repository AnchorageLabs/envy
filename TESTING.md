# TESTING.md — Running Tests for envy

## Test command

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```

`PYTHONPATH=src` is required because the package lives under `src/envy/` and is not installed into the active Python environment during development (unless you have run `pip install -e .`).

## What the tests validate

`tests/test_cli.py` contains an integration test (`EnvyCliTest.test_backup_restore_and_template`) that:

1. Creates isolated temporary directories for the repo and home.
2. Runs `cmd_init` to set up the repo structure and default config.
3. Writes a `.zshrc` to the temp home directory.
4. Runs `cmd_backup` and asserts the file is copied into `<repo>/home/.zshrc`.
5. Modifies the home `.zshrc`, then runs `cmd_restore --force` and asserts the original content is restored.
6. Calls `render_template` with a template string containing `{{ machine.email }}`, `{{ os }}`, and `{{ env.ENVY_EMPTY }}` and asserts the output contains the expected substituted fragments.

## Environment prerequisites

- Python 3.11 or later (required for `tomllib` in the standard library).
- No third-party packages are required to run the tests.
- `git` must be available on `PATH` if you exercise `cmd_init` or `cmd_sync` in a context that calls `git init` or git operations (the test suite calls `cmd_init`, which runs `git init` in the temp repo).

## Running a single test

```bash
PYTHONPATH=src python3 -m unittest tests.test_cli.EnvyCliTest.test_backup_restore_and_template
```
