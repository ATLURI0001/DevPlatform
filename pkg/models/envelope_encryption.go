package models

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// AES256KeySize represents the required key length for AES-256 (32 bytes).
	AES256KeySize = 32
	// GCMStandardNonceSize represents the standard 96-bit (12-byte) nonce for AES-GCM.
	GCMStandardNonceSize = 12
	// DefaultEnvelopeAlgorithm is the canonical algorithm descriptor.
	DefaultEnvelopeAlgorithm = "AES-256-GCM-DEK+KW"
)

var (
	// ErrInvalidKEKLength is returned when a Key Encryption Key does not match 32 bytes.
	ErrInvalidKEKLength = errors.New("KEK must be exactly 32 bytes for AES-256")
	// ErrInvalidDEKLength is returned when a Data Encryption Key does not match 32 bytes.
	ErrInvalidDEKLength = errors.New("DEK must be exactly 32 bytes for AES-256")
	// ErrKEKNotFound indicates the requested KEK ID or version was not found.
	ErrKEKNotFound = errors.New("key encryption key ID or version not found in provider")
	// ErrCorruptedEnvelope indicates the payload contains invalid or unparseable fields.
	ErrEnvelopeMalformed = errors.New("envelope encrypted payload is malformed or missing required fields")
	// ErrEnvelopeAuthFailed is returned when ciphertext authentication tag validation fails.
	ErrEnvelopeAuthFailed = errors.New("envelope decryption authentication tag mismatch or tamper detected")
	// ErrEnvelopeEmptyPlaintext is returned when attempting to encrypt empty bytes.
	ErrEnvelopeEmptyPlaintext = errors.New("cannot envelope-encrypt empty plaintext")
	// ErrContextTampered indicates the supplied context does not match the bound context or hash.
	ErrContextTampered = errors.New("encryption context validation failed or context was tampered with")
)

// EnvelopeSecretPayload encapsulates an envelope-encrypted secret along with KMS metadata.
type EnvelopeSecretPayload struct {
	KEKID        string            `json:"kek_id"`
	KEKVersion   string            `json:"kek_version"`
	Algorithm    string            `json:"algorithm"`
	WrappedDEK   string            `json:"wrapped_dek"`   // Base64-encoded encrypted DEK
	DEKNonce     string            `json:"dek_nonce"`     // Base64-encoded nonce used to wrap the DEK
	Ciphertext   string            `json:"ciphertext"`    // Base64-encoded encrypted secret payload
	DataNonce    string            `json:"data_nonce"`    // Base64-encoded nonce used to encrypt payload
	Context      map[string]string `json:"context"`       // Cryptographically bound encryption context
	ContextHash  string            `json:"context_hash"`  // Hex-encoded SHA-256 fingerprint of canonical context
	CreatedAt    time.Time         `json:"created_at"`
}

// KEKProvider defines the interface for retrieving master Key Encryption Keys from KMS/HSM.
type KEKProvider interface {
	GetKEK(kekID, version string) ([]byte, error)
	CurrentVersion(kekID string) (string, error)
}

// InMemoryKEKProvider is a thread-safe in-memory KEK store for local KMS and testing.
type InMemoryKEKProvider struct {
	mu             sync.RWMutex
	keys           map[string]map[string][]byte // kekID -> version -> key
	currentVersion map[string]string            // kekID -> active version
}

// NewInMemoryKEKProvider initializes a clean InMemoryKEKProvider.
func NewInMemoryKEKProvider() *InMemoryKEKProvider {
	return &InMemoryKEKProvider{
		keys:           make(map[string]map[string][]byte),
		currentVersion: make(map[string]string),
	}
}

// RegisterKEK adds a 32-byte master key under a given ID and version.
func (p *InMemoryKEKProvider) RegisterKEK(kekID, version string, key []byte, isCurrent bool) error {
	kekID = strings.TrimSpace(kekID)
	version = strings.TrimSpace(version)
	if kekID == "" || version == "" {
		return errors.New("kekID and version cannot be empty")
	}
	if len(key) != AES256KeySize {
		return fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKEKLength, AES256KeySize, len(key))
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.keys[kekID]; !exists {
		p.keys[kekID] = make(map[string][]byte)
	}

	keyCopy := make([]byte, AES256KeySize)
	copy(keyCopy, key)
	p.keys[kekID][version] = keyCopy

	if isCurrent || p.currentVersion[kekID] == "" {
		p.currentVersion[kekID] = version
	}
	return nil
}

