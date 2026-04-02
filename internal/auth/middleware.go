package auth

import (
	"context"
	"net/http"
	"strings"

	"satpam-go/internal/web"
)

type contextKey string

const authContextKey contextKey = "auth"

type AuthContext struct {
	UserID string
	Role   string
}

func RequireAuth(tokenService *TokenService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			web.WriteError(w, http.StatusUnauthorized, "Missing bearer token")
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			web.WriteError(w, http.StatusUnauthorized, "Missing bearer token")
			return
		}

		claims, err := tokenService.VerifyAccess(parts[1])
		if err != nil {
			web.WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), authContextKey, AuthContext{
			UserID: claims.UserID,
			Role:   claims.Role,
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AuthFromContext(ctx context.Context) (AuthContext, bool) {
	value, ok := ctx.Value(authContextKey).(AuthContext)
	return value, ok
}
