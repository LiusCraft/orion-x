package apikey

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testBoxKey(t *testing.T) []byte {
	t.Helper()
	key, err := DeriveSecretKey([]byte("master-secret"))
	if err != nil {
		t.Fatalf("DeriveSecretKey() error = %v", err)
	}
	if len(key) != SecretKeySize {
		t.Fatalf("derived key length = %d, want %d", len(key), SecretKeySize)
	}
	return key
}

func TestDeriveSecretKey(t *testing.T) {
	key := testBoxKey(t)

	again, err := DeriveSecretKey([]byte("master-secret"))
	if err != nil {
		t.Fatalf("DeriveSecretKey() error = %v", err)
	}
	if string(again) != string(key) {
		t.Error("derivation is not deterministic")
	}

	other, err := DeriveSecretKey([]byte("another-master"))
	if err != nil {
		t.Fatalf("DeriveSecretKey() error = %v", err)
	}
	if string(other) == string(key) {
		t.Error("different master secrets derived the same key")
	}

	if _, err := DeriveSecretKey(nil); !errors.Is(err, ErrSecretUnavailable) {
		t.Errorf("DeriveSecretKey(nil) error = %v, want ErrSecretUnavailable", err)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := testBoxKey(t)
	plain, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	sealed, err := Seal(key, plain)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if !strings.HasPrefix(sealed, secretBoxVersion+":") {
		t.Errorf("Seal() = %q, want the version prefix", sealed)
	}
	if strings.Contains(sealed, plain) || strings.Contains(sealed, strings.TrimPrefix(plain, Prefix)) {
		t.Errorf("Seal() leaked the plaintext: %q", sealed)
	}
	// 同一个明文两次封装必须给出不同密文（随机 nonce），否则能看出两行是同一把钥。
	second, err := Seal(key, plain)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if second == sealed {
		t.Error("Seal() reused the nonce")
	}

	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened != plain {
		t.Errorf("Open() = %q, want %q", opened, plain)
	}
}

func TestOpenRejectsTamperedAndForeignCiphertext(t *testing.T) {
	key := testBoxKey(t)
	sealed, err := Seal(key, Prefix+strings.Repeat("ab", 24))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	payload := strings.TrimPrefix(sealed, secretBoxVersion+":")
	blob, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	blob[len(blob)-1] ^= 0xff
	tampered := secretBoxVersion + ":" + base64.StdEncoding.EncodeToString(blob)

	otherKey := testBoxKey(t)
	otherKey[0] ^= 0xff

	cases := []struct {
		name   string
		key    []byte
		sealed string
	}{
		{name: "tampered ciphertext", key: key, sealed: tampered},
		{name: "foreign key", key: otherKey, sealed: sealed},
		{name: "no version prefix", key: key, sealed: payload},
		{name: "unknown version", key: key, sealed: "v9:" + payload},
		{name: "empty payload", key: key, sealed: secretBoxVersion + ":"},
		{name: "not base64", key: key, sealed: secretBoxVersion + ":!!!!"},
		{name: "too short", key: key, sealed: secretBoxVersion + ":" + base64.StdEncoding.EncodeToString([]byte("short"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Open(tc.key, tc.sealed); !errors.Is(err, ErrDecrypt) {
				t.Errorf("Open(%q) error = %v, want ErrDecrypt", tc.sealed, err)
			}
		})
	}

	if _, err := Open(nil, sealed); !errors.Is(err, ErrSecretUnavailable) {
		t.Errorf("Open() without a box key error = %v, want ErrSecretUnavailable", err)
	}
	if _, err := Seal(nil, "x"); !errors.Is(err, ErrSecretUnavailable) {
		t.Errorf("Seal() without a box key error = %v, want ErrSecretUnavailable", err)
	}
}
