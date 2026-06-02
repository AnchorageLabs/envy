package server

import (
	"context"

	"github.com/AnchorageLabs/envy/api/internal/server/authcontext"
)

// User is the authenticated API user returned by auth endpoints and stored on request context.
type User = authcontext.User

// UserFromCtx returns the authenticated user attached to ctx by auth middleware.
func UserFromCtx(ctx context.Context) (*User, error) {
	return authcontext.UserFromCtx(ctx)
}
