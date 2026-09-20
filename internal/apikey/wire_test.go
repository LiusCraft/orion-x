package apikey

import (
	"net/http"
	"testing"
)

func TestDecide(t *testing.T) {
	const owner = "user-1"

	cases := []struct {
		name  string
		facts Facts
		want  RejectReason
	}{
		{
			name:  "owner with the voice scope gets in",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeVoice}, DeviceFound: true, DeviceOwnerID: owner},
			want:  RejectNone,
		},
		{
			name:  "scope all covers voice too",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeAll}, DeviceFound: true, DeviceOwnerID: owner},
			want:  RejectNone,
		},
		{
			name:  "unknown key",
			facts: Facts{DeviceFound: true, DeviceOwnerID: owner},
			want:  RejectKeyNotFound,
		},
		{
			name:  "key without the voice scope",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeAgent}, DeviceFound: true, DeviceOwnerID: owner},
			want:  RejectScopeMissing,
		},
		{
			name:  "read-only key cannot open a voice session",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeRead}, DeviceFound: true, DeviceOwnerID: owner},
			want:  RejectScopeMissing,
		},
		{
			name:  "unknown device",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeVoice}, DeviceFound: false},
			want:  RejectDeviceNotKnown,
		},
		{
			name:  "device belongs to someone else",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: []Scope{ScopeVoice}, DeviceFound: true, DeviceOwnerID: "user-2"},
			want:  RejectOwnerMismatch,
		},
		{
			name:  "key without a scope never reaches the device check",
			facts: Facts{KeyFound: true, KeyOwnerID: owner, KeyScopes: nil, DeviceFound: true, DeviceOwnerID: "user-2"},
			want:  RejectScopeMissing,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.facts); got != tc.want {
				t.Errorf("Decide() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAllowsScope(t *testing.T) {
	cases := []struct {
		name     string
		scopes   []Scope
		required Scope
		want     bool
	}{
		{name: "all covers voice", scopes: []Scope{ScopeAll}, required: ScopeVoice, want: true},
		{name: "exact match", scopes: []Scope{ScopeVoice}, required: ScopeVoice, want: true},
		{name: "other namespace does not cover", scopes: []Scope{ScopeMCP}, required: ScopeVoice, want: false},
		{name: "read does not cover a namespace by itself", scopes: []Scope{ScopeRead}, required: ScopeVoice, want: false},
		{name: "one of several matches", scopes: []Scope{ScopeData, ScopeVoice}, required: ScopeVoice, want: true},
		{name: "no scopes", scopes: nil, required: ScopeVoice, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllowsScope(tc.scopes, tc.required); got != tc.want {
				t.Errorf("AllowsScope(%v, %q) = %v, want %v", tc.scopes, tc.required, got, tc.want)
			}
		})
	}

	// 方法维度不参与 AllowsScope：GET 也不能用只读范围换到语音接入。
	if AllowsScope([]Scope{ScopeRead}, ScopeVoice) {
		t.Error("AllowsScope must not be method-dependent")
	}
	// Allows 仍然保留“只读放行安全方法”的语义。
	if !Allows([]Scope{ScopeRead}, ScopeMCP, http.MethodGet) {
		t.Error("Allows() with the read scope must still pass safe methods")
	}
	if Allows([]Scope{ScopeRead}, ScopeMCP, http.MethodPost) {
		t.Error("Allows() must reject writes for the read scope")
	}
}

func TestRejectReasonsAreDistinct(t *testing.T) {
	reasons := []RejectReason{
		RejectNone, RejectKeyNotFound, RejectScopeMissing, RejectKeyMissing,
		RejectKeyUnavailable, RejectDeviceNotKnown, RejectOwnerMismatch,
	}
	seen := make(map[RejectReason]struct{}, len(reasons))
	for _, r := range reasons {
		if _, dup := seen[r]; dup {
			t.Errorf("duplicate reject reason %q", r)
		}
		seen[r] = struct{}{}
		if len(r) > 60 {
			// Close 帧的 reason 有 123 字节上限，这里留足余量。
			t.Errorf("reject reason %q is too long for a close frame", r)
		}
	}
	if RejectNone != "" {
		t.Errorf("RejectNone = %q, want the empty string", RejectNone)
	}
}

func TestPathAuthorize(t *testing.T) {
	// 路径是数据面与控制面之间的约定，改动必须是双向的：常量在领域层，两边都引用它。
	if PathAuthorize != "/internal/apikey/authorize" {
		t.Errorf("PathAuthorize = %q", PathAuthorize)
	}
}
