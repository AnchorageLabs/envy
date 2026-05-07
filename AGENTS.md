# AGENTS.md — Agent Context for envy

## Repo purpose

`envy` is a small CLI dotfiles manager. It backs up, restores, syncs, and bootstraps development environment files across machines using a git-backed repo, a TOML config file, and an optional Jinja-like template system.

## Files to read first

1. `README.md` — user-facing overview, command reference, config format, and template variables.
2. `src/envy/cli.py` — the entire implementation: argument parsing, subcommand dispatch, config loading, template rendering, and git sync.
3. `tests/test_cli.py` — integration tests that exercise backup, restore, and template rendering end-to-end.
4. `ARCHITECTURE.md` — structural description of the CLI and data flow.
5. `TESTING.md` — exact test command and prerequisites.
6. `CONTRIBUTING.md` — development workflow.

## Code ownership boundaries

| Area | Location | Notes |
|---|---|---|
| CLI logic | `src/envy/cli.py` | All subcommands, config loading, template rendering, git operations |
| Test coverage | `tests/test_cli.py` | Integration tests using temp directories |
| Config schema | `.envy/config.toml` (runtime) | Described in `README.md` and `ARCHITECTURE.md` |
| Bootstrap script | `scripts/bootstrap` (runtime) | User-supplied; executed by `envy bootstrap` |

## Agent-specific constraints

- **Do not change CLI behavior** (argument names, subcommand names, exit codes, output format) without a separate issue and explicit approval.
- **Do not expose private orchestrator implementation details** in any committed file.
- **Base all factual claims** in documentation strictly on what is observed in the source files — do not invent features.
- When editing `src/envy/cli.py`, run `PYTHONPATH=src python3 -m unittest discover -s tests` and confirm all tests pass before considering the change complete.
- The `src/` layout means imports use `envy.cli` — keep the package structure intact.
