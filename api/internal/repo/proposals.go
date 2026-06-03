package repo

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "strings"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

const (
    ProposalStatusPending  = "pending"
    ProposalStatusApproved = "approved"
    ProposalStatusRejected = "rejected"
)

var ErrInvalidProposalChange = errors.New("invalid proposal change")

// Proposal is the JSON/API representation of a proposal row.
type Proposal struct {
    ID               string           `json:"id"`
    ProjectID        string           `json:"project_id"`
    EnvironmentID    string           `json:"environment_id"`
    Message          string           `json:"message"`
    Changes          []ProposalChange `json:"changes"`
    Status           string           `json:"status"`
    CreatedByUserID  string           `json:"created_by_user_id"`
    ResolvedByUserID *string          `json:"resolved_by_user_id"`
    ResolvedAt       *string          `json:"resolved_at"`
    CreatedAt        string           `json:"created_at"`
    UpdatedAt        string           `json:"updated_at"`
}

// ProposalChange describes one draft variable operation in a proposal.
type ProposalChange struct {
    Op          string  `json:"op"`
    Key         string  `json:"key,omitempty"`
    To          string  `json:"to,omitempty"`
    Type        string  `json:"type,omitempty"`
    Required    *bool   `json:"required,omitempty"`
    Secret      *bool   `json:"secret,omitempty"`
    Default     *string `json:"default,omitempty"`
    Description *string `json:"description,omitempty"`
    Owner       *string `json:"owner,omitempty"`
    Deprecated  *bool   `json:"deprecated,omitempty"`
}

// ProposalStore provides pgx-backed proposal queries and resolution workflows.
type ProposalStore struct {
    pool *pgxpool.Pool
}

func NewProposalStore(pool *pgxpool.Pool) *ProposalStore {
    return &ProposalStore{pool: pool}
}

func (s *ProposalStore) CreateProposal(ctx context.Context, projectID, environmentID, actorUserID, message string, changes []ProposalChange) (*Proposal, error) {
    message = strings.TrimSpace(message)
    if message == "" || !validProposalChanges(changes) {
        return nil, ErrInvalidProposalChange
    }

    changesJSON, err := json.Marshal(changes)
    if err != nil {
        return nil, err
    }

    return scanProposal(s.pool.QueryRow(ctx, `
        insert into proposals (project_id, environment_id, message, changes, status, created_by_user_id)
        values ($1::uuid, $2::uuid, $3, $4::jsonb, 'pending', $5::uuid)
        returning id::text, project_id::text, environment_id::text, message, changes::text, status,
                  created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
                  created_at::text, updated_at::text
    `, projectID, environmentID, message, changesJSON, actorUserID))
}

