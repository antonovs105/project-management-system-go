package httpsig

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

const (
	AlgorithmRSAV15SHA256 = "rsa-v1_5-sha256"
	legacyRSAAlgorithm    = "rsa-sha256"
)

var (
	ErrInvalidPrivateKey = errors.New("invalid rsa private key")
	ErrInvalidPublicKey  = errors.New("invalid rsa public key")
)

type ActorKey struct {
	ActorID       string `db:"actor_id"`
	ActorAPID     string `db:"actor_ap_id"`
	KeyID         string `db:"key_id"`
	Algorithm     string `db:"algorithm"`
	PublicKeyPEM  string `db:"public_key_pem"`
	PrivateKeyPEM string `db:"private_key_pem"`
}

func (k ActorKey) SignatureAlgorithm() (string, error) {
	switch strings.ToLower(strings.TrimSpace(k.Algorithm)) {
	case "", AlgorithmRSAV15SHA256, legacyRSAAlgorithm:
		return AlgorithmRSAV15SHA256, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, k.Algorithm)
	}
}

func ParseRSAPrivateKeyPEM(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, ErrInvalidPrivateKey
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPrivateKey, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrInvalidPrivateKey
	}
	return key, nil
}

func ParseRSAPublicKeyPEM(raw string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, ErrInvalidPublicKey
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		key, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, ErrInvalidPublicKey
		}
		return key, nil
	}

	key, pkcs1Err := x509.ParsePKCS1PublicKey(block.Bytes)
	if pkcs1Err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}
	return key, nil
}
