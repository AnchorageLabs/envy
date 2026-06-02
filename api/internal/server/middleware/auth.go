package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/AnchorageLabs/envy/api/internal/server/authcontext"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Auth authenticates Authorization: Bearer tokens stored by SHA-256 hash in api_tokens.
func Auth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pool == nil {
				writeUnauthorized(w)
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeUnauthorized(w)
				return
			}

			tokenHash := hashToken(token)
			user := &authcontext.User{}
			err := pool.QueryRow(r.Context(), `
				select u.id::text, u.email, u.name
				from api_tokens t
				join users u on u.id = t.user_id
				where t.token_hash = $1 and t.revoked_at is null
			`, tokenHash).Scan(&user.ID, &user.Email, &user.Name)
			if err != nil {
				writeUnauthorized(w)
				return
			}

			_, _ = pool.Exec(r.Context(), `
				update api_tokens
				set last_used_at = now()
				where token_hash = $1 and revoked_at is null
			`, tokenHash)

			next.ServeHTTP(w, r.WithContext(authcontext.WithAuthenticated(r.Context(), user, tokenHash)))
		})
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: "unauthorized"})
}
