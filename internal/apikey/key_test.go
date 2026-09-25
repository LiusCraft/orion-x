package apikey

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strings"
	"testing"
	"time"
)

func TestGenerateShapeAndParse(t *testing.T) {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	plaintext, record, err := Generate("user-1", "生产环境", []string{ScopeAgentRead}, &expires)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(plaintext) != rawLength {
		t.Errorf("len(plaintext) = %d, want %d", len(plaintext), rawLength)
	}
	if !strings.HasPrefix(plaintext, Prefix) {
		t.Errorf("plaintext %q must start with %q", plaintext, Prefix)
	}
	// 布局固定为 ox_sk_<lookup:10>_<secret:32>_<crc:6>：格式冻结，这条断言是闸门。
	segments := strings.Split(strings.TrimPrefix(plaintext, Prefix), "_")
	if len(segments) != 3 ||
		len(segments[0]) != lookupLength || len(segments[1]) != secretLength || len(segments[2]) != crcLength {
		t.Fatalf("key layout = %q, want ox_sk_<lookup:%d>_<secret:%d>_<crc:%d>",
			plaintext, lookupLength, secretLength, crcLength)
	}

	lookup, secret, ok := Parse(plaintext)
	if !ok {
		t.Fatalf("Parse(%q) rejected a freshly generated key", plaintext)
	}
	if lookup != record.Lookup {
		t.Errorf("Parse lookup = %q, want %q", lookup, record.Lookup)
	}
	if len(secret) != secretLength {
		t.Errorf("len(secret) = %d, want %d", len(secret), secretLength)
	}

	if want := Digest(plaintext); record.Hash != want {
		t.Errorf("record.Hash = %q, want sha256 hex %q", record.Hash, want)
	}
	if len(record.Hash) != 64 {
		t.Errorf("len(record.Hash) = %d, want 64", len(record.Hash))
	}
	if record.UserID != "user-1" || record.Creator != "user-1" {
		t.Errorf("record owner = (%q, %q), want user-1 twice", record.UserID, record.Creator)
	}
	if record.Name != "生产环境" {
		t.Errorf("record.Name = %q", record.Name)
	}
	if record.ID == "" {
		t.Error("record.ID must be set (API 路径用它，不复用 lookup)")
	}
	if record.ExpiresAt == nil || !record.ExpiresAt.Equal(expires) {
		t.Errorf("record.ExpiresAt = %v, want %v", record.ExpiresAt, expires)
	}
	if record.RevokedAt != nil || record.LastUsedAt != nil || record.CallCount != 0 {
		t.Errorf("fresh record must be active and unused: %+v", record)
	}
}

// 明文只存在于创建响应里：记录本身（含 JSON 序列化结果）不能出现完整串。
func TestGenerateRecordNeverCarriesPlaintext(t *testing.T) {
	plaintext, record, err := Generate("user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal(record) error = %v", err)
	}
	if strings.Contains(string(raw), plaintext) {
		t.Fatalf("marshalled record leaks the plaintext: %s", raw)
	}
	if strings.Contains(fmt.Sprintf("%+v", record), plaintext) {
		t.Fatalf("record struct leaks the plaintext: %+v", record)
	}
	// 秘密段按定义不能落在任何字段里：只有 lookup 是公开的。
	_, secret, _ := Parse(plaintext)
	if strings.Contains(fmt.Sprintf("%+v", record), secret) {
		t.Fatal("record struct leaks the secret segment")
	}
}

func TestGenerateUnique(t *testing.T) {
	const n = 200
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		plaintext, record, err := Generate("user-1", "k", []string{ScopeAgentRead}, nil)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if _, dup := seen[plaintext]; dup {
			t.Fatalf("duplicate key generated: %s", plaintext)
		}
		seen[plaintext] = struct{}{}
		if _, _, ok := Parse(plaintext); !ok {
			t.Fatalf("Parse rejected generated key %s", plaintext)
		}
		if record.Lookup == "" {
			t.Fatal("record.Lookup must not be empty")
		}
	}
}

