package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

// fakeVoiceCloneMeter 是 billing.Meter 的内联实现（仓库约定：不引 mock 生成器）。
// 它只记下被调了什么，金额判断留给 service 的单测。
type fakeVoiceCloneMeter struct {
	reserveReqs []billing.ReserveRequest
	chargeReqs  []billing.ChargeRequest
	released    []string

	reserveErr  error
	reservation billing.Reservation

	chargeErr error
	charge    billing.ChargeResult
}

func (m *fakeVoiceCloneMeter) Reserve(_ context.Context, req billing.ReserveRequest) (billing.Reservation, error) {
	m.reserveReqs = append(m.reserveReqs, req)
	if m.reserveErr != nil {
		return billing.Reservation{}, m.reserveErr
	}
	reservation := m.reservation
	if reservation.ID == "" {
		reservation.ID = "resv-1"
	}
	if reservation.AmountMicro == 0 {
		reservation.AmountMicro = 2 * billing.MicroPerUnit
	}
	return reservation, nil
}

func (m *fakeVoiceCloneMeter) Charge(_ context.Context, req billing.ChargeRequest) (billing.ChargeResult, error) {
	m.chargeReqs = append(m.chargeReqs, req)
	if m.chargeErr != nil {
		return billing.ChargeResult{}, m.chargeErr
	}
	result := m.charge
	if result.PriceID == "" {
		result.PriceID = "price-clone"
		result.AmountMicro = 2 * billing.MicroPerUnit
	}
	return result, nil
}

func (m *fakeVoiceCloneMeter) Release(_ context.Context, reservationID string) error {
	m.released = append(m.released, reservationID)
	return nil
}

// cloneURLRequest 是一次用外部音频 URL 的合法复刻请求（legacy 路径要求显式 format）。
func cloneURLRequest() cloneVoiceRequest {
	return cloneVoiceRequest{
		Name:           "My voice",
		SourceAudioURL: "https://audio.test/a.wav",
		Format:         ttsprovider.FormatWAV,
	}
}

// cloneableSystemVoiceCloneModel 返回一个平台代付（Provider.IsSystem）的复刻模型：
// 只有这种模型才该计费（§7）。
func cloneableSystemVoiceCloneModel(t *testing.T) store.AIModel {
	t.Helper()
	registerVoiceCloneTestProvider()
	model := testVoiceCloneModel("model-system", "Cloneable", "user-1", "api-key")
	model.Provider.IsSystem = true
	return model
}

func newCloneServiceWithMeter(model store.AIModel, voices *fakeVoiceCloneVoiceStore, cloner *fakeVoiceCloner, meter billing.Meter) *providerVoiceCloneService {
	models := &fakeVoiceCloneModelStore{byID: map[string]*store.AIModel{model.ID: &model}}
	return newProviderVoiceCloneService(models, voices, nil, func(ttsprovider.ProviderConfig) (ttsprovider.Synthesizer, error) {
		return cloner, nil
	}, meter)
}

