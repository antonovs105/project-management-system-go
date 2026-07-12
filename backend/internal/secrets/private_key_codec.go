package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const (
	// encryptedPrivateKeyV1Prefix marks legacy ciphertext without a key identifier.
	encryptedPrivateKeyV1Prefix = "enc:v1:"
	// encryptedPrivateKeyV2Prefix marks ciphertext that names its encryption key.
	encryptedPrivateKeyV2Prefix = "enc:v2:"
)

// PrivateKeyCodec protects local ActivityPub private keys before persistence.
type PrivateKeyCodec interface {
	EncryptPrivateKey(value string) (string, error)
	DecryptPrivateKey(value string) (string, error)
}

// NewPrivateKeyCodec returns a no-op codec when no key is configured. The first
// key encrypts new values; remaining keys are retained for non-disruptive
// rotation and can decrypt both key-ID-aware and legacy ciphertext.
func NewPrivateKeyCodec(rawKey string, previousKeys ...string) (PrivateKeyCodec, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return NoopPrivateKeyCodec{}, nil
	}
	allKeys := append([]string{rawKey}, previousKeys...)
	codec := AESGCMPrivateKeyCodec{keys: make(map[string][]byte, len(allKeys))}
	for index, value := range allKeys {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) < 32 {
			return nil, fmt.Errorf("actor private key encryption keys must be at least 32 characters")
		}
		sum := sha256.Sum256([]byte(value))
		keyID := hex.EncodeToString(sum[:8])
		codec.keys[keyID] = sum[:]
		if index == 0 {
			codec.primaryKeyID = keyID
		}
	}
	return codec, nil
}

// IsEncryptedPrivateKey reports whether value uses the managed ciphertext prefix.
func IsEncryptedPrivateKey(value string) bool {
	return strings.HasPrefix(value, encryptedPrivateKeyV1Prefix) || strings.HasPrefix(value, encryptedPrivateKeyV2Prefix)
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
	primaryKeyID string
	keys         map[string][]byte
}

// EncryptPrivateKey encrypts a private key unless it already uses the managed prefix.
func (c AESGCMPrivateKeyCodec) EncryptPrivateKey(value string) (string, error) {
	if value == "" || IsEncryptedPrivateKey(value) {
		return value, nil
	}
	gcm, err := c.gcm(c.keys[c.primaryKeyID])
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return encryptedPrivateKeyV2Prefix + c.primaryKeyID + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPrivateKey decrypts managed ciphertext and passes legacy plaintext through.
func (c AESGCMPrivateKeyCodec) DecryptPrivateKey(value string) (string, error) {
	if value == "" || !IsEncryptedPrivateKey(value) {
		return value, nil
	}
	if strings.HasPrefix(value, encryptedPrivateKeyV2Prefix) {
		parts := strings.SplitN(strings.TrimPrefix(value, encryptedPrivateKeyV2Prefix), ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("encrypted private key is malformed")
		}
		key, ok := c.keys[parts[0]]
		if !ok {
			return "", fmt.Errorf("encrypted private key references unavailable key %q", parts[0])
		}
		return c.open(parts[1], key)
	}

	// V1 did not persist a key identifier. Try every configured key so an
	// instance can introduce a new primary before rewriting legacy rows.
	payload := strings.TrimPrefix(value, encryptedPrivateKeyV1Prefix)
	for _, key := range c.keys {
		plaintext, err := c.open(payload, key)
		if err == nil {
			return plaintext, nil
		}
	}
	return "", fmt.Errorf("encrypted private key cannot be decrypted with configured keys")
}

// gcm constructs an AES-GCM cipher from the derived key.
func (c AESGCMPrivateKeyCodec) gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// open decodes and authenticates one AES-GCM payload.
func (c AESGCMPrivateKeyCodec) open(payload string, key []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}
	gcm, err := c.gcm(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted private key is malformed")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
