package apikey

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestGenerateFormatAndUniqueness(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 64; i++ {
		plain, err := Generate()
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if !strings.HasPrefix(plain, Prefix) {
			t.Fatalf("Generate() = %q, want prefix %q", plain, Prefix)
		}
		secret := strings.TrimPrefix(plain, Prefix)
		if len(secret) != secretSize*2 {
			t.Fatalf("secret length = %d, want %d", len(secret), secretSize*2)
		}
		if _, err := hex.DecodeString(secret); err != nil {
			t.Fatalf("secret %q is not hex: %v", secret, err)
		}
		if _, dup := seen[plain]; dup {
			t.Fatalf("Generate() returned a duplicate key %q", plain)
		}
		seen[plain] = struct{}{}
	}
}

func TestHash(t *testing.T) {
	plain, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	sum := Hash(plain)
	if len(sum) != 64 {
		t.Fatalf("Hash() length = %d, want 64", len(sum))
	}
	if Hash(plain) != sum {
		t.Error("Hash() is not deterministic")
	}
	if Hash(plain+"x") == sum {
		t.Error("Hash() collided for different inputs")
	}
	if strings.Contains(sum, strings.TrimPrefix(plain, Prefix)) {
		t.Error("Hash() must not embed the plaintext")
	}
}

func TestLooksLike(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "generated key", raw: Prefix + "0123456789abcdef", want: true},
		{name: "jwt-like token", raw: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig", want: false},
		{name: "empty", raw: "", want: false},
		{name: "prefix only", raw: Prefix, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLike(tc.raw); got != tc.want {
				t.Errorf("LooksLike(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDisplayAndMasked(t *testing.T) {
	plain, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	prefix, last4, err := Display(plain)
	if err != nil {
		t.Fatalf("Display() error = %v", err)
	}
	if want := plain[:len(Prefix)+displayPrefixSize]; prefix != want {
		t.Errorf("Display() prefix = %q, want %q", prefix, want)
	}
	if want := plain[len(plain)-4:]; last4 != want {
		t.Errorf("Display() last4 = %q, want %q", last4, want)
	}
	masked := Masked(prefix, last4)
	if masked != prefix+"..."+last4 {
		t.Errorf("Masked() = %q", masked)
	}
	secret := strings.TrimPrefix(plain, Prefix)
	if middle := secret[displayPrefixSize : len(secret)-4]; strings.Contains(masked, middle) {
		t.Error("Masked() leaked the middle of the secret")
	}

	invalid := []string{
		"",
		"ox:sk:",
		"eyJhbGciOiJIUzI1NiJ9.token",
		Prefix + "short",
		Prefix + strings.Repeat("a", displayPrefixSize+3),
	}
	for _, raw := range invalid {
		if _, _, err := Display(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("Display(%q) error = %v, want ErrInvalid", raw, err)
		}
	}

	if got := Masked("", ""); got != "" {
		t.Errorf("Masked() with no parts = %q, want empty", got)
	}
}

func TestParseScopes(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    []Scope
		wantErr error
	}{
		{name: "single", in: []string{"apikey:scope:mcp"}, want: []Scope{ScopeMCP}},
		{
			name: "dedupes and trims",
			in:   []string{" apikey:scope:agent", "apikey:scope:agent", "apikey:scope:read"},
			want: []Scope{ScopeAgent, ScopeRead},
		},
		{name: "empty", in: nil, wantErr: ErrInvalidScope},
		{name: "unknown", in: []string{"apikey:scope:admin"}, wantErr: ErrInvalidScope},
		{name: "free text", in: []string{"全部权限"}, wantErr: ErrInvalidScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseScopes(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ParseScopes(%v) error = %v, want %v", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseScopes(%v) error = %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseScopes(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("ParseScopes(%v)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestScopeStringsRoundTrip(t *testing.T) {
	scopes := []Scope{ScopeRead, ScopeData}
	raw := Strings(scopes)
	if len(raw) != 2 || raw[0] != "apikey:scope:read" || raw[1] != "apikey:scope:data" {
		t.Fatalf("Strings() = %v", raw)
	}
	if got := FromStrings(raw); len(got) != 2 || got[0] != ScopeRead || got[1] != ScopeData {
		t.Errorf("FromStrings(Strings()) = %v", got)
	}
	if got := FromStrings(nil); len(got) != 0 {
		t.Errorf("FromStrings(nil) = %v, want empty", got)
	}
}

func TestAllScopesIsACopy(t *testing.T) {
	first := AllScopes()
	if len(first) != len(allScopes) {
		t.Fatalf("AllScopes() length = %d, want %d", len(first), len(allScopes))
	}
	first[0] = "tampered"
	if AllScopes()[0] != allScopes[0] {
		t.Error("AllScopes() exposed the internal table")
	}
	for _, s := range allScopes {
		if !s.Valid() {
			t.Errorf("scope %q in the table is not Valid()", s)
		}
	}
	if Scope("apikey:scope:nope").Valid() {
		t.Error("unknown scope reported as valid")
	}
}

func TestAllows(t *testing.T) {
	cases := []struct {
		name     string
		scopes   []Scope
		required Scope
		method   string
		want     bool
	}{
		{name: "all covers any method", scopes: []Scope{ScopeAll}, required: ScopeMCP, method: "POST", want: true},
		{name: "read covers GET", scopes: []Scope{ScopeRead}, required: ScopeData, method: "GET", want: true},
		{name: "read covers HEAD", scopes: []Scope{ScopeRead}, required: ScopeAgent, method: "HEAD", want: true},
		{name: "read does not cover POST", scopes: []Scope{ScopeRead}, required: ScopeData, method: "POST", want: false},
		{name: "read does not cover DELETE", scopes: []Scope{ScopeRead}, required: ScopeAgent, method: "DELETE", want: false},
		{name: "matching scope covers write", scopes: []Scope{ScopeAgent}, required: ScopeAgent, method: "POST", want: true},
		{name: "other scope does not cover", scopes: []Scope{ScopeAgent}, required: ScopeMCP, method: "POST", want: false},
		{name: "any of several scopes matches", scopes: []Scope{ScopeMCP, ScopeData}, required: ScopeData, method: "DELETE", want: true},
		{name: "no scopes", scopes: nil, required: ScopeData, method: "GET", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Allows(tc.scopes, tc.required, tc.method); got != tc.want {
				t.Errorf("Allows(%v, %q, %q) = %v, want %v", tc.scopes, tc.required, tc.method, got, tc.want)
			}
		})
	}
}
