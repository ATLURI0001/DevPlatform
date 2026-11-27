package security

import "fmt"

// CorsOriginContract validates allowed CORS origins.
type CorsOriginContract struct {
	Allowed map[string]bool
}

func NewCorsOriginContract(origins []string) *CorsOriginContract {
	m := make(map[string]bool)
	for _, o := range origins {
		m[o] = true
	}
	return &CorsOriginContract{Allowed: m}
}

func (c *CorsOriginContract) Validate(origin string) error {
	if !c.Allowed[origin] {
		return fmt.Errorf("origin not allowed: %s", origin)
	}
	return nil
}
