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

## Commands

```bash
envy init
envy backup
envy restore --force
envy status
envy sync --pull --push -m "sync dotfiles"
envy bootstrap --force
```

If `scripts/bootstrap` exists, `envy bootstrap` restores files and then executes it.

## Development

Run the unit tests locally without installing the package:

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```

Run a compile check on the CLI module:

```bash
python3 -m py_compile src/envy/cli.py
```
