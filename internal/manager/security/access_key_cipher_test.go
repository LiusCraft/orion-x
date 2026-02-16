package security

import (
	"strings"
	"testing"
)

func TestAESCipher_EncryptDecrypt(t *testing.T) {
	cipher, err := NewAESCipher("unit-test-secret")
	if err != nil {
		t.Fatalf("NewAESCipher() error = %v", err)
	}

	input := "sk-test-123"
	encrypted, err := cipher.Encrypt(input)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if encrypted == "" {
		t.Fatalf("expected non-empty encrypted payload")
	}
	if strings.Contains(encrypted, input) {
		t.Fatalf("expected encrypted payload to hide plaintext")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != input {
		t.Fatalf("expected decrypted %q, got %q", input, decrypted)
	}
}

func TestAESCipher_EncryptNonceRandomized(t *testing.T) {
	cipher, err := NewAESCipher("unit-test-secret")
	if err != nil {
		t.Fatalf("NewAESCipher() error = %v", err)
	}

	first, err := cipher.Encrypt("sk-test-123")
	if err != nil {
		t.Fatalf("Encrypt() first error = %v", err)
	}
	second, err := cipher.Encrypt("sk-test-123")
	if err != nil {
		t.Fatalf("Encrypt() second error = %v", err)
	}
	if first == second {
		t.Fatalf("expected randomized encryption output")
	}
}

func TestAESCipher_InvalidInputs(t *testing.T) {
	if _, err := NewAESCipher("   "); err == nil {
		t.Fatalf("expected NewAESCipher to fail on empty secret")
	}

	cipher, err := NewAESCipher("unit-test-secret")
	if err != nil {
		t.Fatalf("NewAESCipher() error = %v", err)
	}

	if _, err := cipher.Decrypt("bad***payload"); err == nil {
		t.Fatalf("expected decrypt to fail on invalid payload")
	}
}
