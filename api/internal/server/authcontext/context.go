package authcontext

import (
	"context"
	"errors"
)

var ErrNoUser = errors.New("authenticated user not found in context")

type contextKey string

const (
	userContextKey      contextKey = "auth_user"
	tokenHashContextKey contextKey = "auth_token_hash"
)

// User is the authenticated API user returned by auth endpoints and stored on request context.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// WithAuthenticated attaches the authenticated user and token hash to ctx.
func WithAuthenticated(ctx context.Context, user *User, tokenHash string) context.Context {
	ctx = context.WithValue(ctx, userContextKey, user)
	return context.WithValue(ctx, tokenHashContextKey, tokenHash)
}

// UserFromCtx returns the authenticated user attached to ctx.
func UserFromCtx(ctx context.Context) (*User, error) {
	user, ok := ctx.Value(userContextKey).(*User)
	if !ok || user == nil {
		return nil, ErrNoUser
	}
	return user, nil
}

// TokenHashFromCtx returns the authenticated bearer token hash attached to ctx.
func TokenHashFromCtx(ctx context.Context) (string, bool) {
	tokenHash, ok := ctx.Value(tokenHashContextKey).(string)
	return tokenHash, ok
}
