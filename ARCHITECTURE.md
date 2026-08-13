# ENVY Architecture

## CLI structure

The `envy` CLI is built with [Cobra](https://github.com/spf13/cobra). The root
command is defined in `cli/internal/cmd/root.go` and registers 11 subcommands:

| Command | File | Purpose |
|---|---|---|
| `init` | `cli/internal/cmd/init.go` | Scaffold `.envy/` directory, config, schema, and `.env.local` |
| `login` | `cli/internal/cmd/login.go` | Authenticate with the ENVY API and store a bearer token |
| `add` | `cli/internal/cmd/add.go` | Add or update a variable in the local schema |
| `diff` | `cli/internal/cmd/diff.go` | Compare local schema with the remote draft schema |
| `propose` | `cli/internal/cmd/propose.go` | Propose schema and value changes to the bound environment |
| `approve` | `cli/internal/cmd/approve.go` | Approve a pending proposal and publish a new stable version |
| `pull` | `cli/internal/cmd/pull.go` | Fetch stable version values into `.env.local` and update the lockfile |
| `rollback` | `cli/internal/cmd/rollback.go` | Re-point an environment's stable version to a previous version |
| `export` | `cli/internal/cmd/export.go` | Rewrite `.env.local` (or stdout) from the synced lockfile state |
| `run` | `cli/internal/cmd/run.go` | Run a command with resolved environment variables injected |
| `verify` | `cli/internal/cmd/verify.go` | Validate environment health as a CI gate |

**Persistent pre-run** (`PersistentPreRunE` in root.go): resolves the API URL
(from `--api-url` flag, `.envy/config.json`, or `ENVY_API_URL` env var) and
loads the auth token from `~/.envy/credentials`. The `init` and `add` commands
skip this pre-run because they operate locally.

**Exit codes**: `diff`, `verify`, `export`, and `run` use custom exit-error
types (`diffExitError`, `exitError`, `runExitError`, `exportExitError`) that
implement `ExitCode() int` for script-friendly exit semantics.

## API server structure

The `envyd` API server is built with [Chi](https://github.com/go-chi/chi) and
[pgxpool](https://github.com/jackc/pgx). The router is constructed in
`api/internal/server/server.go`.

**Routes**:

| Method | Path | Handler | Auth |
|---|---|---|---|
| GET | `/health` | `healthHandler` | No |
| POST | `/auth/register` | `registerHandler` | No |
| POST | `/auth/login` | `loginHandler` | No |
| POST | `/auth/logout` | `logoutHandler` | Yes |
| POST | `/projects` | `createProjectHandler` | Yes |
| GET | `/projects` | `listProjectsHandler` | Yes |
| GET | `/projects/{slug}` | `getProjectHandler` | Yes |
| PATCH | `/projects/{slug}` | `updateProjectHandler` | Yes |
| DELETE | `/projects/{slug}` | `deleteProjectHandler` | Yes |

**Auth middleware** (`api/internal/server/middleware/auth.go`): extracts a
bearer token from the `Authorization` header, hashes it with SHA-256, looks up
the hash in the `api_tokens` table, and injects the authenticated user into the
request context via `authcontext`.

**Startup** (`api/cmd/envyd/main.go`): loads config from env vars
(`ENVY_ADDR`, `ENVY_DB_URL`, `ENVY_MASTER_KEY_B64`), opens the database pool,
runs migrations, initializes envelope crypto, and serves HTTP.

## Config format

`.envy/config.json` — committable, repository-local configuration:

```json
{
  "api_url": "https://api.envy.example.com",
  "project": "payments-api",
  "environment": "development"
}
```

Loaded by `config.LoadProjectConfig()` which walks upward from `cwd` looking
for `.envy/config.json`. Fields: `api_url` (string), `project` (string),
`environment` (string).

## Schema format

`.envy/schema.json` — committable, describes expected variables with no values:

```json
[
  {
    "key": "DATABASE_URL",
    "type": "string",
    "required": true,
    "secret": true,
    "default": null,
    "description": "Connection string for the application database.",
    "owner": "platform"
  }
]
```

Parsed by `schema.Load()` / `schema.Parse()`. Supports two shapes: a top-level
JSON array (canonical) or an object with a `fields` array (legacy). Each field
has `key`, `type` (string/boolean/number/enum), `required`, `secret`,
`deprecated`, and optional `enum` choices.

## Lockfile format

`.envy/lock.json` — committable, pins each environment to a published version:

```json
{
  "project": "payments-api",
  "environments": {
    "development": {
      "version": 42,
      "checksum": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
      "keys": {
        "DATABASE_URL": { "type": "string", "required": true, "secret": true }
      }
    }
  }
}
```

Written by `pull` and `export`. The checksum algorithm is defined in
`docs/FILES.md`: sorted KEY=VALUE lines, secret values replaced by
`KEY=<sha256(VALUE)>`, joined with newlines, SHA-256 digested, prefixed with
`sha256:`.

## Data model

Six entities as specified in `docs/MODEL.md`:

- **User** — authenticated account (id, email, name, timestamps)
- **Project** — top-level container (id, slug, name, owner_user_id)
- **Environment** — named deployment context (id, project_id, name,
  stable_version_id)
- **Variable** — mutable working draft (id, environment_id, key, type,
  required, secret, default_value, description, owner_user_id, deprecated)
- **EnvironmentVersion** — immutable snapshot (id, environment_id, number,
  snapshot jsonb, checksum, created_by)
- **ChangeProposal** — proposed change awaiting approval (id, environment_id,
  author_id, base_version_id, status, diff jsonb)
- **AuditLog** — append-only action record (id, actor_id, action, entity_type,
  entity_id, metadata jsonb)

## Git sync boundary

- **Committable**: `.envy/config.json`, `.envy/schema.json`, `.envy/lock.json`
- **Gitignored**: `.env.local` (contains local values, may include secrets)

Credentials live outside the repo in `~/.envy/credentials` (permissions 0600).

## Crypto

Envelope encryption in `api/internal/crypto/envelope.go`:

- **Master key**: 32-byte AES-256 key loaded from `ENVY_MASTER_KEY_B64`
  (base64-encoded), set at API startup via `InitFromEnv()`.
- **Per-value data key**: random 32-byte AES-256 key generated for each
  `Encrypt` call, wrapped with the master key using AES-256-GCM.
- **Frame format**: version byte (1) + key nonce (12) + wrapped data key (48) +
  value nonce (12) + value ciphertext (variable + 16-byte GCM tag).
- **Thread safety**: master key is held in a `sync.RWMutex`-guarded package
  variable; `Encrypt`/`Decrypt` snapshot the key under read lock.