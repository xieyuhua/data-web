package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const prefix = "v1:"

// Cipher encrypts sensitive values (datasource passwords) at rest with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives a 32-byte key by SHA-256 over the configured secret. A value
// prefixed with "derived:" is combined with the auth secret by the config layer.
func NewCipher(secret string) (*Cipher, error) {
	if secret == "" {
		return nil, errors.New("empty secret key")
	}
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	if !strings.HasPrefix(enc, prefix) {
		return "", errors.New("unsupported ciphertext format")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(enc, prefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
