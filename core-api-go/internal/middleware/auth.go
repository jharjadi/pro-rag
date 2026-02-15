// Package middleware provides HTTP middleware for the core API.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// ContextKeyTenantID is the context key for the authenticated tenant ID.
	ContextKeyTenantID contextKey = "tenant_id"
	// ContextKeyUserID is the context key for the authenticated user ID.
	ContextKeyUserID contextKey = "user_id"
	// ContextKeyRole is the context key for the authenticated user role.
	ContextKeyRole contextKey = "role"
)

// TenantIDFromContext extracts the tenant_id from the request context.
// Returns empty string if not present.
func TenantIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyTenantID).(string)
	return v
}

// UserIDFromContext extracts the user_id from the request context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyUserID).(string)
	return v
}

// RoleFromContext extracts the role from the request context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyRole).(string)
	return v
}

// AuthMiddleware validates JWT tokens and injects claims into the request context.
//
// When authEnabled=true:
//   - Requires a valid JWT in the Authorization header (Bearer <token>)
//   - Extracts tenant_id, user_id, role from JWT claims into context
//
// When authEnabled=false (dev mode):
//   - Accepts tenant_id from query parameter or form field (backward compat)
//   - Sets user_id="dev-user" and role="admin" in context
func AuthMiddleware(authSvc *service.AuthService, authEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authEnabled {
				// Dev mode: accept tenant_id from query param or form field
				tenantID := r.URL.Query().Get("tenant_id")
				if tenantID == "" {
					tenantID = r.FormValue("tenant_id")
				}
				if tenantID == "" {
					writeAuthError(w, http.StatusBadRequest, "tenant_id is required (auth disabled mode)")
					return
				}

				ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
				ctx = context.WithValue(ctx, ContextKeyUserID, "dev-user")
				ctx = context.WithValue(ctx, ContextKeyRole, "admin")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Auth enabled: require JWT
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeAuthError(w, http.StatusUnauthorized, "invalid Authorization header format (expected: Bearer <token>)")
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenStr == "" {
				writeAuthError(w, http.StatusUnauthorized, "empty bearer token")
				return
			}

			claims, err := authSvc.VerifyToken(tokenStr)
			if err != nil {
				slog.Debug("JWT verification failed", "error", err)
				writeAuthError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			// Inject claims into context
			ctx := context.WithValue(r.Context(), ContextKeyTenantID, claims.TenantID)
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that checks the user has one of the allowed roles.
// Must be used after AuthMiddleware.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if !allowed[role] {
				writeAuthError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Use simple string concatenation to avoid import cycle with handler package
	w.Write([]byte(`{"error":"` + http.StatusText(status) + `","message":"` + message + `"}`))
}
