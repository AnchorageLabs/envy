# CONTRIBUTING.md — Contributing to envy

## Prerequisites

- Python 3.11+
- `git` on your `PATH`

## Set up the development environment

```bash
git clone <repo-url>
cd envy
python3 -m pip install -e .
```

The editable install makes the `envy` command available and puts `src/envy` on the import path. No other dependencies are required.

## Run the tests

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```

All tests must pass before submitting a change. See `TESTING.md` for details on what the tests cover.

## Making changes

1. Create a feature branch: `git checkout -b feature/<short-description>`.
2. Edit files under `src/envy/` for CLI logic or `tests/` for test coverage.
3. Run the test suite and confirm it passes.
4. Commit with a clear message describing the change.
5. Open a pull request against `main`.

## Agent contributors

Automated agents should read `AGENTS.md` before planning or editing any code. Key constraints:

- Do not change CLI behavior (argument names, subcommand names, exit codes) without an explicit issue.
- Base all documentation claims strictly on the source — do not invent features.
- Run the test suite after every code change and include the result in the change summary.

## Code style

- The codebase uses standard-library-only imports and type annotations.
- Follow the existing patterns in `src/envy/cli.py` for new subcommands: register in `build_parser()`, implement as `cmd_<name>(args)`, raise `EnvyError` for user-facing errors.
