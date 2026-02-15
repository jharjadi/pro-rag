package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

func TestAuthMiddleware_AuthDisabled_WithTenantID(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, false)

	var gotTenantID, gotUserID, gotRole string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantID = TenantIDFromContext(r.Context())
		gotUserID = UserIDFromContext(r.Context())
		gotRole = RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents?tenant_id=tenant-123", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if gotTenantID != "tenant-123" {
		t.Errorf("tenant_id: got %q, want %q", gotTenantID, "tenant-123")
	}
	if gotUserID != "dev-user" {
		t.Errorf("user_id: got %q, want %q", gotUserID, "dev-user")
	}
	if gotRole != "admin" {
		t.Errorf("role: got %q, want %q", gotRole, "admin")
	}
}

func TestAuthMiddleware_AuthDisabled_MissingTenantID(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, false)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAuthMiddleware_AuthEnabled_ValidToken(t *testing.T) {
	authSvc := service.NewAuthService("test-jwt-secret-32bytes-minimum!", 24)
	mw := AuthMiddleware(authSvc, true)

	tokenStr, err := authSvc.SignToken("user-abc", "tenant-xyz", "admin")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	var gotTenantID, gotUserID, gotRole string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenantID = TenantIDFromContext(r.Context())
		gotUserID = UserIDFromContext(r.Context())
		gotRole = RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotTenantID != "tenant-xyz" {
		t.Errorf("tenant_id: got %q, want %q", gotTenantID, "tenant-xyz")
	}
	if gotUserID != "user-abc" {
		t.Errorf("user_id: got %q, want %q", gotUserID, "user-abc")
	}
	if gotRole != "admin" {
		t.Errorf("role: got %q, want %q", gotRole, "admin")
	}
}

func TestAuthMiddleware_AuthEnabled_MissingHeader(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["message"] != "missing Authorization header" {
		t.Errorf("message: got %q", body["message"])
	}
}

func TestAuthMiddleware_AuthEnabled_InvalidFormat(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_AuthEnabled_InvalidToken(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_AuthEnabled_EmptyBearer(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	mw := AuthMiddleware(authSvc, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireRole_Allowed(t *testing.T) {
	authSvc := service.NewAuthService("test-jwt-secret-32bytes-minimum!", 24)
	authMW := AuthMiddleware(authSvc, true)
	roleMW := RequireRole("admin")

	tokenStr, _ := authSvc.SignToken("user-1", "tenant-1", "admin")

	handler := authMW(roleMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/action", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestRequireRole_Denied(t *testing.T) {
	authSvc := service.NewAuthService("test-jwt-secret-32bytes-minimum!", 24)
	authMW := AuthMiddleware(authSvc, true)
	roleMW := RequireRole("admin") // only admin allowed

	tokenStr, _ := authSvc.SignToken("user-1", "tenant-1", "user") // role=user

	handler := authMW(roleMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/action", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	authSvc := service.NewAuthService("test-jwt-secret-32bytes-minimum!", 24)
	authMW := AuthMiddleware(authSvc, true)
	roleMW := RequireRole("admin", "user") // both allowed

	tokenStr, _ := authSvc.SignToken("user-1", "tenant-1", "user")

	handler := authMW(roleMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/v1/documents", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestContextHelpers_EmptyContext(t *testing.T) {
	// Context without auth values should return empty strings
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := req.Context()

	if v := TenantIDFromContext(ctx); v != "" {
		t.Errorf("TenantIDFromContext: got %q, want empty", v)
	}
	if v := UserIDFromContext(ctx); v != "" {
		t.Errorf("UserIDFromContext: got %q, want empty", v)
	}
	if v := RoleFromContext(ctx); v != "" {
		t.Errorf("RoleFromContext: got %q, want empty", v)
	}
}
