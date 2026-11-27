package security

import "fmt"

// AuditLogContract ensures audit log entries are well-formed.
type AuditLogContract struct {
	RequiredFields []string
}

func NewAuditLogContract(fields []string) *AuditLogContract {
	return &AuditLogContract{RequiredFields: fields}
}

func (c *AuditLogContract) Validate(entry map[string]string) error {
	for _, f := range c.RequiredFields {
		if _, ok := entry[f]; !ok {
			return fmt.Errorf("missing required field: %s", f)
		}
	}
	return nil
}
