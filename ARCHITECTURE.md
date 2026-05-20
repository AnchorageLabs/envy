# ARCHITECTURE.md — envy System Architecture

## Overview

`envy` is a Python CLI application that helps developers manage environment variable files across projects. It provides commands to initialise configuration, set/get/delete variables, render template files, and optionally synchronise configuration through git.

## Repository Layout

```
envy/
├── src/
│   └── envy/
│       ├── __init__.py
│       └── cli.py          # All CLI logic lives here
├── tests/
│   └── test_cli.py         # Test suite
├── README.md
├── AGENTS.md
├── ARCHITECTURE.md
├── CONTRIBUTING.md
├── TESTING.md
└── setup.py / pyproject.toml
```

## CLI Structure

### Entry Point

The package exposes a single entry point defined in the packaging metadata that calls the `main()` function in `src/envy/cli.py`.

### Command Dispatch

`cli.py` uses Python's `argparse` (or a similar stdlib mechanism) to parse the top-level subcommand and dispatch to the appropriate handler function. The general pattern is:

```
main()
  └── parse_args()
        ├── init      → handle_init()
        ├── set       → handle_set()
        ├── get       → handle_get()
        ├── delete    → handle_delete()
        ├── render    → handle_render()
        └── sync      → handle_sync()   # git boundary
```

(Exact subcommand names are defined in `src/envy/cli.py`; consult that file for the authoritative list.)

## Config File Format

`envy` stores environment variables in a project-local config file (typically `.envy` or similar in the project root). The format is a simple key=value text file, one variable per line:

```
DATABASE_URL=postgres://localhost/mydb
DEBUG=true
SECRET_KEY=changeme
```

The config file path is resolved relative to the current working directory. Consult `cli.py` for the exact filename constant.

## Template Rendering Pipeline

The `render` command reads a template file (e.g., `.env.template`) and substitutes variable placeholders with values from the envy config. The pipeline is:

1. Load config from the envy config file.
2. Read the template file from disk.
3. Replace each `{{VAR_NAME}}` (or equivalent) placeholder with the corresponding config value.
4. Write the rendered output to the target file (e.g., `.env`).

Missing variables cause an error; extra variables in the config that are not referenced in the template are silently ignored.

## Git Sync Boundary

The `sync` command is the only part of `envy` that interacts with git. It shells out to `git` to push or pull the envy config file to/from a remote repository. This is an explicit boundary:

- All other commands are purely local file operations.
- The sync command requires a git remote to be configured.
- Agents should treat the git interaction as an external side-effect and avoid mocking it in unit tests unless specifically testing sync behaviour.

## Key Design Principles

- **Single-file core**: all application logic lives in `src/envy/cli.py` to keep the codebase easy to navigate.
- **No runtime dependencies beyond stdlib** (unless explicitly added to packaging metadata).
- **Fail loudly**: missing config or template variables raise errors rather than silently producing incorrect output.