func (s *ProposalStore) ListProposals(ctx context.Context, projectID, environmentID, status string) ([]Proposal, error) {
    status = strings.TrimSpace(status)
    if status != "" && !validProposalStatus(status) {
        return nil, ErrInvalidProposalChange
    }

    var rows pgx.Rows
    var err error
    if status == "" {
        rows, err = s.pool.Query(ctx, `
            select id::text, project_id::text, environment_id::text, message, changes::text, status,
                   created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
                   created_at::text, updated_at::text
            from proposals
            where project_id = $1::uuid and environment_id = $2::uuid
            order by created_at desc, id desc
        `, projectID, environmentID)
    } else {
        rows, err = s.pool.Query(ctx, `
            select id::text, project_id::text, environment_id::text, message, changes::text, status,
                   created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
                   created_at::text, updated_at::text
            from proposals
            where project_id = $1::uuid and environment_id = $2::uuid and status = $3
            order by created_at desc, id desc
        `, projectID, environmentID, status)
    }
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    proposals := make([]Proposal, 0)
    for rows.Next() {
        proposal, err := scanProposal(rows)
        if err != nil {
            return nil, err
        }
        proposals = append(proposals, *proposal)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    return proposals, nil
}

func (s *ProposalStore) GetProposal(ctx context.Context, projectID, environmentID, proposalID string) (*Proposal, error) {
    proposal, err := scanProposal(s.pool.QueryRow(ctx, `
        select id::text, project_id::text, environment_id::text, message, changes::text, status,
               created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
               created_at::text, updated_at::text
        from proposals
        where id = $1::uuid and project_id = $2::uuid and environment_id = $3::uuid
    `, proposalID, projectID, environmentID))
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    return proposal, nil
}

func (s *ProposalStore) ApproveProposal(ctx context.Context, projectID, environmentID, proposalID, actorUserID string) (*Proposal, *EnvironmentVersion, error) {
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadWrite})
    if err != nil {
        return nil, nil, err
    }
    defer func() { _ = tx.Rollback(ctx) }()

    if err := lockEnvironment(ctx, tx, projectID, environmentID); err != nil {
        return nil, nil, err
    }

    proposal, err := scanProposal(tx.QueryRow(ctx, `
        select id::text, project_id::text, environment_id::text, message, changes::text, status,
               created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
               created_at::text, updated_at::text
        from proposals
        where id = $1::uuid and project_id = $2::uuid and environment_id = $3::uuid
        for update
    `, proposalID, projectID, environmentID))
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil, ErrNotFound
    }
    if err != nil {
        return nil, nil, err
    }
    if proposal.Status != ProposalStatusPending {
        return nil, nil, ErrConflict
    }
    if !validProposalChanges(proposal.Changes) {
        return nil, nil, ErrInvalidProposalChange
    }

    for _, change := range proposal.Changes {
        if err := applyProposalChange(ctx, tx, environmentID, change); err != nil {
            return nil, nil, err
        }
    }

    version, err := publishDraftVersionInTx(ctx, tx, projectID, environmentID, actorUserID, true)
    if err != nil {
        return nil, nil, err
    }

    proposal, err = scanProposal(tx.QueryRow(ctx, `
        update proposals
        set status = 'approved', resolved_by_user_id = $4::uuid, resolved_at = now(), updated_at = now()
        where id = $1::uuid and project_id = $2::uuid and environment_id = $3::uuid and status = 'pending'
        returning id::text, project_id::text, environment_id::text, message, changes::text, status,
                  created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
                  created_at::text, updated_at::text
    `, proposalID, projectID, environmentID, actorUserID))
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil, ErrConflict
    }
    if err != nil {
        return nil, nil, err
    }

    payload, err := json.Marshal(map[string]any{"proposal_id": proposal.ID, "version_number": version.Number})
    if err != nil {
        return nil, nil, err
    }
    if _, err := tx.Exec(ctx, `
        insert into audit_log (actor_user_id, project_id, environment_id, action, payload)
        values ($1::uuid, $2::uuid, $3::uuid, 'proposal.approved', $4::jsonb)
    `, actorUserID, projectID, environmentID, payload); err != nil {
        return nil, nil, err
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, nil, err
    }
    return proposal, version, nil
}

func (s *ProposalStore) RejectProposal(ctx context.Context, projectID, environmentID, proposalID, actorUserID string) (*Proposal, error) {
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadWrite})
    if err != nil {
        return nil, err
    }
    defer func() { _ = tx.Rollback(ctx) }()

    proposal, err := scanProposal(tx.QueryRow(ctx, `
        update proposals
        set status = 'rejected', resolved_by_user_id = $4::uuid, resolved_at = now(), updated_at = now()
        where id = $1::uuid and project_id = $2::uuid and environment_id = $3::uuid and status = 'pending'
        returning id::text, project_id::text, environment_id::text, message, changes::text, status,
                  created_by_user_id::text, resolved_by_user_id::text, resolved_at::text,
                  created_at::text, updated_at::text
    `, proposalID, projectID, environmentID, actorUserID))
    if errors.Is(err, pgx.ErrNoRows) {
        var exists bool
        if scanErr := tx.QueryRow(ctx, `
            select exists(
                select 1 from proposals
                where id = $1::uuid and project_id = $2::uuid and environment_id = $3::uuid
            )
        `, proposalID, projectID, environmentID).Scan(&exists); scanErr != nil {
            return nil, scanErr
        }
        if exists {
            return nil, ErrConflict
        }
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }

    payload, err := json.Marshal(map[string]any{"proposal_id": proposal.ID})
    if err != nil {
        return nil, err
    }
    if _, err := tx.Exec(ctx, `
        insert into audit_log (actor_user_id, project_id, environment_id, action, payload)
        values ($1::uuid, $2::uuid, $3::uuid, 'proposal.rejected', $4::jsonb)
    `, actorUserID, projectID, environmentID, payload); err != nil {
        return nil, err
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return proposal, nil
}

type proposalScanner interface {
    Scan(dest ...any) error
}

func scanProposal(row proposalScanner) (*Proposal, error) {
    proposal := &Proposal{}
    var changesText string
    var resolvedBy sql.NullString
    var resolvedAt sql.NullString

    if err := row.Scan(
        &proposal.ID,
        &proposal.ProjectID,
        &proposal.EnvironmentID,
        &proposal.Message,
        &changesText,
        &proposal.Status,
        &proposal.CreatedByUserID,
        &resolvedBy,
        &resolvedAt,
        &proposal.CreatedAt,
        &proposal.UpdatedAt,
    ); err != nil {
        return nil, err
    }
    if err := json.Unmarshal([]byte(changesText), &proposal.Changes); err != nil {
        return nil, err
    }
    if resolvedBy.Valid {
        proposal.ResolvedByUserID = &resolvedBy.String
    }
    if resolvedAt.Valid {
        proposal.ResolvedAt = &resolvedAt.String
    }
    return proposal, nil
}