// GetKEK retrieves the specific key bytes for a given KEK ID and version.
func (p *InMemoryKEKProvider) GetKEK(kekID, version string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	versions, exists := p.keys[kekID]
	if !exists {
		return nil, fmt.Errorf("%w: kekID=%s", ErrKEKNotFound, kekID)
	}

	key, found := versions[version]
	if !found {
		return nil, fmt.Errorf("%w: kekID=%s, version=%s", ErrKEKNotFound, kekID, version)
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	return keyCopy, nil
}

// CurrentVersion retrieves the currently active version for a KEK ID.
func (p *InMemoryKEKProvider) CurrentVersion(kekID string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ver, exists := p.currentVersion[kekID]
	if !exists || ver == "" {
		return "", fmt.Errorf("%w: no current version for kekID=%s", ErrKEKNotFound, kekID)
	}
	return ver, nil
}

// EnvelopeCryptoEngine orchestrates KMS envelope encryption, DEK generation, wrapping, and zeroization.
type EnvelopeCryptoEngine struct {
	provider KEKProvider
}

// NewEnvelopeCryptoEngine creates an engine bound to the specified KEKProvider.
func NewEnvelopeCryptoEngine(provider KEKProvider) *EnvelopeCryptoEngine {
	return &EnvelopeCryptoEngine{
		provider: provider,
	}
}

// Zeroize explicitly overwrites a byte slice in memory with zeroes to prevent memory scraping.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// GenerateDEK generates a cryptographically secure 256-bit random Data Encryption Key.
func (e *EnvelopeCryptoEngine) GenerateDEK() ([]byte, error) {
	dek := make([]byte, AES256KeySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("failed to generate secure random DEK: %w", err)
	}
	return dek, nil
}

// CanonicalizeContext serializes a context map into a deterministic, canonical byte representation.
func CanonicalizeContext(ctx map[string]string) ([]byte, string) {
	if len(ctx) == 0 {
		h := sha256.Sum256([]byte{})
		return []byte{}, hex.EncodeToString(h[:])
	}

	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(ctx[k])
	}

	canon := []byte(sb.String())
	h := sha256.Sum256(canon)
	return canon, hex.EncodeToString(h[:])
}

// WrapDEK encrypts a plaintext DEK using AES-256-GCM under the KEK with context as AAD.
func (e *EnvelopeCryptoEngine) WrapDEK(kek, dek, contextAAD []byte) (wrappedDEK, nonce []byte, err error) {
	if len(kek) != AES256KeySize {
		return nil, nil, ErrInvalidKEKLength
	}
	if len(dek) != AES256KeySize {
		return nil, nil, ErrInvalidDEKLength
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher for KEK: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize GCM for KEK: %w", err)
	}

	nonce = make([]byte, GCMStandardNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to read random nonce for DEK wrapping: %w", err)
	}

	wrappedDEK = gcm.Seal(nil, nonce, dek, contextAAD)
	return wrappedDEK, nonce, nil
}

// UnwrapDEK decrypts a wrapped DEK using AES-256-GCM under the KEK with context as AAD.
// The caller is responsible for zeroizing the returned DEK after use.
func (e *EnvelopeCryptoEngine) UnwrapDEK(kek, wrappedDEK, nonce, contextAAD []byte) ([]byte, error) {
	if len(kek) != AES256KeySize {
		return nil, ErrInvalidKEKLength
	}
	if len(nonce) != GCMStandardNonceSize {
		return nil, fmt.Errorf("%w: invalid DEK nonce size %d", ErrEnvelopeMalformed, len(nonce))
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher for KEK unwrap: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM for KEK unwrap: %w", err)
	}

	dek, err := gcm.Open(nil, nonce, wrappedDEK, contextAAD)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to unwrap DEK: %v", ErrEnvelopeAuthFailed, err)
	}

	if len(dek) != AES256KeySize {
		Zeroize(dek)
		return nil, ErrInvalidDEKLength
	}

	return dek, nil
}

