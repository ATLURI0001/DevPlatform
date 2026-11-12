package gitops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateEnvironmentManifestsContainmentAndValidation(t *testing.T) {
	base := filepath.Join(t.TempDir(), "gitops")
	mgr, err := NewManager(Config{BasePath: base})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if err := mgr.CreateEnvironmentManifests("../../escape", "prod", "default", "us-east", nil); err == nil {
		t.Fatal("expected traversal projectID to be rejected")
	}
	if err := mgr.CreateEnvironmentManifests("proj1", "prod", "ns\ninjected: true", "us-east", nil); err == nil {
		t.Fatal("expected newline YAML injection in namespace to be rejected")
	}
	if err := mgr.CreateEnvironmentManifests("proj1", "prod", "default", "cluster\r\nmalicious", nil); err == nil {
		t.Fatal("expected newline YAML injection in cluster to be rejected")
	}

	if err := mgr.CreateEnvironmentManifests("proj1", "staging", "app-staging", "eu-west", nil); err != nil {
		t.Fatalf("expected valid CreateEnvironmentManifests to succeed: %v", err)
	}
	kustomizePath := filepath.Join(base, "proj1", "staging", "kustomization.yaml")
	if _, err := os.Stat(kustomizePath); err != nil {
		t.Fatalf("expected %s to exist: %v", kustomizePath, err)
	}
}
