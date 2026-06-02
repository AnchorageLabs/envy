# ENVY Data Model

This document is the canonical specification of the ENVY data model. It
describes every entity, its fields and types, the relationships between
entities, and the conventions ENVY uses for versioning environments.

ENVY separates **schema** (what variables exist) from **values** (the actual
secrets). The entities below model both halves: the schema/working-draft side
(`Variable`) and the versioned, immutable value snapshots
(`EnvironmentVersion`). Types use logical, DB-dialect-agnostic names
(`uuid`, `text`, `timestamp`, `jsonb`, `boolean`, `integer`) so they map
cleanly onto any backing store.

---

## Entities

### User

An authenticated account that can own projects and propose or approve changes.

| Field        | Type        | Description                                          |
|--------------|-------------|------------------------------------------------------|
| `id`         | `uuid`      | Primary key.                                         |
| `email`      | `text`      | Unique login identity for the user.                  |
| `name`       | `text`      | Human-readable display name.                         |
| `created_at` | `timestamp` | When the user account was created.                   |
| `updated_at` | `timestamp` | When the user record was last modified.              |

**Relationships**

- `User` 1—* `Project` (a user owns many projects; `Project.owner_id` → `User.id`).
- `User` 1—* `ChangeProposal` (a user authors many proposals; `ChangeProposal.author_id` → `User.id`).
- `User` 1—* `AuditLog` (a user is the actor on many audit entries; `AuditLog.actor_id` → `User.id`).

---

### Project

A top-level container that groups related environments (e.g. one repository or
one product).

| Field        | Type        | Description                                          |
|--------------|-------------|------------------------------------------------------|
| `id`         | `uuid`      | Primary key.                                         |
| `owner_id`   | `uuid`      | FK → `User.id`. The user who owns the project.       |
| `name`       | `text`      | Project name (unique within an owner).               |
| `slug`       | `text`      | URL/CLI-friendly identifier.                         |
| `created_at` | `timestamp` | When the project was created.                        |
| `updated_at` | `timestamp` | When the project was last modified.                  |

**Relationships**

- `Project` *—1 `User` (owner; `owner_id` → `User.id`).
- `Project` 1—* `Environment` (a project has many environments; `Environment.project_id` → `Project.id`).

---

### Environment

A named deployment context inside a project (e.g. `development`, `staging`,
`production`). Holds the working set of variables and points at a stable
published version.

| Field               | Type        | Description                                                        |
|---------------------|-------------|--------------------------------------------------------------------|
| `id`                | `uuid`      | Primary key.                                                       |
| `project_id`        | `uuid`      | FK → `Project.id`. Owning project.                                 |
| `name`              | `text`      | Environment name (unique within a project), e.g. `production`.     |
| `stable_version_id` | `uuid`      | Nullable FK → `EnvironmentVersion.id`. Currently published version. |
| `created_at`        | `timestamp` | When the environment was created.                                  |
| `updated_at`        | `timestamp` | When the environment was last modified.                            |

**Relationships**

- `Environment` *—1 `Project` (`project_id` → `Project.id`).
- `Environment` 1—* `Variable` (the mutable working draft; `Variable.environment_id` → `Environment.id`).
- `Environment` 1—* `EnvironmentVersion` (immutable snapshots; `EnvironmentVersion.environment_id` → `Environment.id`).
- `Environment` *—1 `EnvironmentVersion` via the nullable `stable_version_id` FK (the currently stable published version).

---

### Variable

A single environment variable in the **mutable working draft** of an
environment. Variables are edited freely; they are snapshotted into an
`EnvironmentVersion` when published.

| Field            | Type        | Description                                                        |
|------------------|-------------|--------------------------------------------------------------------|
| `id`             | `uuid`      | Primary key.                                                       |
| `environment_id` | `uuid`      | FK → `Environment.id`. Owning environment.                         |
| `key`            | `text`      | Variable name (unique within an environment), e.g. `DATABASE_URL`. |
| `value`          | `text`      | The variable value (may be encrypted at rest for secrets).         |
| `type`           | `text`      | Logical type of the value (e.g. `string`, `number`, `boolean`).    |
| `is_secret`      | `boolean`   | Whether the value is sensitive and must be masked.                 |
| `is_required`    | `boolean`   | Whether the variable must be present for the environment to be valid. |
| `description`    | `text`      | Optional one-line description of the variable.                     |
| `created_at`     | `timestamp` | When the variable was created.                                     |
| `updated_at`     | `timestamp` | When the variable was last modified.                               |

**Relationships**

- `Variable` *—1 `Environment` (`environment_id` → `Environment.id`).

---

### EnvironmentVersion

An **immutable** point-in-time snapshot of an environment's variables. Once
created, an `EnvironmentVersion` is never edited; publishing a change creates a
new version with an incremented number.

| Field            | Type        | Description                                                            |
|------------------|-------------|------------------------------------------------------------------------|
| `id`             | `uuid`      | Primary key.                                                           |
| `environment_id` | `uuid`      | FK → `Environment.id`. The environment this version snapshots.         |
| `number`         | `integer`  | Monotonically increasing version number within the environment.        |
| `snapshot`       | `jsonb`     | Frozen copy of the variables (keys, values, types, flags) at publish.  |
| `checksum`       | `text`      | Content checksum of the snapshot for integrity verification.           |
| `created_by`     | `uuid`      | FK → `User.id`. The user who published this version.                   |
| `created_at`     | `timestamp` | When the version was created (immutable thereafter).                   |

**Relationships**

