package models

import (
	"errors"
	"testing"
	"time"
)

func TestJWTClaimsHardener_ValidAdmission(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	cfg := DefaultJWTClaimsHardenerConfig()
	hardener := NewJWTClaimsHardener(cfg)

	claims := map[string]any{
		"iss":         "https://auth.devplatform.internal",
		"aud":         "devplatform-api",
		"sub":         "usr-12345",
		"tenant_id":   "tenant-corp-alpha",
		"jti":         "token-jti-abcdef123456",
		"iat":         now.Add(-10 * time.Minute).Unix(),
		"nbf":         now.Add(-10 * time.Minute).Unix(),
		"exp":         now.Add(50 * time.Minute).Unix(),
		"roles":       []string{"developer", "operator"},
		"permissions": []string{"deploy:write", "logs:read"},
	}

	validated, err := hardener.Validate("RS256", claims, now)
	if err != nil {
		t.Fatalf("expected valid token admission, got err: %v", err)
	}

	if validated.Subject != "usr-12345" || validated.TenantID != "tenant-corp-alpha" {
		t.Fatalf("unexpected subject or tenant: %+v", validated)
	}
	if len(validated.Roles) != 2 || validated.Roles[0] != "developer" {
		t.Fatalf("unexpected roles: %v", validated.Roles)
	}
}

func TestJWTClaimsHardener_AlgorithmGating(t *testing.T) {
	now := time.Now()
	hardener := NewJWTClaimsHardener(DefaultJWTClaimsHardenerConfig())

	claims := map[string]any{
		"iss":       "https://auth.devplatform.internal",
		"aud":       "devplatform-api",
		"sub":       "u1",
		"tenant_id": "tenant-1",
		"jti":       "token-jti-12345678",
		"exp":       now.Add(time.Hour).Unix(),
	}

	// 'none' algorithm attack
	_, err := hardener.Validate("none", claims, now)
	if !errors.Is(err, ErrAlgorithmNoneRejected) {
		t.Fatalf("expected ErrAlgorithmNoneRejected, got: %v", err)
	}

	// Empty algorithm
	_, err = hardener.Validate("", claims, now)
	if !errors.Is(err, ErrAlgorithmNoneRejected) {
		t.Fatalf("expected ErrAlgorithmNoneRejected on empty alg, got: %v", err)
	}

	// Disallowed symmetric HMAC algorithm
	_, err = hardener.Validate("HS256", claims, now)
	if !errors.Is(err, ErrAlgorithmNotAllowed) {
		t.Fatalf("expected ErrAlgorithmNotAllowed on HS256, got: %v", err)
	}
}

func TestJWTClaimsHardener_TemporalVerification(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	cfg := DefaultJWTClaimsHardenerConfig()
	cfg.ClockSkewTolerance = 30 * time.Second
	cfg.MaxTokenLifetime = 1 * time.Hour
	hardener := NewJWTClaimsHardener(cfg)

	baseClaims := func() map[string]any {
		return map[string]any{
			"iss":       "https://auth.devplatform.internal",
			"aud":       "devplatform-api",
			"sub":       "u1",
			"tenant_id": "tenant-1",
			"jti":       "token-jti-12345678",
		}
	}

	// Expired token (>30s past exp)
	expired := baseClaims()
	expired["exp"] = now.Add(-40 * time.Second).Unix()
	_, err := hardener.Validate("RS256", expired, now)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}

	// Token not yet valid (nbf > 30s in future)
	future := baseClaims()
	future["exp"] = now.Add(time.Hour).Unix()
	future["nbf"] = now.Add(2 * time.Minute).Unix()
	_, err = hardener.Validate("RS256", future, now)
	if !errors.Is(err, ErrTokenNotYetValid) {
		t.Fatalf("expected ErrTokenNotYetValid, got: %v", err)
	}

	// Lifetime exceeded (e.g. 5 hours when max is 1 hour)
	longLived := baseClaims()
	longLived["iat"] = now.Add(-5 * time.Hour).Unix()
	longLived["exp"] = now.Add(30 * time.Minute).Unix()
	_, err = hardener.Validate("RS256", longLived, now)
	if !errors.Is(err, ErrTokenLifetimeExceeded) {
		t.Fatalf("expected ErrTokenLifetimeExceeded, got: %v", err)
	}
}

