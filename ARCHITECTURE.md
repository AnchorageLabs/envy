# ARCHITECTURE.md — envy Architecture

## Overview

`envy` is a single-module Python CLI (`src/envy/cli.py`). There are no external runtime dependencies beyond the Python standard library (Python 3.11+ required for `tomllib`).

## Entry point

```
src/envy/cli.py  →  main()  →  build_parser()  →  args.func(args)
```

`main()` builds an `argparse.ArgumentParser`, parses `argv`, and dispatches to the subcommand handler stored in `args.func`. Any `EnvyError` raised by a handler is caught, printed to stderr, and causes a non-zero exit.

## Subcommand dispatch

Each subcommand is registered in `build_parser()` via `subparsers.add_parser(...)` and bound to a handler with `set_defaults(func=cmd_<name>)`.

| Subcommand | Handler | What it does |
|---|---|---|
| `init` | `cmd_init` | Creates `.envy/`, `home/`, `templates/`, `scripts/` directories and writes a default `config.toml`. Runs `git init` if no `.git` exists. |
| `backup` | `cmd_backup` | Copies files listed in config (plus any `--file` extras) from `$HOME` into `home/` inside the repo. |
| `restore` | `cmd_restore` | Copies files from `home/` (or renders from `templates/`) back to `$HOME`. Refuses to overwrite without `--force`. |
| `status` | `cmd_status` | Diffs each tracked file between the repo/template and `$HOME`. Exits 1 if any file is dirty or missing. |
| `sync` | `cmd_sync` | Optionally runs `git pull --rebase`, then `git add .` + `git commit`, then optionally `git push`. |
| `bootstrap` | `cmd_bootstrap` | Runs `restore`, then executes `scripts/bootstrap` if it exists. |

## Configuration

Config lives at `<repo>/.envy/config.toml` and is parsed with `tomllib` (stdlib, read-only TOML).

```toml
files = [".zshrc", ".gitconfig"]   # list of paths relative to $HOME

[machine]
work_email = "you@company.com"     # arbitrary key/value pairs used in templates
```

`load_settings()` reads this file and returns a frozen `Settings` dataclass containing `repo`, `home`, `config_path`, `files`, and `machine`.

## Template rendering pipeline

1. For each tracked file, `template_path()` checks `<repo>/templates/<name>`.
2. If a template exists, `render_template()` is called with the file text and the current `Settings`.
3. `render_template()` uses a single regex (`TOKEN_RE = re.compile(r"{{\s*([a-zA-Z0-9_.-]+)\s*}}")`) to find `{{ token }}` placeholders and replaces them with values from a lookup table.
4. Built-in variables: `hostname`, `machine` (both resolve to `socket.gethostname()`), `os` (lowercased `platform.system()`), `env.<VAR>` (environment variable lookup), and `machine.<key>` (values from the `[machine]` config table).
5. Unknown tokens are left unchanged.
6. If no template exists, the stored file in `home/` is used verbatim.

## Git sync boundary

`cmd_sync` is the only function that calls git. It:
- Requires a `.git` directory (enforced by `ensure_git_repo()`).
- Optionally pulls with rebase before staging.
- Stages all changes (`git add .`).
- Commits only if `git diff --cached --quiet` returns non-zero (i.e., there are staged changes).
- Optionally pushes after committing.

All other commands are purely local filesystem operations.

## Path helpers

| Function | Purpose |
|---|---|
| `resolve_home_path(home, name)` | Resolves a config file name to an absolute path under `$HOME` |
| `repo_home_path(repo, home, source)` | Maps an absolute home path to its mirror under `<repo>/home/` |
| `stored_path(repo, name)` | Returns `<repo>/home/<name>` (strips leading `/` for absolute names) |
| `template_path(repo, name)` | Returns `<repo>/templates/<name>` (strips leading `/` for absolute names) |
