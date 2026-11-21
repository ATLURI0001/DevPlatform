package models

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidKeyLength  = errors.New("AES-256 key must be exactly 32 bytes")
	ErrKeyNotFound       = errors.New("encryption key version not found in keyring")
	ErrNoActiveKey       = errors.New("no active encryption key designated in keyring")
	ErrDecryptionFailed  = errors.New("ciphertext authentication or decryption failed")
	ErrCorruptedEnvelope = errors.New("secret envelope contains invalid or corrupted fields")
	ErrEmptyPlaintext    = errors.New("cannot encrypt empty plaintext")
)

// SecretEnvelope holds AES-256-GCM encrypted payload metadata for secure storage and transit.
type SecretEnvelope struct {
	KeyVersion       string    `json:"key_version"`
	Algorithm        string    `json:"algorithm"`
	NonceBase64      string    `json:"nonce"`
	CiphertextBase64 string    `json:"ciphertext"`
	AuthDataBase64   string    `json:"auth_data,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// SecretKeyRing manages cryptographic keys across versions, enabling zero-downtime key rotation.
type SecretKeyRing struct {
	mu        sync.RWMutex
	activeVer string
	keys      map[string][]byte
}

// NewSecretKeyRing initializes an empty, concurrency-safe cryptographic key ring.
func NewSecretKeyRing() *SecretKeyRing {
	return &SecretKeyRing{
		keys: make(map[string][]byte),
	}
}

// AddKey registers an AES-256 (32-byte) key under a specific version string.
func (kr *SecretKeyRing) AddKey(version string, key []byte, makeActive bool) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("key version cannot be empty")
	}
	if len(key) != 32 {
		return fmt.Errorf("%w: expected 32 bytes, got %d", ErrInvalidKeyLength, len(key))
	}

	kr.mu.Lock()
	defer kr.mu.Unlock()

	keyCopy := make([]byte, 32)
	copy(keyCopy, key)
	kr.keys[version] = keyCopy

	if makeActive || kr.activeVer == "" {
		kr.activeVer = version
	}
	return nil
}

// SetActiveKey promotes an existing key version to be the primary encryption key.
func (kr *SecretKeyRing) SetActiveKey(version string) error {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	if _, exists := kr.keys[version]; !exists {
		return fmt.Errorf("%w: %s", ErrKeyNotFound, version)
	}
	kr.activeVer = version
	return nil
}

// ActiveKeyVersion returns the identifier of the currently active encryption key.
func (kr *SecretKeyRing) ActiveKeyVersion() (string, error) {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	if kr.activeVer == "" {
		return "", ErrNoActiveKey
	}
	return kr.activeVer, nil
}

// Encrypt seals plaintext data with AES-256-GCM using the active key and optional authenticated data (AAD).
func (kr *SecretKeyRing) Encrypt(plaintext []byte, additionalData []byte) (*SecretEnvelope, error) {
	if len(plaintext) == 0 {
		return nil, ErrEmptyPlaintext
	}

	kr.mu.RLock()
	activeVer := kr.activeVer
	key, exists := kr.keys[activeVer]
	kr.mu.RUnlock()

	if !exists || activeVer == "" {
		return nil, ErrNoActiveKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate secure nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)

	var authDataB64 string
	if len(additionalData) > 0 {
		authDataB64 = base64.StdEncoding.EncodeToString(additionalData)
	}

	return &SecretEnvelope{
		KeyVersion:       activeVer,
		Algorithm:        "AES-256-GCM",
		NonceBase64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextBase64: base64.StdEncoding.EncodeToString(ciphertext),
		AuthDataBase64:   authDataB64,
		CreatedAt:        time.Now().UTC(),
	}, nil
}

// Decrypt opens an encrypted envelope using the designated key version and verifies authenticated data.
func (kr *SecretKeyRing) Decrypt(env *SecretEnvelope) ([]byte, error) {
	if env == nil {
		return nil, errors.New("cannot decrypt nil envelope")
	}
	if env.KeyVersion == "" || env.NonceBase64 == "" || env.CiphertextBase64 == "" {
		return nil, ErrCorruptedEnvelope
	}

	kr.mu.RLock()
	key, exists := kr.keys[env.KeyVersion]
	kr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: version %s", ErrKeyNotFound, env.KeyVersion)
	}

	nonce, err := base64.StdEncoding.DecodeString(env.NonceBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid nonce encoding", ErrCorruptedEnvelope)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(env.CiphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid ciphertext encoding", ErrCorruptedEnvelope)
	}

	var authData []byte
	if env.AuthDataBase64 != "" {
		authData, err = base64.StdEncoding.DecodeString(env.AuthDataBase64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid auth data encoding", ErrCorruptedEnvelope)
		}
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("%w: invalid nonce length %d", ErrCorruptedEnvelope, len(nonce))
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, authData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// Rotate re-encrypts an existing envelope under the current active key version.
func (kr *SecretKeyRing) Rotate(env *SecretEnvelope) (*SecretEnvelope, error) {
	plaintext, err := kr.Decrypt(env)
	if err != nil {
		return nil, fmt.Errorf("rotation failed to decrypt source envelope: %w", err)
	}

	var authData []byte
	if env.AuthDataBase64 != "" {
		authData, _ = base64.StdEncoding.DecodeString(env.AuthDataBase64)
	}

	return kr.Encrypt(plaintext, authData)
}
