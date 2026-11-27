package security

import "testing"

func TestAuditLogContract(t *testing.T) {
	c := NewAuditLogContract([]string{"user", "action"})
	entry := map[string]string{"user": "alice", "action": "login"}
	if err := c.Validate(entry); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(map[string]string{"user": "bob"}); err == nil {
		t.Fatal("expected error for missing action field")
	}
}