- `EnvironmentVersion` *—1 `Environment` (`environment_id` → `Environment.id`).
- `EnvironmentVersion` *—1 `User` (publisher; `created_by` → `User.id`).
- An `Environment` may point at one `EnvironmentVersion` as its stable version (`Environment.stable_version_id`).

---

### ChangeProposal

A proposed change to an environment's variables, awaiting review/approval
before it becomes a new `EnvironmentVersion`.

| Field             | Type        | Description                                                          |
|-------------------|-------------|----------------------------------------------------------------------|
| `id`              | `uuid`      | Primary key.                                                         |
| `environment_id`  | `uuid`      | FK → `Environment.id`. Target environment of the proposed change.    |
| `author_id`       | `uuid`      | FK → `User.id`. The user who created the proposal.                   |
| `base_version_id` | `uuid`      | Nullable FK → `EnvironmentVersion.id`. Version the proposal builds on. |
| `status`          | `text`      | Proposal state, e.g. `open`, `approved`, `rejected`, `merged`.       |
| `diff`            | `jsonb`     | Proposed additions/updates/removals of variables.                    |
| `created_at`      | `timestamp` | When the proposal was created.                                       |
| `updated_at`      | `timestamp` | When the proposal was last modified.                                 |

**Relationships**

- `ChangeProposal` *—1 `Environment` (`environment_id` → `Environment.id`).
- `ChangeProposal` *—1 `User` (author; `author_id` → `User.id`).
- `ChangeProposal` *—1 `EnvironmentVersion` (optional base; `base_version_id` → `EnvironmentVersion.id`).
- When merged, a `ChangeProposal` produces a new `EnvironmentVersion`.

---

### AuditLog

An append-only record of significant actions performed in ENVY (pulls,
publishes, proposal approvals, etc.).

| Field         | Type        | Description                                                       |
|---------------|-------------|-------------------------------------------------------------------|
| `id`          | `uuid`      | Primary key.                                                      |
| `actor_id`    | `uuid`      | FK → `User.id`. The user who performed the action.                |
| `action`      | `text`      | The action performed, e.g. `environment.published`.              |
| `entity_type` | `text`      | Type of entity affected, e.g. `Environment`, `ChangeProposal`.    |
| `entity_id`   | `uuid`      | Identifier of the affected entity.                                |
| `metadata`    | `jsonb`     | Additional structured context for the action.                     |
| `created_at`  | `timestamp` | When the action occurred.                                         |

**Relationships**

- `AuditLog` *—1 `User` (actor; `actor_id` → `User.id`).
- `AuditLog` references arbitrary entities via the polymorphic `entity_type` / `entity_id` pair.

---

## Relationship diagram

```
                          +------------------+
                          |       User       |
                          +------------------+
                            | 1     | 1   | 1
            owner_id        |       |     | author_id      actor_id
            +---------------+       |     +-----------------------------+
            |                      | created_by                        |
            v 1                    |                                   v *
     +-------------+               |                          +----------------+
     |   Project   |               |                          |    AuditLog    |
     +-------------+               |                          +----------------+
            | 1                    |
 project_id |                      |
            v *                    |
     +-----------------+           |
     |   Environment   |<----------+----- (publisher) ----------------+
     +-----------------+                                              |
        | 1   | 1   | 1                                                |
        |     |     | stable_version_id (nullable)                    |
        |     |     +--------------------------------+                |
        |     |                                      v 1              |
        |     | environment_id                +---------------------+ |
        |     +------------------------------>| EnvironmentVersion  |-+
        |                                  *  +---------------------+
        | environment_id                          ^ 1
        v *                                       | base_version_id (nullable)
  +-------------+                          +---------------------+
  |  Variable   |        environment_id    |   ChangeProposal    |
  +-------------+   *<---------------------- +---------------------+
   (mutable                                   * (author_id -> User)
    working draft)
```

Legend: `1` and `*` denote the cardinality endpoints; `1—*` means "one to many".

Cardinality summary:

- `User` 1—* `Project`
- `User` 1—* `ChangeProposal`
- `User` 1—* `EnvironmentVersion` (as publisher)
- `User` 1—* `AuditLog`
- `Project` 1—* `Environment`
- `Environment` 1—* `Variable`
- `Environment` 1—* `EnvironmentVersion`
- `Environment` *—1 `EnvironmentVersion` (via nullable `stable_version_id`)
- `Environment` 1—* `ChangeProposal`
- `ChangeProposal` *—1 `EnvironmentVersion` (via nullable `base_version_id`)

---

## Version naming

Environment versions are referenced using the convention:

```
<environment>@<number>
```

where `<environment>` is the `Environment.name` and `<number>` is the
`EnvironmentVersion.number` (a monotonically increasing integer scoped to that
environment). The number starts at `1` for the first published version and
increments by one on each subsequent publish.

Examples:

- `development@42` — the 42nd published version of the `development` environment.
- `production@91` — the 91st published version of the `production` environment.

The stable/published version of an environment is the one referenced by
`Environment.stable_version_id`.

---

## Mutability

- **`EnvironmentVersion` is immutable.** Once a version is created it is never
  edited or deleted. Publishing a change always creates a *new* version with the
  next `number`; history is therefore append-only and fully recoverable.
- **`Variable` is the mutable working draft.** Variables attached to an
  `Environment` are edited freely as the active draft. Publishing the draft
  snapshots the current variables into a new immutable `EnvironmentVersion`.

All other entities (`User`, `Project`, `Environment`, `ChangeProposal`) are
mutable records, while `AuditLog` is append-only by convention.
