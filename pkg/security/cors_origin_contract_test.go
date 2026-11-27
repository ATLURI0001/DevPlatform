package security

import "testing"

func TestCorsOriginContract(t *testing.T) {
	c := NewCorsOriginContract([]string{"https://app.example.com"})
	if err := c.Validate("https://app.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate("https://evil.com"); err == nil {
		t.Fatal("expected error for disallowed origin")
	}
}