// 平台代付的复刻：Reserve → 厂商成功 → Charge，两段用同一个 ref，且不额外 Release
// （Charge 内部会捕获冻结）。
func TestVoiceCloneChargesPlatformClone(t *testing.T) {
	model := cloneableSystemVoiceCloneModel(t)
	voices := &fakeVoiceCloneVoiceStore{result: &store.ModelVoice{ID: "stored-voice", VoiceID: "provider-voice"}}
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	meter := &fakeVoiceCloneMeter{}
	svc := newCloneServiceWithMeter(model, voices, cloner, meter)

	voice, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if voice.ID != "stored-voice" {
		t.Fatalf("Clone() voice = %#v", voice)
	}

	if len(meter.reserveReqs) != 1 || len(meter.chargeReqs) != 1 {
		t.Fatalf("meter calls = %d reserve / %d charge, want 1 / 1", len(meter.reserveReqs), len(meter.chargeReqs))
	}
	reserve := meter.reserveReqs[0]
	if reserve.ItemCode != billing.ItemVoiceClone || reserve.Quantity != 1 {
		t.Errorf("reserve = %+v, want item %q quantity 1", reserve, billing.ItemVoiceClone)
	}
	if reserve.RefType != billing.RefVoiceClone || reserve.RefID == "" {
		t.Errorf("reserve ref = (%q, %q), want ref_type %q", reserve.RefType, reserve.RefID, billing.RefVoiceClone)
	}
	if reserve.SubjectType != billing.SubjectTypeUser || reserve.SubjectID != "user-1" {
		t.Errorf("reserve subject = (%q, %q), want (user, user-1)", reserve.SubjectType, reserve.SubjectID)
	}

	charge := meter.chargeReqs[0]
	if charge.RefType != reserve.RefType || charge.RefID != reserve.RefID {
		t.Errorf("charge ref = (%q, %q), want it to match reserve ref (%q, %q)",
			charge.RefType, charge.RefID, reserve.RefType, reserve.RefID)
	}
	if charge.Dims["model_id"] != model.ID || charge.Dims["voice_id"] != "stored-voice" {
		t.Errorf("charge dims = %#v, want model_id and voice_id", charge.Dims)
	}
	if len(meter.released) != 0 {
		t.Errorf("released = %v, want no explicit release after a successful charge", meter.released)
	}
}

// 厂商失败：释放预冻结，不计费。
func TestVoiceCloneReleasesReservationWhenVendorFails(t *testing.T) {
	model := cloneableSystemVoiceCloneModel(t)
	cloner := &fakeVoiceCloner{err: errors.New("provider exploded")}
	meter := &fakeVoiceCloneMeter{reservation: billing.Reservation{ID: "resv-42"}}
	svc := newCloneServiceWithMeter(model, &fakeVoiceCloneVoiceStore{}, cloner, meter)

	_, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest())
	if err == nil {
		t.Fatal("Clone() error = nil, want the provider error")
	}
	if len(meter.chargeReqs) != 0 {
		t.Errorf("charge calls = %d, want 0", len(meter.chargeReqs))
	}
	if len(meter.released) != 1 || meter.released[0] != "resv-42" {
		t.Fatalf("released = %v, want [resv-42]", meter.released)
	}
}

// BYOK：用户自带 key 复刻，整个链路不碰账户。
func TestVoiceCloneSkipsBillingForBYOK(t *testing.T) {
	registerVoiceCloneTestProvider()
	model := testVoiceCloneModel("model-byok", "BYOK", "user-1", "api-key") // Provider.IsSystem 默认 false
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	meter := &fakeVoiceCloneMeter{}
	svc := newCloneServiceWithMeter(model, &fakeVoiceCloneVoiceStore{}, cloner, meter)

	if _, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest()); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if len(meter.reserveReqs) != 0 || len(meter.chargeReqs) != 0 || len(meter.released) != 0 {
		t.Fatalf("meter calls = %d/%d/%d, want no billing for BYOK",
			len(meter.reserveReqs), len(meter.chargeReqs), len(meter.released))
	}
}

// 计费关闭（meter 为 nil）时复刻照常工作。
func TestVoiceCloneWithoutMeter(t *testing.T) {
	model := cloneableSystemVoiceCloneModel(t)
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	svc := newCloneServiceWithMeter(model, &fakeVoiceCloneVoiceStore{}, cloner, nil)

	if _, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest()); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
}

// 预冻结失败（余额不足）：不调厂商，错误原样带出去给 writeVoiceCloneError 映射成 402。
func TestVoiceCloneReserveFailureBlocksVendorCall(t *testing.T) {
	model := cloneableSystemVoiceCloneModel(t)
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	meter := &fakeVoiceCloneMeter{reserveErr: fmt.Errorf("wrapped: %w", service.ErrInsufficientBalance)}
	svc := newCloneServiceWithMeter(model, &fakeVoiceCloneVoiceStore{}, cloner, meter)

	_, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest())
	if !errors.Is(err, service.ErrInsufficientBalance) {
		t.Fatalf("Clone() error = %v, want ErrInsufficientBalance", err)
	}
	if cloner.request.TargetModel != "" {
		t.Errorf("vendor was called despite a failed reserve: %+v", cloner.request)
	}
}

