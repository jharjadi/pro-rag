package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	pool    *pgxpool.Pool
	authSvc *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{
		pool:    pool,
		authSvc: authSvc,
	}
}

// loginRequest is the POST /v1/auth/login request body.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the POST /v1/auth/login response body.
type loginResponse struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
	Email    string `json:"email"`
}

// Login handles POST /v1/auth/login.
// Validates credentials against the users table and returns a signed JWT.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email and password are required")
		return
	}

	// Look up user by email (across all tenants — email is unique per tenant)
	var userID, tenantID, role, passwordHash string
	var isActive bool
	err := h.pool.QueryRow(ctx,
		`SELECT user_id, tenant_id, role, password_hash, is_active
		 FROM users
		 WHERE email = $1
		 LIMIT 1`,
		req.Email,
	).Scan(&userID, &tenantID, &role, &passwordHash, &isActive)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Don't reveal whether the email exists
			slog.Debug("login failed: user not found", "email", req.Email)
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid email or password")
			return
		}
		slog.Error("login: database error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "authentication failed")
		return
	}

	if !isActive {
		slog.Debug("login failed: user deactivated", "email", req.Email, "user_id", userID)
		writeError(w, http.StatusUnauthorized, "unauthorized", "account is deactivated")
		return
	}

	if passwordHash == "" {
		// Dev-only user with no password — reject in auth-enabled mode
		slog.Debug("login failed: no password hash set", "email", req.Email, "user_id", userID)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid email or password")
		return
	}

	// Verify password
	if err := h.authSvc.CheckPassword(passwordHash, req.Password); err != nil {
		slog.Debug("login failed: wrong password", "email", req.Email, "user_id", userID)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid email or password")
		return
	}

	// Sign JWT
	token, err := h.authSvc.SignToken(userID, tenantID, role)
	if err != nil {
		slog.Error("login: failed to sign token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "authentication failed")
		return
	}

	slog.Info("user logged in",
		"event", "user_login",
		"user_id", userID,
		"tenant_id", tenantID,
		"role", role,
		"email", req.Email,
	)

	writeJSON(w, http.StatusOK, loginResponse{
		Token:    token,
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		Email:    req.Email,
	})
}
