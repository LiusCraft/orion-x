// Package apikey 是 API Key 的领域层（docs/api-key-design.md §5.1）：key 串的生成 /
// 解析 / 摘要、scope 目录与预设、授权判定、限流与计数聚合。
//
// 边界：不 import gin、不 import internal/billing。与仓储相接只通过本包的 Store 接口
// 和 store.APIKey 这个记录类型；限定在"生成 + 校验 + 聚合"这些纯逻辑里，
// 不认识 HTTP，也不认识账号余额。
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/store"
)

// Prefix 是 key 串的公开前缀。改它等于换一代凭证：老串会全部不被识别（§1 D3）。
const Prefix = "ox_sk_"

// key 串的形状：`ox_sk_<lookup>_<secret>_<crc>`，四段、下划线分隔（对齐 GitHub/Stripe，
// 下划线还让双击能选中整串）。
const (
	// lookupLength 是公开段的长度（10 位 base58 ≈ 58.6 bit），落库为 varchar(16)。
	lookupLength = 10
	// secretLength 是秘密段的长度（32 位 base58 ≈ 187 bit）：离线爆破不可行，
	// 所以存 sha256 就够，不需要慢哈希（§1 D4）。
	secretLength = 32
	// crcLength 是校验段的定长。CRC32 最大 4294967295，base58 六位（58^6 ≈ 3.8e10）足够。
	crcLength = 6
	// rawLength 是完整串的长度，形状校验的第一道闸。
	rawLength = len(Prefix) + lookupLength + 1 + secretLength + 1 + crcLength
)

// alphabet 是 base58（Bitcoin）字母表：去掉 0OIl 等易混字符，人工抄写 / 电话报 key
// 时少出错。代价是每字符熵略低（≈ 5.86 bit），32 字符的秘密段 ≈ 187 bit，仍远超
// 所需的 128 bit。
const (
	alphabet     = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	alphabetBase = uint64(len(alphabet))
)

// ErrInvalid 形状不对或查不到。两者对调用方是同一件事：这把 key 不能用。
var ErrInvalid = errors.New("apikey: invalid key")

// Generate 生成明文（仅此一次返回）与可落库的记录。
//
// 返回的 store.APIKey 带了 ID 与 Creator，只差入库：调用方拿到明文后不该再把它
// 放进任何返回值里（唯一例外是创建响应）。
func Generate(userID, name string, scopes []string, expiresAt *time.Time) (string, store.APIKey, error) {
	lookup, err := randomBase58(lookupLength)
	if err != nil {
		return "", store.APIKey{}, err
	}
	secret, err := randomBase58(secretLength)
	if err != nil {
		return "", store.APIKey{}, err
	}

	plaintext := Prefix + lookup + "_" + secret + "_" + crcSuffix(lookup, secret)

	return plaintext, store.APIKey{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Lookup:    lookup,
		Hash:      Digest(plaintext),
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		BaseModel: store.BaseModel{Creator: userID},
	}, nil
}

// Parse 只做形状校验：前缀、分段、字符集、长度、CRC。不打库。
//
// 校验段的两个收益都在这里兑现：外部扫描器可以离线判伪；服务端对畸形输入不打库
// 就能拒掉。单字符改动一定落在 8 bit 之内，CRC32 对这种错误是保证检出的。
func Parse(raw string) (lookup, secret string, ok bool) {
	if len(raw) != rawLength || !strings.HasPrefix(raw, Prefix) {
		return "", "", false
	}

	parts := strings.Split(raw[len(Prefix):], "_")
	if len(parts) != 3 {
		return "", "", false
	}
	lookup, secret, crc := parts[0], parts[1], parts[2]
	if len(lookup) != lookupLength || len(secret) != secretLength || len(crc) != crcLength {
		return "", "", false
	}
	if !isBase58(lookup) || !isBase58(secret) || !isBase58(crc) {
		return "", "", false
	}

	want := crcSuffix(lookup, secret)
	if subtle.ConstantTimeCompare([]byte(crc), []byte(want)) != 1 {
		return "", "", false
	}
	return lookup, secret, true
}

// LookupOf 从一把（可能畸形的）串里取出公开段，取不到就返回空串。
// 只给日志用：排障时靠它定位"是哪把 key 在报错"，而不用打印任何秘密。
func LookupOf(raw string) string {
	lookup, _, ok := Parse(raw)
	if !ok {
		return ""
	}
	return lookup
}

// Digest 返回落库用的摘要（sha256 hex，64 字符）。
func Digest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Mask 返回列表里给用户看的掩码形式：保留可辨识的公开段，秘密段一律隐去。
// 明文一旦离开创建响应就没有第二份，所以掩码只能由公开段拼出来。
func Mask(lookup string) string {
	return Prefix + lookup + "••••"
}

// crcSuffix 是串末的校验段：对 `lookup + "_" + secret`（不含前缀）取 CRC32(IEEE)，
// 左补零到 6 位 base58。它只用于"不打库就拒掉畸形输入"与让外部扫描器少报误判，
// 不承担安全职责。
func crcSuffix(lookup, secret string) string {
	sum := uint64(crc32.ChecksumIEEE([]byte(lookup + "_" + secret)))
	return encodeBase58(sum, crcLength)
}

func encodeBase58(v uint64, width int) string {
	out := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		out[i] = alphabet[v%alphabetBase]
		v /= alphabetBase
	}
	return string(out)
}

func isBase58(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}

// randomBase58 用拒绝采样取随机串：256 % 58 != 0，直接取模会让前几个字符偏多。
// 高熵随机串的均匀性是这个模块唯一的密码学假设，不能靠"差不多随机"。
func randomBase58(n int) (string, error) {
	// 232 = 58 * 4，最大的 58 的倍数；>= 232 的字节丢弃。
	const limit = 232
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("apikey: read random: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, alphabet[b%byte(alphabetBase)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}
