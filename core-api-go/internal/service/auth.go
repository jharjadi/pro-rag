// Package service provides business logic for the core API.
package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthClaims are the JWT claims for pro-rag auth (spec v2.3 §10).
type AuthClaims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenant_id"`
	UserID   string `json:"sub"`
	Role     string `json:"role"`
}

// AuthService handles JWT signing/verification and password checking.
type AuthService struct {
	jwtSecret  []byte
	jwtExpiryH int
	bcryptCost int
}

// NewAuthService creates a new AuthService.
// jwtSecret is the HMAC-SHA256 signing key.
// expiryHours is the JWT token lifetime in hours.
func NewAuthService(jwtSecret string, expiryHours int) *AuthService {
	if expiryHours <= 0 {
		expiryHours = 24
	}
	return &AuthService{
		jwtSecret:  []byte(jwtSecret),
		jwtExpiryH: expiryHours,
		bcryptCost: bcrypt.DefaultCost, // 10
	}
}

// CheckPassword verifies a plaintext password against a bcrypt hash.
// Returns nil if the password matches, an error otherwise.
func (s *AuthService) CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// HashPassword generates a bcrypt hash for the given password.
func (s *AuthService) HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// SignToken creates a signed JWT for the given user.
func (s *AuthService) SignToken(userID, tenantID, role string) (string, error) {
	now := time.Now().UTC()
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.jwtExpiryH) * time.Hour)),
			Issuer:    "pro-rag",
		},
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signed, nil
}

// VerifyToken parses and validates a JWT string, returning the claims.
func (s *AuthService) VerifyToken(tokenStr string) (*AuthClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AuthClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Ensure signing method is HMAC
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate required fields
	if claims.TenantID == "" {
		return nil, fmt.Errorf("token missing tenant_id")
	}
	if claims.UserID == "" {
		return nil, fmt.Errorf("token missing sub (user_id)")
	}
	if claims.Role == "" {
		return nil, fmt.Errorf("token missing role")
	}

	return claims, nil
}