func validProposalStatus(status string) bool {
    return status == ProposalStatusPending || status == ProposalStatusApproved || status == ProposalStatusRejected
}

func validProposalChanges(changes []ProposalChange) bool {
    if len(changes) == 0 {
        return false
    }
    for _, change := range changes {
        if !validProposalChange(change) {
            return false
        }
    }
    return true
}

func validProposalChange(change ProposalChange) bool {
    op := strings.TrimSpace(change.Op)
    key := strings.TrimSpace(change.Key)
    switch op {
    case "add":
        return key != "" && validVariableType(change.Type)
    case "update":
        return key != "" && hasUpdatePatch(change) && (change.Type == "" || validVariableType(change.Type))
    case "remove":
        return key != ""
    case "rename":
        return key != "" && strings.TrimSpace(change.To) != ""
    default:
        return false
    }
}

func hasUpdatePatch(change ProposalChange) bool {
    return change.Type != "" || change.Required != nil || change.Secret != nil || change.Default != nil || change.Description != nil || change.Owner != nil || change.Deprecated != nil
}

func validVariableType(variableType string) bool {
    switch variableType {
    case "string", "enum", "boolean", "number":
        return true
    default:
        return false
    }
}

func lockEnvironment(ctx context.Context, tx pgx.Tx, projectID, environmentID string) error {
    var id string
    err := tx.QueryRow(ctx, `
        select id::text
        from environments
        where id = $1::uuid and project_id = $2::uuid
        for update
    `, environmentID, projectID).Scan(&id)
    if errors.Is(err, pgx.ErrNoRows) {
        return ErrNotFound
    }
    return err
}

func applyProposalChange(ctx context.Context, tx pgx.Tx, environmentID string, change ProposalChange) error {
    switch change.Op {
    case "add":
        return applyAddProposalChange(ctx, tx, environmentID, change)
    case "update":
        return applyUpdateProposalChange(ctx, tx, environmentID, change)
    case "remove":
        return applyRemoveProposalChange(ctx, tx, environmentID, change)
    case "rename":
        return applyRenameProposalChange(ctx, tx, environmentID, change)
    default:
        return ErrInvalidProposalChange
    }
}

func applyAddProposalChange(ctx context.Context, tx pgx.Tx, environmentID string, change ProposalChange) error {
    required := false
    if change.Required != nil {
        required = *change.Required
    }
    secret := false
    if change.Secret != nil {
        secret = *change.Secret
    }
    description := ""
    if change.Description != nil {
        description = *change.Description
    }
    deprecated := false
    if change.Deprecated != nil {
        deprecated = *change.Deprecated
    }

    _, err := tx.Exec(ctx, `
        insert into variables (environment_id, key, type, required, secret, default_value, description, owner_user_id, deprecated)
        values ($1::uuid, $2, $3, $4, $5, $6, $7, nullif($8, '')::uuid, $9)
    `, environmentID, change.Key, change.Type, required, secret, change.Default, description, change.Owner, deprecated)
    if err != nil {
        if isUniqueViolation(err) {
            return ErrInvalidProposalChange
        }
        return err
    }
    return nil
}

func applyUpdateProposalChange(ctx context.Context, tx pgx.Tx, environmentID string, change ProposalChange) error {
    var variableType string
    var required bool
    var secret bool
    var defaultValue sql.NullString
    var description string
    var owner sql.NullString
    var deprecated bool

    err := tx.QueryRow(ctx, `
        select type, required, secret, default_value, description, owner_user_id::text, deprecated
        from variables
        where environment_id = $1::uuid and key = $2
        for update
    `, environmentID, change.Key).Scan(&variableType, &required, &secret, &defaultValue, &description, &owner, &deprecated)
    if errors.Is(err, pgx.ErrNoRows) {
        return ErrInvalidProposalChange
    }
    if err != nil {
        return err
    }

    if change.Type != "" {
        variableType = change.Type
    }
    if change.Required != nil {
        required = *change.Required
    }
    if change.Secret != nil {
        secret = *change.Secret
    }
    if change.Default != nil {
        defaultValue = sql.NullString{String: *change.Default, Valid: true}
    }
    if change.Description != nil {
        description = *change.Description
    }
    if change.Owner != nil {
        owner = sql.NullString{String: *change.Owner, Valid: *change.Owner != ""}
    }
    if change.Deprecated != nil {
        deprecated = *change.Deprecated
    }

    _, err = tx.Exec(ctx, `
        update variables
        set type = $3,
            required = $4,
            secret = $5,
            default_value = $6,
            description = $7,
            owner_user_id = nullif($8, '')::uuid,
            deprecated = $9,
            updated_at = now()
        where environment_id = $1::uuid and key = $2
    `, environmentID, change.Key, variableType, required, secret, defaultValue, description, owner.String, deprecated)
    return err
}

