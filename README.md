# envy

`envy` is a small CLI dotfiles manager for backing up, restoring, syncing, and bootstrapping development environment files across machines.

It is intended as a lightweight alternative to full dotfiles managers: keep files in git, restore them to `$HOME`, and render small machine-specific templates when needed.

## Install locally

```bash
python3 -m pip install -e .
```

## Start a dotfiles repo

```bash
envy init
envy backup --file .zshrc --file .gitconfig
envy sync -m "initial dotfiles"
```

Tracked files live in `home/`. Configuration lives in `.envy/config.toml`.

## Restore on a new machine

```bash
git clone git@github.com:YOUR_USER/envy.git ~/.dotfiles
cd ~/.dotfiles
python3 -m pip install -e .
envy restore --force
```

### Backup on overwrite

When `--force` is used, any file or directory that already exists at the target
location is **automatically backed up** before being overwritten. Backups are
written to:

```
.envy/backups/<timestamp>/<relative-path-from-home>
```

All files overwritten in a single `envy restore --force` run share the same
`<timestamp>` directory (format: `YYYYMMDDTHHmmSS`), making it easy to roll
back an entire restore in one step.

Example output:

```
backed up /home/user/.zshrc -> /home/user/.dotfiles/.envy/backups/20240101T120000/.zshrc
restored /home/user/.zshrc
```

No backup is created when `--dry-run` is specified, or when the target file
does not yet exist.

## Templates

Place templates under `templates/` using the same relative path as the home file. For example:

```text
templates/.gitconfig
```

Available variables:

```text
{{ hostname }}
{{ machine }}
{{ os }}
{{ env.VAR_NAME }}
{{ machine.work_email }}
```

Machine values come from `.envy/config.toml`:

```toml
files = [".gitconfig"]

[machine]
work_email = "you@company.com"
```

> **Note:** Each entry in the `files` list must be unique. Duplicate paths are
> rejected immediately with an error when the config is loaded.

## Commands

```bash
envy init
envy backup
envy restore --force
envy status
envy sync --pull --push -m "sync dotfiles"
envy bootstrap --force
envy hook <shell>
envy install-hooks
```

If `scripts/bootstrap` exists, `envy bootstrap` restores files and then executes it.

### Bootstrap dry-run

`envy bootstrap --dry-run` shows what would happen without making any changes
or executing any scripts. The output depends on the state of `scripts/bootstrap`:

| Condition | Output |
|---|---|
| `scripts/bootstrap` not found | `[dry-run] scripts/bootstrap not found; nothing to run` |
| Script exists, **not** executable | `[dry-run] <path> exists but is not executable` |
| Script exists and is executable | `[dry-run] would run <path> (executable)` |

No subprocess is ever invoked during a dry-run.

## Automatic .env Synchronization

### Shell Hooks

You can automatically run `envy status` (and optionally `envy restore`) when entering a directory containing an `.envy/` folder by adding a shell hook to your shell profile.

Generate the hook code for your shell:

```bash
envy hook zsh   # for zsh
envy hook bash  # for bash
envy hook fish  # for fish
```

Add the output to your `~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish` as appropriate.

### Git Hooks

To automatically run `envy restore` after git operations (such as `git pull` or branch switch), install git hooks:

```bash
envy install-hooks
```

This installs `post-merge` and `post-checkout` hooks in `.git/hooks/` that run `envy restore` if an `.envy/` directory is present. The installation is idempotent and safe to run multiple times.

### Checking Hook Installation

The CLI may warn you if hooks are not installed, to help ensure your `.env` files are always up-to-date.

## Development

Run the unit tests locally without installing the package:

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```

Run a compile check on the CLI module:

```bash
python3 -m py_compile src/envy/cli.py
```
