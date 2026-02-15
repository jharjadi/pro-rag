package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthService_HashAndCheckPassword(t *testing.T) {
	svc := NewAuthService("test-secret", 24)

	password := "correct-horse-battery-staple"
	hash, err := svc.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Correct password should pass
	if err := svc.CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword with correct password: %v", err)
	}

	// Wrong password should fail
	if err := svc.CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("CheckPassword with wrong password should fail")
	}
}

func TestAuthService_SignAndVerifyToken(t *testing.T) {
	svc := NewAuthService("test-jwt-secret-32bytes-minimum!", 24)

	tokenStr, err := svc.SignToken("user-123", "tenant-456", "admin")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("SignToken returned empty string")
	}

	claims, err := svc.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user-123")
	}
	if claims.TenantID != "tenant-456" {
		t.Errorf("TenantID: got %q, want %q", claims.TenantID, "tenant-456")
	}
	if claims.Role != "admin" {
		t.Errorf("Role: got %q, want %q", claims.Role, "admin")
	}
	if claims.Issuer != "pro-rag" {
		t.Errorf("Issuer: got %q, want %q", claims.Issuer, "pro-rag")
	}
}

func TestAuthService_VerifyToken_Expired(t *testing.T) {
	svc := NewAuthService("test-jwt-secret-32bytes-minimum!", 24)

	// Create a token that's already expired
	now := time.Now().UTC().Add(-25 * time.Hour)
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)), // expired 24h ago
			Issuer:    "pro-rag",
		},
		TenantID: "tenant-1",
		UserID:   "user-1",
		Role:     "user",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-jwt-secret-32bytes-minimum!"))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	_, err = svc.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject expired token")
	}
}

func TestAuthService_VerifyToken_WrongSecret(t *testing.T) {
	svc1 := NewAuthService("secret-one-32bytes-minimum!!!!!", 24)
	svc2 := NewAuthService("secret-two-32bytes-minimum!!!!!", 24)

	tokenStr, err := svc1.SignToken("user-1", "tenant-1", "user")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	_, err = svc2.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject token signed with different secret")
	}
}

func TestAuthService_VerifyToken_InvalidFormat(t *testing.T) {
	svc := NewAuthService("test-secret", 24)

	_, err := svc.VerifyToken("not-a-jwt")
	if err == nil {
		t.Error("VerifyToken should reject invalid token format")
	}

	_, err = svc.VerifyToken("")
	if err == nil {
		t.Error("VerifyToken should reject empty token")
	}
}

func TestAuthService_VerifyToken_MissingClaims(t *testing.T) {
	secret := []byte("test-jwt-secret-32bytes-minimum!")
	svc := NewAuthService(string(secret), 24)

	// Token with missing tenant_id
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			Issuer:    "pro-rag",
		},
		TenantID: "", // missing
		UserID:   "user-1",
		Role:     "user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(secret)

	_, err := svc.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject token with missing tenant_id")
	}

	// Token with missing user_id
	claims.TenantID = "tenant-1"
	claims.UserID = "" // missing
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ = token.SignedString(secret)

	_, err = svc.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject token with missing user_id")
	}

	// Token with missing role
	claims.UserID = "user-1"
	claims.Role = "" // missing
	token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ = token.SignedString(secret)

	_, err = svc.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject token with missing role")
	}
}

func TestAuthService_VerifyToken_WrongSigningMethod(t *testing.T) {
	svc := NewAuthService("test-secret", 24)

	// Create a token with "none" signing method
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"tenant_id": "tenant-1",
		"sub":       "user-1",
		"role":      "admin",
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := svc.VerifyToken(tokenStr)
	if err == nil {
		t.Error("VerifyToken should reject token with 'none' signing method")
	}
}

func TestAuthService_DefaultExpiry(t *testing.T) {
	// Zero expiry should default to 24h
	svc := NewAuthService("test-secret", 0)
	if svc.jwtExpiryH != 24 {
		t.Errorf("expected default expiry 24h, got %d", svc.jwtExpiryH)
	}

	// Negative expiry should default to 24h
	svc = NewAuthService("test-secret", -1)
	if svc.jwtExpiryH != 24 {
		t.Errorf("expected default expiry 24h, got %d", svc.jwtExpiryH)
	}
}
