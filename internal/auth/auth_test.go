package auth

import (
	"errors"
	"testing"
)

func TestGenerateAndValidateTokenGuards(t *testing.T) {
	if _, err := GenerateToken("u1", "a@b.com", []string{"admin"}, "", 60); !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("expected ErrInvalidSecret for empty secret, got %v", err)
	}
	if _, err := GenerateToken("u1", "a@b.com", []string{"admin"}, "super-secret", 0); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("expected ErrInvalidTTL for non-positive ttl, got %v", err)
	}
	tok, err := GenerateToken("u1", "a@b.com", []string{"admin"}, "super-secret", 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	claims, err := ValidateToken(tok, "super-secret")
	if err != nil || claims.UserID != "u1" || !claims.HasRole("admin") {
		t.Fatalf("token validation failed: claims=%+v err=%v", claims, err)
	}
}

func TestExtractTokenFromHeader(t *testing.T) {
	for _, bad := range []string{"", "Basic abc", "Bearer ", "Bearer    "} {
		if _, err := ExtractTokenFromHeader(bad); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expected ErrInvalidToken for %q, got %v", bad, err)
		}
	}
	tok, err := ExtractTokenFromHeader("bearer  abc.def.ghi ")
	if err != nil || tok != "abc.def.ghi" {
		t.Fatalf("expected clean extracted token, got %q err=%v", tok, err)
	}
}
