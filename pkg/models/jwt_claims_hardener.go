package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrAlgorithmNotAllowed   = errors.New("jwt: algorithm is not in allowed cryptographic set")
	ErrAlgorithmNoneRejected = errors.New("jwt: algorithm 'none' is strictly prohibited")
	ErrTokenExpired          = errors.New("jwt: token has expired")
	ErrTokenNotYetValid      = errors.New("jwt: token is not valid yet (nbf violation)")
	ErrTokenLifetimeExceeded = errors.New("jwt: token validity lifetime exceeds maximum allowed duration")
	ErrInvalidIssuer         = errors.New("jwt: token issuer does not match expected identity")
	ErrInvalidAudience       = errors.New("jwt: token audience does not contain expected audience")
	ErrMissingTenantID       = errors.New("jwt: mandatory tenant_id claim is missing or malformed")
	ErrInvalidRole           = errors.New("jwt: role contains invalid characters or is not permitted")
	ErrMissingSubject        = errors.New("jwt: subject (sub) claim is missing or empty")
	ErrMissingJTI            = errors.New("jwt: unique token identifier (jti) is missing")
	ErrMalformedClaims       = errors.New("jwt: claims payload contains invalid or malicious types")
)

var (
	tenantIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{3,64}$`)
	roleNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\:]{2,48}$`)
	jtiRegex      = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{8,128}$`)
)

// JWTClaimsHardenerConfig dictates strict validation rules for token admission.
type JWTClaimsHardenerConfig struct {
	AllowedAlgorithms   []string
	ExpectedIssuer      string
	ExpectedAudience    string
	MaxTokenLifetime    time.Duration
	ClockSkewTolerance  time.Duration
	AllowedRoles        []string
	RequireJTI          bool
	RequireTenantID     bool
}

// DefaultJWTClaimsHardenerConfig returns enterprise-hardened token admission defaults.
func DefaultJWTClaimsHardenerConfig() JWTClaimsHardenerConfig {
	return JWTClaimsHardenerConfig{
		AllowedAlgorithms:   []string{"RS256", "ES256", "EdDSA", "PS256"},
		ExpectedIssuer:      "https://auth.devplatform.internal",
		ExpectedAudience:    "devplatform-api",
		MaxTokenLifetime:    2 * time.Hour,
		ClockSkewTolerance:  1 * time.Minute,
		AllowedRoles:        []string{"admin", "developer", "viewer", "operator", "deployer"},
		RequireJTI:          true,
		RequireTenantID:     true,
	}
}

// ValidatedClaims contains normalized, sanitized token claims safe for internal RBAC routing.
type ValidatedClaims struct {
	Subject     string
	TenantID    string
	Roles       []string
	Permissions []string
	IssuedAt    time.Time
	NotBefore   time.Time
	ExpiresAt   time.Time
	TokenID     string
	Issuer      string
	Audience    string
}

// JWTClaimsHardener verifies raw claims maps against defensive admission policies.
type JWTClaimsHardener struct {
	config       JWTClaimsHardenerConfig
	allowedAlgs  map[string]struct{}
	allowedRoles map[string]struct{}
}

// NewJWTClaimsHardener instantiates a claims hardener.
func NewJWTClaimsHardener(cfg JWTClaimsHardenerConfig) *JWTClaimsHardener {
	if cfg.MaxTokenLifetime <= 0 {
		cfg.MaxTokenLifetime = 2 * time.Hour
	}
	if cfg.ClockSkewTolerance < 0 {
		cfg.ClockSkewTolerance = 1 * time.Minute
	}

	algs := make(map[string]struct{}, len(cfg.AllowedAlgorithms))
	for _, a := range cfg.AllowedAlgorithms {
		algs[strings.ToUpper(strings.TrimSpace(a))] = struct{}{}
	}

	roles := make(map[string]struct{}, len(cfg.AllowedRoles))
	for _, r := range cfg.AllowedRoles {
		roles[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}

	return &JWTClaimsHardener{
		config:       cfg,
		allowedAlgs:  algs,
		allowedRoles: roles,
	}
}

// Validate inspects algorithm and raw claims map, enforcing temporal bounds, tenant boundaries, and role sanitization.
func (h *JWTClaimsHardener) Validate(rawAlg string, claims map[string]any, now time.Time) (*ValidatedClaims, error) {
	if claims == nil {
		return nil, ErrMalformedClaims
	}

	// 1. Algorithm enforcement & alg:none rejection
	normAlg := strings.ToUpper(strings.TrimSpace(rawAlg))
	if normAlg == "NONE" || normAlg == "" {
		return nil, ErrAlgorithmNoneRejected
	}
	if len(h.allowedAlgs) > 0 {
		if _, ok := h.allowedAlgs[normAlg]; !ok {
			return nil, ErrAlgorithmNotAllowed
		}
	}

	// 2. Issuer validation
	iss, _ := claims["iss"].(string)
	if h.config.ExpectedIssuer != "" && iss != h.config.ExpectedIssuer {
		return nil, ErrInvalidIssuer
	}

	// 3. Audience validation
	var audMatches bool
	if h.config.ExpectedAudience == "" {
		audMatches = true
	} else {
		switch audVal := claims["aud"].(type) {
		case string:
			audMatches = (audVal == h.config.ExpectedAudience)
		case []any:
			for _, item := range audVal {
				if s, ok := item.(string); ok && s == h.config.ExpectedAudience {
					audMatches = true
					break
				}
			}
		case []string:
			for _, s := range audVal {
				if s == h.config.ExpectedAudience {
					audMatches = true
					break
				}
			}
		}
	}
	if !audMatches {
		return nil, ErrInvalidAudience
	}

	// 4. Temporal boundaries (exp, nbf, iat)
	expUnix, err := extractUnixTime(claims["exp"])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid exp claim", ErrMalformedClaims)
	}
	expiresAt := time.Unix(expUnix, 0).UTC()

	skew := h.config.ClockSkewTolerance
	if now.After(expiresAt.Add(skew)) {
		return nil, ErrTokenExpired
	}

	var notBefore time.Time
	if nbfVal, exists := claims["nbf"]; exists && nbfVal != nil {
		nbfUnix, err := extractUnixTime(nbfVal)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid nbf claim", ErrMalformedClaims)
		}
		notBefore = time.Unix(nbfUnix, 0).UTC()
		if now.Before(notBefore.Add(-skew)) {
			return nil, ErrTokenNotYetValid
		}
	}

	var issuedAt time.Time
	if iatVal, exists := claims["iat"]; exists && iatVal != nil {
		iatUnix, err := extractUnixTime(iatVal)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid iat claim", ErrMalformedClaims)
		}
		issuedAt = time.Unix(iatUnix, 0).UTC()

		// Max lifetime check
		if expiresAt.Sub(issuedAt) > h.config.MaxTokenLifetime {
			return nil, ErrTokenLifetimeExceeded
		}
	}

	// 5. Subject validation
	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return nil, ErrMissingSubject
	}

	// 6. Tenant ID validation
	tenantID, _ := claims["tenant_id"].(string)
	tenantID = strings.TrimSpace(tenantID)
	if h.config.RequireTenantID {
		if tenantID == "" || !tenantIDRegex.MatchString(tenantID) {
			return nil, ErrMissingTenantID
		}
	}

	// 7. JTI (Unique Token ID)
	jti, _ := claims["jti"].(string)
	jti = strings.TrimSpace(jti)
	if h.config.RequireJTI {
		if jti == "" || !jtiRegex.MatchString(jti) {
			return nil, ErrMissingJTI
		}
	}

	// 8. Roles sanitization and privilege boundary
	var sanitizedRoles []string
	if rawRoles, exists := claims["roles"]; exists && rawRoles != nil {
		rolesList, err := extractStringSlice(rawRoles)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid roles claim structure", ErrMalformedClaims)
		}

		for _, r := range rolesList {
			cleanRole := strings.ToLower(strings.TrimSpace(r))
			if !roleNameRegex.MatchString(cleanRole) {
				return nil, ErrInvalidRole
			}
			if len(h.allowedRoles) > 0 {
				if _, ok := h.allowedRoles[cleanRole]; !ok {
					return nil, fmt.Errorf("%w: '%s'", ErrInvalidRole, cleanRole)
				}
			}
			sanitizedRoles = append(sanitizedRoles, cleanRole)
		}
	}

	// 9. Permissions slice extraction
	var sanitizedPermissions []string
	if rawPerms, exists := claims["permissions"]; exists && rawPerms != nil {
		permsList, err := extractStringSlice(rawPerms)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid permissions claim structure", ErrMalformedClaims)
		}
		for _, p := range permsList {
			cleanPerm := strings.ToLower(strings.TrimSpace(p))
			if cleanPerm != "" {
				sanitizedPermissions = append(sanitizedPermissions, cleanPerm)
			}
		}
	}

	audStr, _ := claims["aud"].(string)

	return &ValidatedClaims{
		Subject:     sub,
		TenantID:    tenantID,
		Roles:       sanitizedRoles,
		Permissions: sanitizedPermissions,
		IssuedAt:    issuedAt,
		NotBefore:   notBefore,
		ExpiresAt:   expiresAt,
		TokenID:     jti,
		Issuer:      iss,
		Audience:    audStr,
	}, nil
}

func extractUnixTime(val any) (int64, error) {
	switch v := val.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	default:
		return 0, errors.New("cannot parse unix timestamp")
	}
}

func extractStringSlice(val any) ([]string, error) {
	switch v := val.(type) {
	case []string:
		return v, nil
	case []any:
		res := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			} else {
				return nil, errors.New("item in slice is not string")
			}
		}
		return res, nil
	default:
		return nil, errors.New("not a valid slice")
	}
}