func TestJWTClaimsHardener_IssuerAndAudience(t *testing.T) {
	now := time.Now()
	hardener := NewJWTClaimsHardener(DefaultJWTClaimsHardenerConfig())

	// Wrong issuer
	c1 := map[string]any{
		"iss":       "https://rogue-issuer.com",
		"aud":       "devplatform-api",
		"sub":       "u1",
		"tenant_id": "tenant-1",
		"jti":       "token-jti-12345678",
		"exp":       now.Add(time.Hour).Unix(),
	}
	_, err := hardener.Validate("RS256", c1, now)
	if !errors.Is(err, ErrInvalidIssuer) {
		t.Fatalf("expected ErrInvalidIssuer, got: %v", err)
	}

	// Wrong audience
	c2 := map[string]any{
		"iss":       "https://auth.devplatform.internal",
		"aud":       "other-internal-service",
		"sub":       "u1",
		"tenant_id": "tenant-1",
		"jti":       "token-jti-12345678",
		"exp":       now.Add(time.Hour).Unix(),
	}
	_, err = hardener.Validate("RS256", c2, now)
	if !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("expected ErrInvalidAudience, got: %v", err)
	}

	// Audience in array of audiences should pass
	c3 := map[string]any{
		"iss":       "https://auth.devplatform.internal",
		"aud":       []any{"other-service", "devplatform-api"},
		"sub":       "u1",
		"tenant_id": "tenant-1",
		"jti":       "token-jti-12345678",
		"exp":       now.Add(time.Hour).Unix(),
	}
	val, err := hardener.Validate("RS256", c3, now)
	if err != nil || val.Subject != "u1" {
		t.Fatalf("expected array audience match to succeed, got: %v", err)
	}
}

func TestJWTClaimsHardener_TenantAndJTI(t *testing.T) {
	now := time.Now()
	hardener := NewJWTClaimsHardener(DefaultJWTClaimsHardenerConfig())

	base := func() map[string]any {
		return map[string]any{
			"iss": "https://auth.devplatform.internal",
			"aud": "devplatform-api",
			"sub": "u1",
			"exp": now.Add(time.Hour).Unix(),
		}
	}

	// Missing tenant_id
	c1 := base()
	c1["jti"] = "token-jti-12345678"
	_, err := hardener.Validate("RS256", c1, now)
	if !errors.Is(err, ErrMissingTenantID) {
		t.Fatalf("expected ErrMissingTenantID, got: %v", err)
	}

	// Malformed tenant_id with injection characters
	c2 := base()
	c2["tenant_id"] = "tenant/../../etc/passwd"
	c2["jti"] = "token-jti-12345678"
	_, err = hardener.Validate("RS256", c2, now)
	if !errors.Is(err, ErrMissingTenantID) {
		t.Fatalf("expected ErrMissingTenantID on invalid tenant format, got: %v", err)
	}

	// Missing JTI
	c3 := base()
	c3["tenant_id"] = "valid-tenant"
	_, err = hardener.Validate("RS256", c3, now)
	if !errors.Is(err, ErrMissingJTI) {
		t.Fatalf("expected ErrMissingJTI, got: %v", err)
	}
}

func TestJWTClaimsHardener_RoleBoundaries(t *testing.T) {
	now := time.Now()
	hardener := NewJWTClaimsHardener(DefaultJWTClaimsHardenerConfig())

	claims := map[string]any{
		"iss":       "https://auth.devplatform.internal",
		"aud":       "devplatform-api",
		"sub":       "u1",
		"tenant_id": "tenant-1",
		"jti":       "token-jti-12345678",
		"exp":       now.Add(time.Hour).Unix(),
		"roles":     []string{"developer", "superuser_unauthorized"},
	}

	_, err := hardener.Validate("RS256", claims, now)
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected ErrInvalidRole for unauthorized role, got: %v", err)
	}
}
