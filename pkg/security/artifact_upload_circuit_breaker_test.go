package security

import (
	"errors"
	"testing"
	"time"
)

func TestArtifactUploadCircuitBreaker(t *testing.T) {
	cb := NewArtifactUploadCircuitBreaker(2, 5*time.Second)
	fail := func() error { return errors.New("s3 503") }
	_ = cb.Call(fail)
	_ = cb.Call(fail)
	if err := cb.Call(func() error { return nil }); err != ErrArtifactUploadOpen {
		t.Fatalf("expected ErrArtifactUploadOpen, got %v", err)
	}
}
