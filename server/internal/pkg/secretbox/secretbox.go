package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const prefix = "enc:v1:"

// Box 使用 AES-256-GCM 加密敏感配置（如数据源 DSN）。
type Box struct {
	aead cipher.AEAD
}

// New 由密钥派生 32 字节 AES 密钥并初始化 GCM。
func New(key string) (*Box, error) {
	sum := sha256.Sum256([]byte("bi-platform/secretbox/" + key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Encrypt 加密明文；空值与已加密值原样返回，保证幂等。
func (b *Box) Encrypt(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, prefix) {
		return plain, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密密文；非加密前缀的值按历史明文兼容处理。
func (b *Box) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, prefix) {
		// 兼容历史明文配置。
		return value, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", err
	}
	size := b.aead.NonceSize()
	if len(raw) < size {
		return "", errors.New("密文长度不正确")
	}
	plain, err := b.aead.Open(nil, raw[:size], raw[size:], nil)
	if err != nil {
		return "", errors.New("敏感配置解密失败，请检查 JWT_SECRET 是否变更")
	}
	return string(plain), nil
}
