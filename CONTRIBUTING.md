# CONTRIBUTING.md — Contributing to envy

Thank you for contributing to `envy`. This document describes the minimal workflow for human and automated contributors.

## Setting Up the Development Environment

1. **Clone the repository**

   ```
   git clone <repo-url>
   cd envy
   ```

2. **Create a virtual environment** (recommended)

   ```
   python3 -m venv .venv
   source .venv/bin/activate
   ```

3. **Install the package in editable mode**

   ```
   pip install -e .
   ```

   This makes the `envy` command available in your shell and keeps `src/envy/` as the live source.

## Running Tests

```
PYTHONPATH=src python3 -m unittest discover -s tests
```

All tests must pass before submitting a change. See `TESTING.md` for details.

## Making Changes

1. Create a feature branch: `git checkout -b feature/your-description`
2. Make the smallest change that satisfies the requirement.
3. Add or update tests in `tests/test_cli.py` to cover the change.
4. Run the full test suite and confirm zero failures.
5. Update `README.md` and/or documentation files if user-visible behaviour changes.
6. Open a pull request against the main branch.

## Code Style

- Follow PEP 8.
- Keep functions small and focused.
- Prefer stdlib over third-party dependencies.
- Add docstrings to public functions.

## Commit Messages

Use short, imperative-mood subject lines, e.g.:

```
Add --force flag to delete command
Fix template rendering for nested placeholders
```

## What Not to Do

- Do not commit secrets, tokens, or credentials.
- Do not change CLI command names or flags without updating tests and README.
- Do not add private orchestrator implementation details to this repository.

## Questions

Open an issue or start a discussion in the repository if you are unsure about the right approach before making a large change.
