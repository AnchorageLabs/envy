# ENVY Repository Files

ENVY keeps committable project metadata and schema separate from local values.
Files under `.envy/` describe where the project lives, what variables exist,
and which published environment versions are pinned. Local values are stored in
`.env.local`, which is gitignored and should not be committed.

This document specifies file shapes only. It does not add or imply any CLI or
API file-loading implementation.

---

## `.envy/config.json`

**Status:** committable.

Repository-local configuration that tells ENVY which backend project and default
environment this checkout is associated with.

Example:

```json
{
  "api_url": "https://api.envy.example.com",
  "project": "payments-api",
  "environment": "development"
}
```

Fields:

| Field | Type | Description |
|---|---|---|
| `api_url` | string | Base URL of the ENVY API used by the CLI for this repository. |
| `project` | string | Project slug or name for the repository. It corresponds to the ENVY `Project`. |
| `environment` | string | Default environment name for local commands, such as `development`, `staging`, or `production`. It corresponds to the ENVY `Environment.name`. |

---

## `.envy/schema.json`

**Status:** committable.

Schema metadata for variables expected by the project. This file mirrors the
metadata side of the `Variable` entity described in `docs/MODEL.md`, but it
contains no values or secrets.

Example:

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
  },
  {
    "key": "LOG_LEVEL",
    "type": "string",
    "required": false,
    "secret": false,
    "default": "info",
    "description": "Application log verbosity.",
    "owner": "backend"
  }
]
```

Shape:

- The top-level value is a JSON array.
- Each array item defines one environment variable.
- Variable values are intentionally absent. Secret values must never be stored
  in this file.

Fields per variable:

| Field | Type | Description |
|---|---|---|
| `key` | string | Environment variable name, such as `DATABASE_URL`. Corresponds to `Variable.key`. |
| `type` | string | Logical value type, such as `string`, `number`, or `boolean`. Corresponds to `Variable.type`. |
| `required` | boolean | Whether the variable must be present for the environment to be valid. Corresponds to `Variable.is_required`. |
| `secret` | boolean | Whether the value is sensitive and must be masked. Corresponds to `Variable.is_secret`. |
| `default` | string, number, boolean, null, or object/array | Optional default metadata for non-secret or non-sensitive defaults. This is metadata only and is not a published environment value. |
| `description` | string | Optional human-readable description of the variable. Corresponds to `Variable.description`. |
| `owner` | string | Optional team, service, or person responsible for the variable. |

---

## `.envy/lock.json`

**Status:** committable.

Lock metadata pinning each environment to a published stable version and a
content checksum. This file contains no values and no secrets. It records enough
metadata for implementations to verify that a fetched environment snapshot
matches the pinned version.

Example:

```json
{
  "project": "payments-api",
  "environments": {
    "development": {
      "version": 42,
      "checksum": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
      "keys": {
        "DATABASE_URL": {
          "type": "string",
          "required": true,
          "secret": true
        },
        "LOG_LEVEL": {
          "type": "string",
          "required": false,
          "secret": false
        }
      }
    },
    "production": {
      "version": 7,
      "checksum": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
      "keys": {
        "DATABASE_URL": {
          "type": "string",
          "required": true,
          "secret": true
        },
        "LOG_LEVEL": {
          "type": "string",
          "required": false,
          "secret": false
        }
      }
    }
  }
}
```

Fields:

| Field | Type | Description |
|---|---|---|
| `project` | string | Project slug or name for the repository. |
| `environments` | object | Map from environment name to lock metadata for that environment. |
| `environments.<name>.version` | integer | Published environment version number. This follows the `<environment>@<number>` version naming described in `docs/MODEL.md`. |
| `environments.<name>.checksum` | string | Deterministic checksum for the pinned environment snapshot. |
| `environments.<name>.keys` | object | Map from variable key to metadata for that variable at the pinned version. Values are not included. |
| `environments.<name>.keys.<key>.type` | string | Logical variable type. |
| `environments.<name>.keys.<key>.required` | boolean | Whether the variable is required. |
| `environments.<name>.keys.<key>.secret` | boolean | Whether the variable value is sensitive. |

Checksum algorithm:

1. Start from the variables in the pinned environment snapshot, including their
   actual values as available to the checksum producer.
2. Sort variables by key alphabetically using bytewise lexicographic order of
   the key strings.
3. For each non-secret variable, produce the per-variable string
   `KEY=VALUE`, where `KEY` is the variable key and `VALUE` is the exact value
   string.
4. For each secret variable, produce the per-variable string
   `KEY=<sha256(VALUE)>`, where `<sha256(VALUE)>` is the lowercase hexadecimal
   SHA-256 digest of the exact secret value string.
5. Join the per-variable strings with a single newline character (`\n`) between
   entries. Do not add an extra trailing newline.
6. Compute the SHA-256 digest of the joined string.
7. Hex-encode the digest in lowercase and prefix it with `sha256:`.

The resulting value is stored as `checksum`, for example
`sha256:<64 lowercase hex characters>`.

---

## `.env.local`

**Status:** gitignored.

Local dotenv file containing values for development on the current machine.
This file may contain secrets and must not be committed.

Example:

```dotenv
# Local development values hydrated by `envy pull`
DATABASE_URL=postgres://envy:envy@localhost:5432/payments
LOG_LEVEL=debug

# Comments and blank lines are allowed
FEATURE_FLAG_PAYMENTS=true
```

Shape:

- Standard `KEY=VALUE` lines are read as local environment values.
- Lines beginning with `#` are comments.
- Blank lines are ignored.
- Values are local machine state. The canonical versioned values live in the
  ENVY backend and are fetched on demand by the CLI.