// 厂商成功但扣费失败：结果照给（事件已落库，worker 会补扣），绝不静默。
func TestVoiceCloneChargeFailureStillReturnsVoice(t *testing.T) {
	model := cloneableSystemVoiceCloneModel(t)
	voices := &fakeVoiceCloneVoiceStore{result: &store.ModelVoice{ID: "stored-voice", VoiceID: "provider-voice"}}
	cloner := &fakeVoiceCloner{result: &ttsprovider.VoiceCloneResult{VoiceID: "provider-voice"}}
	meter := &fakeVoiceCloneMeter{chargeErr: errors.New("billing db is down")}
	svc := newCloneServiceWithMeter(model, voices, cloner, meter)

	voice, err := svc.Clone(context.Background(), model.ID, "user-1", cloneURLRequest())
	if err != nil {
		t.Fatalf("Clone() error = %v, want the successful clone to survive a charge failure", err)
	}
	if voice == nil || voice.ID != "stored-voice" {
		t.Fatalf("Clone() voice = %#v", voice)
	}
	if len(meter.chargeReqs) != 1 {
		t.Fatalf("charge calls = %d, want 1", len(meter.chargeReqs))
	}
}

func TestVoiceCloneRefID(t *testing.T) {
	base := cloneVoiceRequest{
		Name:           "My voice",
		Description:    "d",
		SourceAudioURL: "https://audio.test/a.wav",
		Format:         ttsprovider.FormatWAV,
		Extra:          map[string]any{"noise_reduction": true, "seed": 1},
	}

	cases := []struct {
		name     string
		userID   string
		modelID  string
		req      cloneVoiceRequest
		wantSame bool
	}{
		{name: "same request is stable", userID: "user-1", modelID: "model-1", req: base, wantSame: true},
		{
			// prefix 缺省是每次随机生成的，不能算进引用里，否则重试就是新操作。
			name:     "generated prefix does not change the ref",
			userID:   "user-1",
			modelID:  "model-1",
			req:      func() cloneVoiceRequest { r := base; r.Prefix = "vcdeadbeef"; return r }(),
			wantSame: true,
		},
		{
			name:    "different name is a different operation",
			userID:  "user-1",
			modelID: "model-1",
			req:     func() cloneVoiceRequest { r := base; r.Name = "Another"; return r }(),
		},
		{
			name:    "different user is a different operation",
			userID:  "user-2",
			modelID: "model-1",
			req:     base,
		},
		{
			name:    "different source is a different operation",
			userID:  "user-1",
			modelID: "model-1",
			req:     func() cloneVoiceRequest { r := base; r.SourceAudioURL = "https://audio.test/b.wav"; return r }(),
		},
		{
			name:    "different extra is a different operation",
			userID:  "user-1",
			modelID: "model-1",
			req:     func() cloneVoiceRequest { r := base; r.Extra = map[string]any{"noise_reduction": false}; return r }(),
		},
	}

	want := voiceCloneRefID("user-1", "model-1", base)
	if want == "" {
		t.Fatal("voiceCloneRefID() returned an empty ref")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := voiceCloneRefID(tc.userID, tc.modelID, tc.req)
			if tc.wantSame && got != want {
				t.Fatalf("voiceCloneRefID() = %q, want the stable ref %q", got, want)
			}
			if !tc.wantSame && got == want {
				t.Fatalf("voiceCloneRefID() = %q, want a ref different from %q", got, want)
			}
		})
	}

	// map 的迭代顺序不影响结果：json.Marshal 对 map 的键是排序输出的。
	shuffled := cloneVoiceRequest{Name: base.Name, Description: base.Description, SourceAudioURL: base.SourceAudioURL, Format: base.Format}
	shuffled.Extra = map[string]any{"seed": 1, "noise_reduction": true}
	if got := voiceCloneRefID("user-1", "model-1", shuffled); got != want {
		t.Fatalf("extra map order changed the ref: %q != %q", got, want)
	}
}
