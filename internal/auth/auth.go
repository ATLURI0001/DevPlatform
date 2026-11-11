package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token expired")
	ErrInvalidSecret = errors.New("signing secret must not be empty")
	ErrInvalidTTL    = errors.New("token ttl must be positive")
)

type Claims struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

func GenerateToken(userID, email string, roles []string, secret string, ttl int) (string, error) {
	if secret == "" {
		return "", ErrInvalidSecret
	}
	if ttl <= 0 {
		return "", ErrInvalidTTL
	}
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(ttl) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
			Issuer:    "devplatform",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func ValidateToken(tokenString, secret string) (*Claims, error) {
	if secret == "" {
		return nil, ErrInvalidSecret
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func RefreshToken(tokenString, secret string, ttl int) (string, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return "", err
	}

	return GenerateToken(claims.UserID, claims.Email, claims.Roles, secret, ttl)
}

func ExtractTokenFromHeader(authHeader string) (string, error) {
	trimmed := strings.TrimSpace(authHeader)
	if len(trimmed) <= len("Bearer ") {
		return "", ErrInvalidToken
	}

	const prefix = "Bearer "
	if !strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return "", ErrInvalidToken
	}

	token := strings.TrimSpace(trimmed[len(prefix):])
	if token == "" {
		return "", ErrInvalidToken
	}
	return token, nil
}

// HasRole checks if the claims have a specific role
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the claims have any of the specified roles
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, r := range c.Roles {
		for _, allowed := range roles {
			if r == allowed {
				return true
			}
		}
	}
	return false
}