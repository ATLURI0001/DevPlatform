package models

import (
	"bytes"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
)

func generateRandomKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	return k
}

func TestSecretKeyRing_KeyValidation(t *testing.T) {
	kr := NewSecretKeyRing()

	// 16-byte key rejected
	err := kr.AddKey("v1", make([]byte, 16), true)
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("expected ErrInvalidKeyLength, got %v", err)
	}

	// 32-byte key accepted
	k32 := generateRandomKey(t)
	if err := kr.AddKey("v1", k32, true); err != nil {
		t.Fatalf("failed to add 32-byte key: %v", err)
	}

	ver, err := kr.ActiveKeyVersion()
	if err != nil || ver != "v1" {
		t.Fatalf("expected active key v1, got %s (err: %v)", ver, err)
	}
}

func TestSecretKeyRing_RoundTripEncryption(t *testing.T) {
	kr := NewSecretKeyRing()
	key := generateRandomKey(t)
	_ = kr.AddKey("2026-v1", key, true)

	plaintext := []byte("super-sensitive-database-password-XYZ-999")
	aad := []byte("tenant=org_442;cluster=us-east-1")

	env, err := kr.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if env.KeyVersion != "2026-v1" {
		t.Fatalf("expected key version 2026-v1, got %s", env.KeyVersion)
	}
	if env.Algorithm != "AES-256-GCM" {
		t.Fatalf("expected AES-256-GCM, got %s", env.Algorithm)
	}

	decrypted, err := kr.Decrypt(env)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted %s does not match plaintext %s", string(decrypted), string(plaintext))
	}
}

func TestSecretKeyRing_AuthenticationTampering(t *testing.T) {
	kr := NewSecretKeyRing()
	key := generateRandomKey(t)
	_ = kr.AddKey("v1", key, true)

	env, err := kr.Encrypt([]byte("payload"), []byte("correct-aad"))
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with ciphertext by altering characters
	tampered := *env
	tampered.CiphertextBase64 = "AAAA" + tampered.CiphertextBase64[4:]

	_, err = kr.Decrypt(&tampered)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed on tampered ciphertext, got %v", err)
	}

	// Tamper with AAD
	tamperedAAD := *env
	tamperedAAD.AuthDataBase64 = "Zm9v" // base64 of "foo"
	_, err = kr.Decrypt(&tamperedAAD)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed on tampered AAD, got %v", err)
	}
}

func TestSecretKeyRing_KeyRotation(t *testing.T) {
	kr := NewSecretKeyRing()
	keyV1 := generateRandomKey(t)
	keyV2 := generateRandomKey(t)

	_ = kr.AddKey("v1", keyV1, true)

	originalData := []byte("api-token-secret-production")
	envV1, err := kr.Encrypt(originalData, nil)
	if err != nil {
		t.Fatal(err)
	}
	if envV1.KeyVersion != "v1" {
		t.Fatalf("expected env v1, got %s", envV1.KeyVersion)
	}

	// Introduce key v2 and make it active
	_ = kr.AddKey("v2", keyV2, true)

	// Decrypting v1 envelope still succeeds using keyring v1 key
	recovered, err := kr.Decrypt(envV1)
	if err != nil {
		t.Fatalf("failed to decrypt v1 envelope after v2 activation: %v", err)
	}
	if !bytes.Equal(recovered, originalData) {
		t.Fatal("data mismatch")
	}

	// Rotate envelope to v2
	envV2, err := kr.Rotate(envV1)
	if err != nil {
		t.Fatalf("rotation to v2 failed: %v", err)
	}
	if envV2.KeyVersion != "v2" {
		t.Fatalf("expected rotated envelope to have key version v2, got %s", envV2.KeyVersion)
	}

	// Decrypt v2 envelope
	finalData, err := kr.Decrypt(envV2)
	if err != nil || !bytes.Equal(finalData, originalData) {
		t.Fatalf("failed to decrypt rotated envelope: %v", err)
	}
}

func TestSecretKeyRing_Concurrency(t *testing.T) {
	kr := NewSecretKeyRing()
	_ = kr.AddKey("v1", generateRandomKey(t), true)

	var wg sync.WaitGroup
	data := []byte("concurrent-encryption-payload")

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			env, err := kr.Encrypt(data, nil)
			if err != nil {
				t.Errorf("concurrent encrypt failed: %v", err)
				return
			}
			dec, err := kr.Decrypt(env)
			if err != nil || !bytes.Equal(dec, data) {
				t.Errorf("concurrent decrypt failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}
