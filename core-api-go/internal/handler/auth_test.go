package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

// ── Login request/response serialization tests ───────────

func TestLoginRequest_Serialization(t *testing.T) {
	req := loginRequest{
		Email:    "admin@test.local",
		Password: "secret123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded loginRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Email != "admin@test.local" {
		t.Errorf("email: got %q, want %q", decoded.Email, "admin@test.local")
	}
	if decoded.Password != "secret123" {
		t.Errorf("password: got %q, want %q", decoded.Password, "secret123")
	}
}

func TestLoginResponse_Serialization(t *testing.T) {
	resp := loginResponse{
		Token:    "jwt-token-here",
		UserID:   "user-123",
		TenantID: "tenant-456",
		Role:     "admin",
		Email:    "admin@test.local",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	requiredFields := []string{"token", "user_id", "tenant_id", "role", "email"}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}
}

// ── Login handler validation tests (no DB needed) ────────

func TestLogin_InvalidJSON(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	h := NewAuthHandler(nil, authSvc) // nil pool — won't reach DB

	body := bytes.NewBufferString("not json")
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] != "bad_request" {
		t.Errorf("error: got %q, want %q", errResp["error"], "bad_request")
	}
}

func TestLogin_MissingEmail(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	h := NewAuthHandler(nil, authSvc)

	body, _ := json.Marshal(loginRequest{Email: "", Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["message"] != "email and password are required" {
		t.Errorf("message: got %q", errResp["message"])
	}
}

func TestLogin_MissingPassword(t *testing.T) {
	authSvc := service.NewAuthService("test-secret", 24)
	h := NewAuthHandler(nil, authSvc)

	body, _ := json.Marshal(loginRequest{Email: "admin@test.local", Password: ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ── AuthService integration with handler ─────────────────

func TestAuthHandler_PasswordHashVerification(t *testing.T) {
	// Verify that the auth service can hash and check passwords
	// This is an integration test between AuthHandler and AuthService
	authSvc := service.NewAuthService("test-secret", 24)

	password := "admin-password-123"
	hash, err := authSvc.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Correct password
	if err := authSvc.CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword with correct password: %v", err)
	}

	// Wrong password
	if err := authSvc.CheckPassword(hash, "wrong"); err == nil {
		t.Error("CheckPassword with wrong password should fail")
	}

	// Empty hash (dev user)
	if err := authSvc.CheckPassword("", password); err == nil {
		t.Error("CheckPassword with empty hash should fail")
	}
}

func TestAuthHandler_TokenRoundTrip(t *testing.T) {
	// Verify that a token signed by the auth service can be verified
	authSvc := service.NewAuthService("test-jwt-secret-32bytes-minimum!", 24)

	token, err := authSvc.SignToken("user-id", "tenant-id", "admin")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	claims, err := authSvc.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}

	if claims.UserID != "user-id" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user-id")
	}
	if claims.TenantID != "tenant-id" {
		t.Errorf("TenantID: got %q, want %q", claims.TenantID, "tenant-id")
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q, want %q", claims.Role, "admin")
	}
}
