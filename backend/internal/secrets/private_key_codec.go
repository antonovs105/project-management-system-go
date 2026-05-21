package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// encryptedPrivateKeyPrefix marks managed ciphertext rows.
const encryptedPrivateKeyPrefix = "enc:v1:"

// PrivateKeyCodec protects local ActivityPub private keys before persistence.
type PrivateKeyCodec interface {
	EncryptPrivateKey(value string) (string, error)
	DecryptPrivateKey(value string) (string, error)
}

// NewPrivateKeyCodec returns a no-op codec when no key is configured, otherwise
// an AES-GCM codec derived from the configured deployment secret.
func NewPrivateKeyCodec(rawKey string) (PrivateKeyCodec, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return NoopPrivateKeyCodec{}, nil
	}
	if len(rawKey) < 32 {
		return nil, fmt.Errorf("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY must be at least 32 characters")
	}
	sum := sha256.Sum256([]byte(rawKey))
	return AESGCMPrivateKeyCodec{key: sum[:]}, nil
}

// IsEncryptedPrivateKey reports whether value uses the managed ciphertext prefix.
func IsEncryptedPrivateKey(value string) bool {
	return strings.HasPrefix(value, encryptedPrivateKeyPrefix)
}

// NoopPrivateKeyCodec leaves private keys as plaintext for tests and local dev.
type NoopPrivateKeyCodec struct{}

// EncryptPrivateKey returns the original value.
func (NoopPrivateKeyCodec) EncryptPrivateKey(value string) (string, error) {
	return value, nil
}

// DecryptPrivateKey returns the original value.
func (NoopPrivateKeyCodec) DecryptPrivateKey(value string) (string, error) {
	return value, nil
}

// AESGCMPrivateKeyCodec encrypts and decrypts private keys with AES-GCM.
type AESGCMPrivateKeyCodec struct {
	key []byte
}

// EncryptPrivateKey encrypts a private key unless it already uses the managed prefix.
func (c AESGCMPrivateKeyCodec) EncryptPrivateKey(value string) (string, error) {
	if value == "" || IsEncryptedPrivateKey(value) {
		return value, nil
	}
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return encryptedPrivateKeyPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPrivateKey decrypts managed ciphertext and passes legacy plaintext through.
func (c AESGCMPrivateKeyCodec) DecryptPrivateKey(value string) (string, error) {
	if value == "" || !IsEncryptedPrivateKey(value) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrivateKeyPrefix))
	if err != nil {
		return "", err
	}
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted private key is malformed")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// gcm constructs an AES-GCM cipher from the derived key.
func (c AESGCMPrivateKeyCodec) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