func applyRemoveProposalChange(ctx context.Context, tx pgx.Tx, environmentID string, change ProposalChange) error {
    tag, err := tx.Exec(ctx, `
        delete from variables
        where environment_id = $1::uuid and key = $2
    `, environmentID, change.Key)
    if err != nil {
        return err
    }
    if tag.RowsAffected() == 0 {
        return ErrInvalidProposalChange
    }
    return nil
}

func applyRenameProposalChange(ctx context.Context, tx pgx.Tx, environmentID string, change ProposalChange) error {
    var id string
    err := tx.QueryRow(ctx, `
        select id::text
        from variables
        where environment_id = $1::uuid and key = $2
        for update
    `, environmentID, change.Key).Scan(&id)
    if errors.Is(err, pgx.ErrNoRows) {
        return ErrInvalidProposalChange
    }
    if err != nil {
        return err
    }

    var exists bool
    if err := tx.QueryRow(ctx, `
        select exists(
            select 1 from variables
            where environment_id = $1::uuid and key = $2 and key <> $3
        )
    `, environmentID, change.To, change.Key).Scan(&exists); err != nil {
        return err
    }
    if exists {
        return ErrInvalidProposalChange
    }

    _, err = tx.Exec(ctx, `
        update variables
        set key = $3, updated_at = now()
        where environment_id = $1::uuid and key = $2
    `, environmentID, change.Key, change.To)
    if err != nil {
        if isUniqueViolation(err) {
            return ErrInvalidProposalChange
        }
        return err
    }
    return nil
}

func publishDraftVersionInTx(ctx context.Context, tx pgx.Tx, projectID, environmentID, actorUserID string, setStable bool) (*EnvironmentVersion, error) {
    var number int64
    if err := tx.QueryRow(ctx, `
        select coalesce(max(number), 0) + 1
        from environment_versions
        where environment_id = $1::uuid
    `, environmentID).Scan(&number); err != nil {
        return nil, err
    }

    var schemaSnapshot []byte
    if err := tx.QueryRow(ctx, `
        select coalesce(
            jsonb_agg(
                jsonb_build_object(
                    'key', key,
                    'type', type,
                    'required', required,
                    'secret', secret,
                    'default', default_value,
                    'description', description,
                    'owner', owner_user_id::text,
                    'deprecated', deprecated
                )
                order by key
            ),
            '[]'::jsonb
        )
        from variables
        where environment_id = $1::uuid
    `, environmentID).Scan(&schemaSnapshot); err != nil {
        return nil, err
    }

    valuesSnapshot := []byte("{}")
    version := &EnvironmentVersion{}
    if err := tx.QueryRow(ctx, `
        insert into environment_versions (
            id,
            environment_id,
            number,
            schema_snapshot,
            values_snapshot,
            published_by_user_id
        ) values (
            gen_random_uuid(),
            $1::uuid,
            $2,
            $3::jsonb,
            $4::bytea,
            $5::uuid
        )
        returning id::text, environment_id::text, number, schema_snapshot, values_snapshot, published_at::text, published_by_user_id::text
    `, environmentID, number, schemaSnapshot, valuesSnapshot, actorUserID).Scan(
        &version.ID,
        &version.EnvironmentID,
        &version.Number,
        &version.SchemaSnapshot,
        &version.ValuesSnapshot,
        &version.PublishedAt,
        &version.PublishedByUserID,
    ); err != nil {
        if isUniqueViolation(err) {
            return nil, ErrConflict
        }
        return nil, err
    }

    if setStable {
        if _, err := tx.Exec(ctx, `
            update environments
            set stable_version_id = $1::uuid
            where id = $2::uuid
        `, version.ID, environmentID); err != nil {
            return nil, err
        }
    }

    payload, err := json.Marshal(map[string]any{"number": version.Number, "set_stable": setStable})
    if err != nil {
        return nil, err
    }
    if _, err := tx.Exec(ctx, `
        insert into audit_log (actor_user_id, project_id, environment_id, action, payload)
        values ($1::uuid, $2::uuid, $3::uuid, 'version.published', $4::jsonb)
    `, actorUserID, projectID, environmentID, payload); err != nil {
        return nil, err
    }

    return version, nil
}