// Encrypt generates a fresh ephemeral DEK, encrypts the plaintext using AES-256-GCM,
// wraps the DEK under the specified KMS KEK, zeroizes the plaintext DEK, and returns the envelope payload.
func (e *EnvelopeCryptoEngine) Encrypt(kekID string, plaintext []byte, context map[string]string) (*EnvelopeSecretPayload, error) {
	if len(plaintext) == 0 {
		return nil, ErrEnvelopeEmptyPlaintext
	}

	kekID = strings.TrimSpace(kekID)
	if kekID == "" {
		return nil, errors.New("kekID cannot be empty")
	}

	version, err := e.provider.CurrentVersion(kekID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve active version for KEK %s: %w", kekID, err)
	}

	kek, err := e.provider.GetKEK(kekID, version)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve KEK %s version %s: %w", kekID, version, err)
	}
	defer Zeroize(kek)

	// Canonicalize context and compute hash
	canonContext, contextHash := CanonicalizeContext(context)

	// Generate fresh ephemeral DEK
	dek, err := e.GenerateDEK()
	if err != nil {
		return nil, err
	}
	defer Zeroize(dek)

	// Encrypt payload with DEK
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher for payload encryption: %w", err)
	}

	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM for payload encryption: %w", err)
	}

	dataNonce := make([]byte, GCMStandardNonceSize)
	if _, err := io.ReadFull(rand.Reader, dataNonce); err != nil {
		return nil, fmt.Errorf("failed to generate payload nonce: %w", err)
	}

	ciphertext := dekGCM.Seal(nil, dataNonce, plaintext, canonContext)

	// Wrap DEK with KEK
	wrappedDEK, dekNonce, err := e.WrapDEK(kek, dek, canonContext)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap DEK: %w", err)
	}

	// Copy context map for immutability
	ctxCopy := make(map[string]string, len(context))
	for k, v := range context {
		ctxCopy[k] = v
	}

	return &EnvelopeSecretPayload{
		KEKID:       kekID,
		KEKVersion:  version,
		Algorithm:   DefaultEnvelopeAlgorithm,
		WrappedDEK:  base64.StdEncoding.EncodeToString(wrappedDEK),
		DEKNonce:    base64.StdEncoding.EncodeToString(dekNonce),
		Ciphertext:  base64.StdEncoding.EncodeToString(ciphertext),
		DataNonce:   base64.StdEncoding.EncodeToString(dataNonce),
		Context:     ctxCopy,
		ContextHash: contextHash,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// Decrypt verifies context integrity, unwraps the ephemeral DEK via KEK, decrypts the payload,
// immediately zeroizes the decrypted DEK, and returns the plaintext.
func (e *EnvelopeCryptoEngine) Decrypt(payload *EnvelopeSecretPayload, expectedContext map[string]string) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: payload is nil", ErrEnvelopeMalformed)
	}
	if payload.KEKID == "" || payload.KEKVersion == "" || payload.WrappedDEK == "" ||
		payload.DEKNonce == "" || payload.Ciphertext == "" || payload.DataNonce == "" {
		return nil, fmt.Errorf("%w: missing required cryptographic envelope attributes", ErrEnvelopeMalformed)
	}

	// Validate context integrity
	canonContext, computedHash := CanonicalizeContext(payload.Context)
	if subtle.ConstantTimeCompare([]byte(payload.ContextHash), []byte(computedHash)) != 1 {
		return nil, fmt.Errorf("%w: context hash mismatch", ErrContextTampered)
	}

	// If caller provides expected context, verify every key/value match
	if expectedContext != nil {
		if len(payload.Context) != len(expectedContext) {
			return nil, fmt.Errorf("%w: expected context size %d, got %d", ErrContextTampered, len(expectedContext), len(payload.Context))
		}
		for k, v := range expectedContext {
			if pv, ok := payload.Context[k]; !ok || pv != v {
				return nil, fmt.Errorf("%w: mismatch on context key %q", ErrContextTampered, k)
			}
		}
	}

	// Retrieve KEK from provider
	kek, err := e.provider.GetKEK(payload.KEKID, payload.KEKVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve KEK for decryption: %w", err)
	}
	defer Zeroize(kek)

	// Decode Base64 components
	wrappedDEK, err := base64.StdEncoding.DecodeString(payload.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 wrapped_dek", ErrEnvelopeMalformed)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(payload.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 dek_nonce", ErrEnvelopeMalformed)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 ciphertext", ErrEnvelopeMalformed)
	}

	dataNonce, err := base64.StdEncoding.DecodeString(payload.DataNonce)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 data_nonce", ErrEnvelopeMalformed)
	}

	if len(dataNonce) != GCMStandardNonceSize {
		return nil, fmt.Errorf("%w: invalid data nonce size %d", ErrEnvelopeMalformed, len(dataNonce))
	}

	// Unwrap DEK
	dek, err := e.UnwrapDEK(kek, wrappedDEK, dekNonce, canonContext)
	if err != nil {
		return nil, err
	}
	defer Zeroize(dek)

	// Decrypt payload with DEK
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher from unwrapped DEK: %w", err)
	}

	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM from unwrapped DEK: %w", err)
	}

	plaintext, err := dekGCM.Open(nil, dataNonce, ciphertext, canonContext)
	if err != nil {
		return nil, fmt.Errorf("%w: payload authentication tag mismatch: %v", ErrEnvelopeAuthFailed, err)
	}

	return plaintext, nil
}

