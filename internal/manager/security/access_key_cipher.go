package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

type AccessKeyCipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type AESCipher struct {
	aead cipher.AEAD
	rand io.Reader
}

func NewAESCipher(secret string) (*AESCipher, error) {
	return newAESCipherWithRand(secret, rand.Reader)
}

func newAESCipherWithRand(secret string, random io.Reader) (*AESCipher, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("access key cipher secret must not be empty")
	}
	if random == nil {
		return nil, errors.New("random source is required")
	}

	derived := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new aes gcm: %w", err)
	}

	return &AESCipher{aead: aead, rand: random}, nil
}

func (c *AESCipher) Encrypt(plaintext string) (string, error) {
	if c == nil || c.aead == nil || c.rand == nil {
		return "", errors.New("cipher is not initialized")
	}
	if strings.TrimSpace(plaintext) == "" {
		return "", errors.New("plaintext must not be empty")
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(c.rand, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}

	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (c *AESCipher) Decrypt(ciphertext string) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("cipher is not initialized")
	}
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return "", errors.New("ciphertext must not be empty")
	}

	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.New("ciphertext is invalid")
	}

	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", errors.New("ciphertext payload is invalid")
	}

	nonce := payload[:nonceSize]
	sealed := payload[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", errors.New("ciphertext verification failed")
	}

	return string(plain), nil
}
