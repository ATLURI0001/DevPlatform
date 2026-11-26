package security

import (
	"testing"
	"time"
)

func TestArtifactTTLSentinel(t *testing.T) {
	s := &ArtifactTTLSentinel{MaxTTL: 24 * time.Hour}
	valid := ArtifactRecord{ID: "artifact-001", CreatedAt: time.Now(), TTL: 1 * time.Hour}
	if err := s.Validate(valid); err != nil {
		t.Fatalf("expected valid artifact, got: %v", err)
	}
	expired := ArtifactRecord{ID: "artifact-002", CreatedAt: time.Now().Add(-2 * time.Hour), TTL: 1 * time.Hour}
	if err := s.Validate(expired); err == nil {
		t.Fatal("expected expired artifact sentinel violation")
	}
	tooLong := ArtifactRecord{ID: "artifact-003", CreatedAt: time.Now(), TTL: 48 * time.Hour}
	if err := s.Validate(tooLong); err == nil {
		t.Fatal("expected TTL exceeds max sentinel violation")
	}
}
