package apikey

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// scopeValuePattern 是命名深度的硬约束：默认两段，最多三段（§5.3）。
var scopeValuePattern = regexp.MustCompile(`^[a-z][a-z0-9]*:[a-z][a-z0-9]*(:[a-z][a-z0-9]*)?$`)

func TestCatalogIntegrity(t *testing.T) {
	seen := make(map[string]bool, len(catalog))
	groupValues := make(map[string]bool, len(groups))
	for _, g := range groups {
		if groupValues[g.Value] {
			t.Errorf("duplicate group %q", g.Value)
		}
		groupValues[g.Value] = true
		if strings.TrimSpace(g.Title) == "" {
			t.Errorf("group %q has no title", g.Value)
		}
	}

	for _, s := range catalog {
		if seen[s.Value] {
			t.Errorf("duplicate scope %q", s.Value)
		}
		seen[s.Value] = true
		if !scopeValuePattern.MatchString(s.Value) {
			t.Errorf("scope %q violates the resource:action naming rule", s.Value)
		}
		if strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Description) == "" {
			t.Errorf("scope %q must carry a title and a description (前端不硬编码文案)", s.Value)
		}
		if !groupValues[s.Group] {
			t.Errorf("scope %q references unknown group %q", s.Value, s.Group)
		}
	}
	// 容量假设：目录 ≤ 200 条且只增（§3.2）；超了先想清楚分类，而不是继续加。
	if len(catalog) > 200 {
		t.Errorf("catalog has %d entries, capacity assumption is <= 200", len(catalog))
	}
}

func TestPresetsReferenceCatalog(t *testing.T) {
	known := make(map[string]ScopeInfo, len(catalog))
	for _, s := range catalog {
		known[s.Value] = s
	}

	for _, p := range presets {
		if len(p.Scopes) == 0 {
			t.Errorf("preset %q has no scopes", p.Name)
		}
		if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Description) == "" {
			t.Errorf("preset %q must carry a title and a description", p.Name)
		}
		seen := make(map[string]bool, len(p.Scopes))
		for _, s := range p.Scopes {
			info, ok := known[s]
			if !ok {
				t.Errorf("preset %q references unknown scope %q", p.Name, s)
				continue
			}
			if info.Deprecated {
				t.Errorf("preset %q references deprecated scope %q", p.Name, s)
			}
			if seen[s] {
				t.Errorf("preset %q lists %q twice", p.Name, s)
			}
			seen[s] = true
		}
	}
}

// "默认勾选什么"和"只读预设给什么"必须是同一批：两套说法迟早会漂移。
func TestReadonlyPresetEqualsDefaults(t *testing.T) {
	var defaults []string
	for _, s := range catalog {
		if s.Default {
			defaults = append(defaults, s.Value)
		}
	}

	var readonly []string
	for _, p := range presets {
		if p.Name == "readonly" {
			readonly = append(readonly, p.Scopes...)
		}
	}
	if len(readonly) == 0 {
		t.Fatal("readonly preset is missing")
	}

	sort.Strings(defaults)
	sort.Strings(readonly)
	if strings.Join(defaults, ",") != strings.Join(readonly, ",") {
		t.Fatalf("readonly preset = %v, catalog defaults = %v — 两者必须一致", readonly, defaults)
	}

	// 只读就是只读：预设里不该混进任何写权限。
	for _, s := range readonly {
		if !strings.HasSuffix(s, ":read") {
			t.Errorf("readonly preset contains non-read scope %q", s)
		}
	}
}

// 会扣费的 model:write 不进任何内置预设：预设是"少想一步"的便利，不是隐性授权。
func TestPresetsExcludeBillableScopes(t *testing.T) {
	for _, p := range presets {
		for _, s := range p.Scopes {
			if s == ScopeModelWrite {
				t.Errorf("preset %q must not grant %s (音色克隆会扣费)", p.Name, s)
			}
		}
	}
}

func TestMissing(t *testing.T) {
	cases := []struct {
		name     string
		granted  []string
		required []string
		want     []string
	}{
		{name: "deny by default", granted: nil, required: []string{ScopeAgentRead}, want: []string{ScopeAgentRead}},
		{name: "empty requirement passes", granted: []string{ScopeAgentRead}, required: nil, want: nil},
		{name: "exact match", granted: []string{ScopeAgentRead}, required: []string{ScopeAgentRead}},
		{name: "AND semantics", granted: []string{ScopeAgentRead}, required: []string{ScopeAgentRead, ScopeAgentWrite}, want: []string{ScopeAgentWrite}},
		{name: "no wildcard", granted: []string{"agent:*"}, required: []string{ScopeAgentRead}, want: []string{ScopeAgentRead}},
		{name: "no prefix implication", granted: []string{ScopeAgentWrite}, required: []string{ScopeAgentRead}, want: []string{ScopeAgentRead}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Missing(tc.granted, tc.required)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("Missing(%v, %v) = %v, want %v", tc.granted, tc.required, got, tc.want)
			}
			if wantAllowed := len(tc.want) == 0; HasAll(tc.granted, tc.required) != wantAllowed {
				t.Fatalf("HasAll(%v, %v) = %v, want %v", tc.granted, tc.required, !wantAllowed, wantAllowed)
			}
		})
	}
}

func TestNormalizeScopes(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    []string
		wantErr error
	}{
		{
			name: "dedupe and catalog order",
			in:   []string{ScopeDataRead, ScopeAgentRead, ScopeDataRead},
			want: []string{ScopeAgentRead, ScopeDataRead},
		},
		{
			name: "blank entries are dropped",
			in:   []string{"", "  ", ScopeAgentRead},
			want: []string{ScopeAgentRead},
		},
		{name: "empty list", in: nil, wantErr: ErrNoScopes},
		{name: "only blanks", in: []string{" ", ""}, wantErr: ErrNoScopes},
		{name: "unknown scope", in: []string{"agent:admin"}, wantErr: ErrUnknownScope},
		{name: "wildcard is not a scope", in: []string{"*"}, wantErr: ErrUnknownScope},
		{name: "preset name is not a scope", in: []string{"readonly"}, wantErr: ErrUnknownScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeScopes(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("NormalizeScopes(%v) error = %v, want %v", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeScopes(%v) error = %v", tc.in, err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("NormalizeScopes(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeScopesRejectsDeprecated(t *testing.T) {
	// 废弃态：历史 key 仍可持有，但不能授予新 key。目录是编译期常量，临时改一条再还原。
	const probe = ScopeAgentRead
	original := catalog[0]
	t.Cleanup(func() { catalog[0] = original })

	catalog[0].Deprecated = true
	if _, err := NormalizeScopes([]string{probe}); !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("NormalizeScopes(deprecated) error = %v, want ErrUnknownScope", err)
	}
}

func TestExpandPreset(t *testing.T) {
	scopes, ok := ExpandPreset("readonly")
	if !ok || len(scopes) == 0 {
		t.Fatalf("ExpandPreset(readonly) = (%v, %v), want a non-empty list", scopes, ok)
	}
	// 返回的是副本：调用方改它不该影响下一次展开。
	scopes[0] = "mutated"
	again, _ := ExpandPreset("readonly")
	if again[0] == "mutated" {
		t.Fatal("ExpandPreset leaks its internal slice")
	}
	if _, ok := ExpandPreset("no-such-preset"); ok {
		t.Fatal("ExpandPreset accepted an unknown preset")
	}
}

func TestCatalogCopyIsDetached(t *testing.T) {
	out := Catalog()
	out[0].Title = "mutated"
	if Catalog()[0].Title == "mutated" {
		t.Fatal("Catalog() must return a copy")
	}
}