// ReWrap rotates the master KEK wrapping without decrypting or re-encrypting the underlying payload.
// This allows high-throughput zero-downtime key rotation across millions of stored secrets.
func (e *EnvelopeCryptoEngine) ReWrap(payload *EnvelopeSecretPayload, newKEKID string, expectedContext map[string]string) (*EnvelopeSecretPayload, error) {
	if payload == nil {
		return nil, fmt.Errorf("%w: payload is nil", ErrEnvelopeMalformed)
	}

	newKEKID = strings.TrimSpace(newKEKID)
	if newKEKID == "" {
		return nil, errors.New("newKEKID cannot be empty")
	}

	// Validate context integrity
	canonContext, computedHash := CanonicalizeContext(payload.Context)
	if subtle.ConstantTimeCompare([]byte(payload.ContextHash), []byte(computedHash)) != 1 {
		return nil, fmt.Errorf("%w: context hash mismatch during re-wrap", ErrContextTampered)
	}

	if expectedContext != nil {
		for k, v := range expectedContext {
			if pv, ok := payload.Context[k]; !ok || pv != v {
				return nil, fmt.Errorf("%w: mismatch on context key %q", ErrContextTampered, k)
			}
		}
	}

	// Retrieve old KEK and unwrap existing DEK
	oldKEK, err := e.provider.GetKEK(payload.KEKID, payload.KEKVersion)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve source KEK: %w", err)
	}
	defer Zeroize(oldKEK)

	wrappedDEK, err := base64.StdEncoding.DecodeString(payload.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid wrapped_dek base64", ErrEnvelopeMalformed)
	}

	dekNonce, err := base64.StdEncoding.DecodeString(payload.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dek_nonce base64", ErrEnvelopeMalformed)
	}

	dek, err := e.UnwrapDEK(oldKEK, wrappedDEK, dekNonce, canonContext)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap DEK with source KEK: %w", err)
	}
	defer Zeroize(dek)

	// Retrieve target KEK
	newVersion, err := e.provider.CurrentVersion(newKEKID)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve active version for target KEK %s: %w", newKEKID, err)
	}

	newKEK, err := e.provider.GetKEK(newKEKID, newVersion)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve target KEK %s version %s: %w", newKEKID, newVersion, err)
	}
	defer Zeroize(newKEK)

	// Re-wrap the SAME DEK with the NEW KEK
	newWrappedDEK, newDEKNonce, err := e.WrapDEK(newKEK, dek, canonContext)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap DEK under new KEK: %w", err)
	}

	// Return new envelope with unchanged ciphertext and data nonce
	ctxCopy := make(map[string]string, len(payload.Context))
	for k, v := range payload.Context {
		ctxCopy[k] = v
	}

	return &EnvelopeSecretPayload{
		KEKID:       newKEKID,
		KEKVersion:  newVersion,
		Algorithm:   payload.Algorithm,
		WrappedDEK:  base64.StdEncoding.EncodeToString(newWrappedDEK),
		DEKNonce:    base64.StdEncoding.EncodeToString(newDEKNonce),
		Ciphertext:  payload.Ciphertext,
		DataNonce:   payload.DataNonce,
		Context:     ctxCopy,
		ContextHash: payload.ContextHash,
		CreatedAt:   time.Now().UTC(),
	}, nil
}
