package models

import (
	"testing"
)

func TestValidateRoleBindingContract(t *testing.T) {
	// Valid user binding
	validUser := RoleBindingContract{
		Subject:   "engineer@company.internal",
		Namespace: "prod-payments",
		Role:      RoleDeveloper,
	}
	if err := ValidateRoleBindingContract(validUser, RoleOperator); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// Valid service account binding
	validSA := RoleBindingContract{
		Subject:   "sa:ci-cd:deployer-bot",
		Namespace: "staging-apps",
		Role:      RoleOperator,
	}
	if err := ValidateRoleBindingContract(validSA, RoleClusterAdmin); err != nil {
		t.Fatalf("unexpected SA validation error: %v", err)
	}

	// Invalid subject format
	invalidSubject := RoleBindingContract{
		Subject:   "admin; rm -rf /",
		Namespace: "default",
		Role:      RoleViewer,
	}
	if err := ValidateRoleBindingContract(invalidSubject, RoleClusterAdmin); err == nil {
		t.Errorf("expected error for malformed subject, got nil")
	}

	// System namespace modification by non-cluster-admin
	systemNs := RoleBindingContract{
		Subject:   "developer@company.internal",
		Namespace: "kube-system",
		Role:      RoleDeveloper,
	}
	if err := ValidateRoleBindingContract(systemNs, RoleOperator); err == nil {
		t.Errorf("expected error for unauthorized system namespace binding, got nil")
	}

	// Privilege escalation attempt (developer trying to grant operator)
	escalation := RoleBindingContract{
		Subject:   "peer@company.internal",
		Namespace: "dev-sandboxes",
		Role:      RoleOperator,
	}
	if err := ValidateRoleBindingContract(escalation, RoleDeveloper); err == nil {
		t.Errorf("expected error for role privilege escalation, got nil")
	}
}

func TestCheckPermission(t *testing.T) {
	// Viewer can READ but not DEPLOY
	if !CheckPermission(RoleViewer, ActionRead) {
		t.Errorf("Viewer should have READ permission")
	}
	if CheckPermission(RoleViewer, ActionDeploy) {
		t.Errorf("Viewer should NOT have DEPLOY permission")
	}

	// Developer can DEPLOY and PROMOTE_CANARY but not DELETE namespace configs
	if !CheckPermission(RoleDeveloper, ActionDeploy) {
		t.Errorf("Developer should have DEPLOY permission")
	}
	if !CheckPermission(RoleDeveloper, ActionPromoteCanary) {
		t.Errorf("Developer should have PROMOTE_CANARY permission")
	}
	if CheckPermission(RoleDeveloper, ActionDelete) {
		t.Errorf("Developer should NOT have DELETE permission")
	}

	// Operator can DELETE
	if !CheckPermission(RoleOperator, ActionDelete) {
		t.Errorf("Operator should have DELETE permission")
	}

	// ClusterAdmin can ADMIN_OVERRIDE
	if !CheckPermission(RoleClusterAdmin, ActionAdminOverride) {
		t.Errorf("ClusterAdmin should have ADMIN_OVERRIDE permission")
	}
}

func TestSanitizeEnvironmentOverrides(t *testing.T) {
	// Valid env map
	validEnv := map[string]string{
		"PORT":          "8080",
		"LOG_LEVEL":     "INFO",
		"ENABLE_CANARY": "true",
	}
	sanitized, err := SanitizeEnvironmentOverrides(validEnv)
	if err != nil || len(sanitized) != 3 {
		t.Fatalf("unexpected error for valid env: %v", err)
	}

	// Dangerous prefix: LD_PRELOAD
	badEnv := map[string]string{
		"LD_PRELOAD": "/tmp/rootkit.so",
	}
	if _, err := SanitizeEnvironmentOverrides(badEnv); err == nil {
		t.Errorf("expected rejection of LD_PRELOAD, got nil")
	}

	// Dangerous prefix: PYTHONPATH
	badPath := map[string]string{
		"PYTHONPATH_OVERRIDE": "/malicious/pkg",
	}
	if _, err := SanitizeEnvironmentOverrides(badPath); err == nil {
		t.Errorf("expected rejection of PYTHONPATH_OVERRIDE, got nil")
	}

	// Malformed key with spaces/symbols
	badKey := map[string]string{
		"INVALID KEY!": "val",
	}
	if _, err := SanitizeEnvironmentOverrides(badKey); err == nil {
		t.Errorf("expected rejection of malformed key, got nil")
	}

	// CRLF injection in value
	crlfVal := map[string]string{
		"HEADER": "val\r\nEVIL=true",
	}
	if _, err := SanitizeEnvironmentOverrides(crlfVal); err == nil {
		t.Errorf("expected rejection of CRLF in value, got nil")
	}
}
