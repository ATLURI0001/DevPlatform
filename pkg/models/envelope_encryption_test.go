package models

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"sync"
	"testing"
)

func generateRandomBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		t.Fatalf("failed to read random bytes: %v", err)
	}
	return buf
}

func setupTestProvider(t *testing.T) (*InMemoryKEKProvider, []byte) {
	t.Helper()
	provider := NewInMemoryKEKProvider()
	kek := generateRandomBytes(t, 32)
	err := provider.RegisterKEK("kms/us-east-1/key-primary", "v1", kek, true)
	if err != nil {
		t.Fatalf("failed to register test KEK: %v", err)
	}
	return provider, kek
}

func TestEnvelopeEncryption_Roundtrip(t *testing.T) {
	provider, _ := setupTestProvider(t)
	engine := NewEnvelopeCryptoEngine(provider)

	plaintext := []byte("DATABASE_URL=postgres://admin:supersecret@db.prod.internal:5432/core")
	context := map[string]string{
		"tenant_id":   "tenant-789",
		"environment": "production",
		"service":     "billing-gateway",
	}

	payload, err := engine.Encrypt("kms/us-east-1/key-primary", plaintext, context)
	if err != nil {
		t.Fatalf("unexpected encryption error: %v", err)
	}

	if payload.Algorithm != DefaultEnvelopeAlgorithm {
		t.Errorf("expected algorithm %s, got %s", DefaultEnvelopeAlgorithm, payload.Algorithm)
	}
	if payload.KEKID != "kms/us-east-1/key-primary" {
		t.Errorf("expected KEKID kms/us-east-1/key-primary, got %s", payload.KEKID)
	}
	if payload.KEKVersion != "v1" {
		t.Errorf("expected KEKVersion v1, got %s", payload.KEKVersion)
	}

	// Decrypt with matching context
	decrypted, err := engine.Decrypt(payload, context)
	if err != nil {
		t.Fatalf("unexpected decryption error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted payload mismatch:\nexpected: %s\ngot:      %s", string(plaintext), string(decrypted))
	}
}

func TestEnvelopeEncryption_ValidationErrors(t *testing.T) {
	provider, _ := setupTestProvider(t)
	engine := NewEnvelopeCryptoEngine(provider)

	// Empty plaintext
	_, err := engine.Encrypt("kms/us-east-1/key-primary", []byte(""), nil)
	if err != ErrEnvelopeEmptyPlaintext {
		t.Errorf("expected ErrEnvelopeEmptyPlaintext, got %v", err)
	}

	// Non-existent KEK ID
	_, err = engine.Encrypt("kms/unknown-key", []byte("secret"), nil)
	if err == nil {
		t.Error("expected error for non-existent KEK ID, got nil")
	}

	// Invalid key length in provider
	err = provider.RegisterKEK("kms/bad-key", "v1", []byte("too-short"), true)
	if err == nil {
		t.Error("expected ErrInvalidKEKLength, got nil")
	}
}

func TestEnvelopeEncryption_TamperDetection(t *testing.T) {
	provider, _ := setupTestProvider(t)
	engine := NewEnvelopeCryptoEngine(provider)

	plaintext := []byte("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	context := map[string]string{
		"tenant_id": "tenant-alpha",
	}

	payload, err := engine.Encrypt("kms/us-east-1/key-primary", plaintext, context)
	if err != nil {
		t.Fatalf("encryption error: %v", err)
	}

	// Test 1: Tamper with ciphertext
	t.Run("tampered ciphertext", func(t *testing.T) {
		tampered := *payload
		rawCipher, _ := base64.StdEncoding.DecodeString(tampered.Ciphertext)
		rawCipher[0] ^= 0xFF
		tampered.Ciphertext = base64.StdEncoding.EncodeToString(rawCipher)

		_, err := engine.Decrypt(&tampered, context)
		if err == nil {
			t.Error("expected decryption failure on tampered ciphertext, got nil")
		}
	})

	// Test 2: Tamper with wrapped DEK
	t.Run("tampered wrapped DEK", func(t *testing.T) {
		tampered := *payload
		rawDEK, _ := base64.StdEncoding.DecodeString(tampered.WrappedDEK)
		rawDEK[0] ^= 0xAA
		tampered.WrappedDEK = base64.StdEncoding.EncodeToString(rawDEK)

		_, err := engine.Decrypt(&tampered, context)
		if err == nil {
			t.Error("expected decryption failure on tampered wrapped DEK, got nil")
		}
	})

	// Test 3: Tamper with context (tenant spoofing attempt)
	t.Run("context spoofing", func(t *testing.T) {
		spoofedContext := map[string]string{
			"tenant_id": "tenant-bravo", // Attacker tries to steal secret under tenant-bravo
		}
		_, err := engine.Decrypt(payload, spoofedContext)
		if err == nil {
			t.Error("expected error when validating spoofed context, got nil")
		}
	})

	// Test 4: Tamper with internal context map without updating hash
	t.Run("tampered internal context map", func(t *testing.T) {
		tampered := *payload
		tampered.Context = map[string]string{
			"tenant_id": "tenant-charlie",
		}
		_, err := engine.Decrypt(&tampered, nil)
		if err == nil {
			t.Error("expected error when context hash does not match context map, got nil")
		}
	})
}

