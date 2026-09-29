package main

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/channels/platform"
)

// TestNewQRBindersMatchesDescriptors 钉住「注册表声明 ⇔ 运行时装配」一致：
// 声明了 qr_binding 的平台必须有 Binder，反向 Binder 不能指向没声明的平台。
// 两个来源分开演进时，这条测试先红，而不是控制台先出现点不动的入口。
func TestNewQRBindersMatchesDescriptors(t *testing.T) {
	binders := newQRBinders(context.Background(), ChannelsConfig{})
	for _, desc := range platform.All() {
		if desc.QRBinding && binders[desc.Name] == nil {
			t.Errorf("platform %s declares qr_binding but no binder is registered", desc.Name)
		}
	}
	for name := range binders {
		desc, ok := platform.Get(name)
		if !ok {
			t.Errorf("binder registered for unknown platform %q", name)
			continue
		}
		if !desc.QRBinding {
			t.Errorf("binder registered for platform %q which does not declare qr_binding", name)
		}
	}
}

func TestNewQRBindersDisabledByConfig(t *testing.T) {
	off := false
	binders := newQRBinders(context.Background(), ChannelsConfig{
		QRBinding: QRBindingConfig{Enabled: &off},
	})
	if binders != nil {
		t.Fatalf("binders = %v, want nil when qr_binding is disabled", binders)
	}
}