// 单字符改动必须被校验段拒掉：这是"不打库就拒畸形输入"的全部价值所在。
func TestParseRejectsSingleCharacterMutations(t *testing.T) {
	plaintext, _, err := Generate("user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	replacements := []byte{'0', '9', 'A', 'a', 'z', 'Z'}
	for i := 0; i < len(plaintext); i++ {
		for _, repl := range replacements {
			if repl == plaintext[i] {
				continue
			}
			mutated := plaintext[:i] + string(repl) + plaintext[i+1:]
			if _, _, ok := Parse(mutated); ok {
				t.Fatalf("Parse accepted %q (position %d changed to %q)", mutated, i, repl)
			}
		}
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	plaintext, _, err := Generate("user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	lookup, secret, _ := Parse(plaintext)

	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "prefix only", raw: Prefix},
		{name: "truncated", raw: plaintext[:len(plaintext)-1]},
		{name: "extra suffix", raw: plaintext + "x"},
		{name: "wrong prefix", raw: "ox_pk_" + plaintext[len(Prefix):]},
		{name: "no prefix", raw: plaintext[len(Prefix):]},
		{name: "lookup only", raw: Prefix + lookup},
		{name: "missing crc", raw: Prefix + lookup + "_" + secret + "_"},
		{name: "wrong crc", raw: Prefix + lookup + "_" + secret + "_000000"},
		{name: "hyphen is not base62", raw: Prefix + strings.Replace(plaintext, "_", "-", 1)},
		{name: "uppercase prefix", raw: "OX_SK_" + plaintext[len(Prefix):]},
		{name: "jwt shaped", raw: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.sig"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := Parse(tc.raw); ok {
				t.Fatalf("Parse(%q) = ok, want rejection", tc.raw)
			}
		})
	}

	// "wrong crc" 那条只是碰巧不对；用真算出来的另一个 CRC 才证明校验段在起作用。
	other := encodeBase58(uint64(crc32.ChecksumIEEE([]byte("something else"))), crcLength)
	if other == plaintext[len(plaintext)-crcLength:] {
		t.Fatal("crc fixture collided with the generated key")
	}
	if _, _, ok := Parse(Prefix + lookup + "_" + secret + "_" + other); ok {
		t.Fatal("Parse accepted a valid-looking but wrong crc")
	}

	// 前缀不进校验段：把它拼回去就该被拒。
	if _, _, ok := Parse(Prefix + Prefix + lookup + "_" + secret + "_" + crcSuffix(lookup, secret)); ok {
		t.Fatal("Parse accepted a key with a doubled prefix")
	}
}

func TestParseAcceptsKnownVector(t *testing.T) {
	// 固定向量：格式一旦发布就冻结，这条断言是闸门。`1URfjp` 是用独立实现
	// （python zlib + base58）算出来的，不是拿 crcSuffix 自证。
	lookup := "123456789A"
	secret := strings.Repeat("1", secretLength)
	raw := Prefix + lookup + "_" + secret + "_1URfjp"

	gotLookup, gotSecret, ok := Parse(raw)
	if !ok {
		t.Fatalf("Parse(%q) rejected the fixed vector", raw)
	}
	if gotLookup != lookup || gotSecret != secret {
		t.Fatalf("Parse = (%q, %q), want (%q, %q)", gotLookup, gotSecret, lookup, secret)
	}
	if len(raw) != rawLength {
		t.Fatalf("fixed vector length = %d, want %d", len(raw), rawLength)
	}
	if got := crcSuffix(lookup, secret); got != "1URfjp" {
		t.Fatalf("crcSuffix = %q, want %q", got, "1URfjp")
	}
	// 校验段只盖 lookup+secret：前缀不进 CRC。
	if crcSuffix(lookup, secret) == crcSuffix(lookup[:len(lookup)-1]+"B", secret) {
		t.Fatal("crc must depend on the lookup")
	}
}

// 字母表是 base58（Bitcoin）：去掉 0OIl 等易混字符。改字母表等于换一代凭证。
func TestAlphabetIsBase58(t *testing.T) {
	if len(alphabet) != 58 {
		t.Fatalf("len(alphabet) = %d, want 58", len(alphabet))
	}
	seen := make(map[rune]bool, len(alphabet))
	for _, c := range alphabet {
		if seen[c] {
			t.Fatalf("alphabet repeats %q", c)
		}
		seen[c] = true
	}
	for _, banned := range []rune{'0', 'O', 'I', 'l'} {
		if seen[banned] {
			t.Errorf("alphabet must not contain %q (易混字符)", banned)
		}
	}
	if alphabetBase != 58 {
		t.Fatalf("alphabetBase = %d, want 58", alphabetBase)
	}
}

func TestLookupOf(t *testing.T) {
	plaintext, record, err := Generate("user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got := LookupOf(plaintext); got != record.Lookup {
		t.Errorf("LookupOf(plaintext) = %q, want %q", got, record.Lookup)
	}
	if got := LookupOf("garbage"); got != "" {
		t.Errorf("LookupOf(garbage) = %q, want empty", got)
	}
}

func TestMask(t *testing.T) {
	plaintext, record, err := Generate("user-1", "ci", []string{ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	masked := Mask(record.Lookup)
	if !strings.HasPrefix(masked, Prefix+record.Lookup) {
		t.Errorf("Mask(%q) = %q, want it to keep the lookup", record.Lookup, masked)
	}
	_, secret, _ := Parse(plaintext)
	if strings.Contains(masked, secret) {
		t.Errorf("Mask(%q) = %q leaks the secret", record.Lookup, masked)
	}
}