func TestEnvelopeEncryption_ReWrap(t *testing.T) {
	provider, _ := setupTestProvider(t)
	engine := NewEnvelopeCryptoEngine(provider)

	// Register secondary KEK (e.g. rotated key)
	kekV2 := generateRandomBytes(t, 32)
	err := provider.RegisterKEK("kms/us-east-1/key-primary", "v2", kekV2, true)
	if err != nil {
		t.Fatalf("failed to register KEK v2: %v", err)
	}

	plaintext := []byte("STRIPE_API_KEY=rk_live_9923847291823719827391")
	context := map[string]string{
		"tenant_id": "tenant-99",
	}

	// Encrypt using v1 (set active back to v1 temporarily)
	_ = provider.RegisterKEK("kms/us-east-1/key-v1-only", "v1", kekV2, true)
	payloadV1, err := engine.Encrypt("kms/us-east-1/key-v1-only", plaintext, context)
	if err != nil {
		t.Fatalf("encryption error: %v", err)
	}

	// Re-wrap envelope to the primary KEK (v2)
	reWrapped, err := engine.ReWrap(payloadV1, "kms/us-east-1/key-primary", context)
	if err != nil {
		t.Fatalf("unexpected ReWrap error: %v", err)
	}

	if reWrapped.KEKID != "kms/us-east-1/key-primary" {
		t.Errorf("expected KEKID kms/us-east-1/key-primary, got %s", reWrapped.KEKID)
	}
	if reWrapped.KEKVersion != "v2" {
		t.Errorf("expected KEKVersion v2, got %s", reWrapped.KEKVersion)
	}
	// Verify ciphertext remained identical (no payload re-encryption overhead)
	if reWrapped.Ciphertext != payloadV1.Ciphertext {
		t.Error("ciphertext should remain unchanged during re-wrapping")
	}
	// Verify wrapped DEK changed
	if reWrapped.WrappedDEK == payloadV1.WrappedDEK {
		t.Error("wrapped DEK should have been updated with new KEK")
	}

	// Decrypt re-wrapped payload
	decrypted, err := engine.Decrypt(reWrapped, context)
	if err != nil {
		t.Fatalf("failed to decrypt re-wrapped payload: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted content mismatch: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestEnvelopeEncryption_Concurrency(t *testing.T) {
	provider, _ := setupTestProvider(t)
	engine := NewEnvelopeCryptoEngine(provider)

	var wg sync.WaitGroup
	numWorkers := 20
	iterations := 15

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				secret := []byte("worker-payload-concurrent-test")
				ctx := map[string]string{"worker": string(rune('A' + workerID))}

				env, err := engine.Encrypt("kms/us-east-1/key-primary", secret, ctx)
				if err != nil {
					t.Errorf("worker %d encrypt failed: %v", workerID, err)
					return
				}

				dec, err := engine.Decrypt(env, ctx)
				if err != nil {
					t.Errorf("worker %d decrypt failed: %v", workerID, err)
					return
				}

				if !bytes.Equal(dec, secret) {
					t.Errorf("worker %d decrypt mismatch", workerID)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestEnvelopeEncryption_Zeroize(t *testing.T) {
	data := []byte("super-sensitive-data-that-must-be-zeroized")
	Zeroize(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("byte at index %d was not zeroized, got %v", i, b)
		}
	}
}
