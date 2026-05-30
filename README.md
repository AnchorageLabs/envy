# ENVY

Version control system for environment variables.

ENVY helps dev teams keep `.env` files synced, reviewed, validated and
recoverable without copy-pasting secrets across password managers.

## Core idea

ENVY separates **schema** (what variables exist) from **values** (the actual
secrets):

- `.envy/schema.json` — committable. Lists each variable with type,
  required/optional, secret/public, default, description, owner.
- `.envy/lock.json` — committable. Pins the current stable version of each
  environment plus a checksum. Holds no secrets.
- `.env.local` — gitignored. Local development values, hydrated by `envy pull`.

The values themselves are versioned in the ENVY backend (API + storage) and
fetched on demand via the CLI.

## MVP

- `cli/` — the `envy` CLI, in Go.
- `api/` — backend service, in Go.
- `docs/` — model, file format and design notes.

No web UI in the MVP. CLI and API only.

## Status

Greenfield. See open issues for the build plan.
