package apikey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// 密钥封套：明文密钥在库里以 AES-256-GCM 密文形态保存（key_secret 列），
// 只服务于“验证账号密码后再复制一次”这条路径；认证本身仍然走 SHA-256 摘要，
// 因此封装密钥缺失或轮换都不会让已有密钥失效，只会让复制不可用。
const (
	secretBoxVersion = "v1"
	// secretBoxInfo 是 HKDF 的用途标签：同一个主密钥在主密钥派生的其它用途
	// （JWT 签名等）之间必须互不影响，标签一变就派生出完全不同的密钥。
	secretBoxInfo = "orion-x:apikey:secretbox:v1"
	// SecretKeySize 是封装密钥长度（AES-256）。
	SecretKeySize = 32
)

// ErrSecretUnavailable 表示封装密钥缺失或非法（没配置主密钥、或这行是旧版本签发的）。
var ErrSecretUnavailable = errors.New("apikey: secret box key is not configured")

// ErrDecrypt 表示密文无法解开：被截断、被篡改，或用了另一把封装密钥。
var ErrDecrypt = errors.New("apikey: cannot decrypt secret")

// DeriveSecretKey 从部署主密钥派生出封装密钥。
func DeriveSecretKey(master []byte) ([]byte, error) {
	if len(master) == 0 {
		return nil, ErrSecretUnavailable
	}
	key, err := hkdf.Key(sha256.New, master, nil, secretBoxInfo, SecretKeySize)
	if err != nil {
		return nil, fmt.Errorf("apikey: derive secret box key: %w", err)
	}
	return key, nil
}

// Seal 把明文封装成可落库的密文（随机 nonce 前置，带版本前缀）。
func Seal(key []byte, plain string) (string, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("apikey: seal: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return secretBoxVersion + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open 解出 Seal 写下的明文。
func Open(key []byte, sealed string) (string, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}
	payload, ok := strings.CutPrefix(sealed, secretBoxVersion+":")
	if !ok || payload == "" {
		return "", ErrDecrypt
	}
	blob, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(blob) <= aead.NonceSize() {
		return "", ErrDecrypt
	}
	plain, err := aead.Open(nil, blob[:aead.NonceSize()], blob[aead.NonceSize():], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != SecretKeySize {
		return nil, ErrSecretUnavailable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("apikey: aes: %w", err)
	}
	return cipher.NewGCM(block)
}
