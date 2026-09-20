package store

import (
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestBillingTiersValue(t *testing.T) {
	tests := []struct {
		name     string
		tiers    BillingTiers
		wantNil  bool
		wantJSON string
	}{
		{name: "nil writes NULL", tiers: nil, wantNil: true},
		{name: "empty writes NULL", tiers: BillingTiers{}, wantNil: true},
		{
			name:     "single tier",
			tiers:    BillingTiers{{UpTo: 1000, UnitPriceMicro: 5}},
			wantJSON: `[{"up_to":1000,"unit_price_micro":5}]`,
		},
		{
			name:     "open ended last tier",
			tiers:    BillingTiers{{UpTo: 1000, UnitPriceMicro: 5}, {UpTo: 0, UnitPriceMicro: 3}},
			wantJSON: `[{"up_to":1000,"unit_price_micro":5},{"up_to":0,"unit_price_micro":3}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tiers.Value()
			if err != nil {
				t.Fatalf("Value: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("Value = %v, want nil", got)
				}
				return
			}
			raw, ok := got.([]byte)
			if !ok {
				t.Fatalf("Value type = %T, want []byte", got)
			}
			if string(raw) != tt.wantJSON {
				t.Errorf("Value = %s, want %s", raw, tt.wantJSON)
			}
		})
	}
}

func TestBillingTiersScan(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    BillingTiers
		wantErr bool
	}{
		{name: "NULL", value: nil, want: nil},
		{name: "empty bytes", value: []byte{}, want: nil},
		{name: "empty string", value: "", want: nil},
		{
			name:  "json bytes",
			value: []byte(`[{"up_to":1000,"unit_price_micro":5}]`),
			want:  BillingTiers{{UpTo: 1000, UnitPriceMicro: 5}},
		},
		{
			name:  "json string",
			value: `[{"up_to":0,"unit_price_micro":7}]`,
			want:  BillingTiers{{UpTo: 0, UnitPriceMicro: 7}},
		},
		{name: "empty array", value: []byte(`[]`), want: BillingTiers{}},
		{name: "invalid json", value: []byte(`[{"up_to":`), wantErr: true},
		{name: "unsupported type", value: int64(1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tiers BillingTiers
			err := tiers.Scan(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Scan: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if len(tiers) != len(tt.want) || !reflect.DeepEqual(tiers, tt.want) {
				t.Errorf("Scan = %+v, want %+v", tiers, tt.want)
			}
		})
	}
}

func TestBillingTiersRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		tiers BillingTiers
	}{
		{name: "nil", tiers: nil},
		{name: "empty", tiers: BillingTiers{}},
		{name: "tiers", tiers: BillingTiers{{UpTo: 1_000_000, UnitPriceMicro: 800_000}, {UpTo: 0, UnitPriceMicro: 500_000}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.tiers.Value()
			if err != nil {
				t.Fatalf("Value: %v", err)
			}
			var back BillingTiers
			if err := back.Scan(val); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if len(back) == 0 && len(tt.tiers) == 0 {
				return // NULL 与空数组都表示“没有阶梯”，两者互通
			}
			if !reflect.DeepEqual(back, tt.tiers) {
				t.Errorf("round trip = %+v, want %+v", back, tt.tiers)
			}
		})
	}
}

// TestBillingModelSchema 钉住表名、主键、唯一索引与易漂的列名。这些名字在
// billing/service 和 SQL 里都被直接引用，改 Go 字段名而不改 tag 会静默改列名。
func TestBillingModelSchema(t *testing.T) {
	tests := []struct {
		model     any
		table     string
		primary   []string
		columns   map[string]string   // Go 字段名 → 列名
		uniqueIdx map[string][]string // 索引名 → 按 priority 排好的列名
	}{
		{
			model:   BillingItem{},
			table:   "billing_items",
			primary: []string{"code"},
			columns: map[string]string{
				"Code": "code", "Name": "name", "ChargeMode": "charge_mode", "Unit": "unit",
				"MeterSource": "meter_source", "Enabled": "enabled", "IsSystem": "is_system",
			},
		},
		{
			model:   BillingPrice{},
			table:   "billing_prices",
			primary: []string{"id"},
			columns: map[string]string{
				"ItemCode": "item_code", "AccountID": "account_id", "ResourceType": "resource_type",
				"ResourceID": "resource_id", "EffectiveFrom": "effective_from", "EffectiveTo": "effective_to",
				"Currency": "currency", "UnitPriceMicro": "unit_price_micro", "UnitSize": "unit_size",
				"MinChargeMicro": "min_charge_micro", "Rounding": "rounding", "Tiers": "tiers",
			},
			uniqueIdx: map[string][]string{
				"idx_billing_price_scope": {"item_code", "account_id", "resource_type", "resource_id", "effective_from"},
			},
		},
		{
			model:   BillingAccount{},
			table:   "billing_accounts",
			primary: []string{"id"},
			columns: map[string]string{
				"ID": "id", "SubjectType": "subject_type", "SubjectID": "subject_id", "Currency": "currency",
				"BalanceMicro": "balance_micro", "FrozenMicro": "frozen_micro",
				"CreditLimitMicro": "credit_limit_micro", "Status": "status",
			},
			uniqueIdx: map[string][]string{
				"idx_billing_account_subject": {"subject_type", "subject_id"},
			},
		},
		{
			model:   BillingLedger{},
			table:   "billing_ledger", // 单数：表名由 TableName 钉死
			primary: []string{"id"},
			columns: map[string]string{
				"AccountID": "account_id", "Direction": "direction", "AmountMicro": "amount_micro",
				"BalanceAfterMicro": "balance_after_micro", "Kind": "kind", "ItemCode": "item_code",
				"RefType": "ref_type", "RefID": "ref_id", "IdempotencyKey": "idempotency_key",
				"OccurredAt": "occurred_at", "Note": "note", "Creator": "creator",
			},
			uniqueIdx: map[string][]string{"idx_billing_ledger_idempotency_key": {"idempotency_key"}},
		},
		{
			model:   BillingUsageEvent{},
			table:   "billing_usage_events",
			primary: []string{"id"},
			columns: map[string]string{
				"ID": "id", "AccountID": "account_id", "VoicebotID": "voicebot_id", "DeviceID": "device_id",
				"SessionID": "session_id", "ItemCode": "item_code", "Quantity": "quantity",
				"AIModelID": "aimodel_id", "ProviderID": "provider_id", "VoiceID": "voice_id", "BYOK": "byok",
				"OccurredAt": "occurred_at", "ReceivedAt": "received_at", "TurnIndex": "turn_index",
				"Dimensions": "dimensions", "Status": "status", "PriceID": "price_id",
				"AmountMicro": "amount_micro", "LastError": "last_error",
			},
		},
		{
			model:   BillingReservation{},
			table:   "billing_reservations",
			primary: []string{"id"},
			columns: map[string]string{
				"SessionID": "session_id", "AccountID": "account_id", "DeviceID": "device_id",
				"VoicebotID": "voicebot_id", "ReservedMicro": "reserved_micro", "Status": "status",
				"ExpiresAt": "expires_at", "ResourceSnapshot": "resource_snapshot",
				"PriceSnapshot": "price_snapshot", "SettledAt": "settled_at",
				"ReleasedMicro": "released_micro", "LastError": "last_error",
			},
			uniqueIdx: map[string][]string{"idx_billing_reservations_session_id": {"session_id"}},
		},
		{
			model:   BillingGrant{},
			table:   "billing_grants",
			primary: []string{"id"},
			columns: map[string]string{
				"AccountID": "account_id", "ItemCode": "item_code", "Source": "source",
				"GrantedMicro": "granted_micro", "UsedMicro": "used_micro", "ExpiresAt": "expires_at",
			},
			uniqueIdx: map[string][]string{
				"idx_billing_grant_scope": {"account_id", "item_code", "source"},
			},
		},
		{
			model:   BillingPeriodState{},
			table:   "billing_period_states",
			primary: []string{"account_id", "item_code", "price_id", "period_start"},
			columns: map[string]string{
				"AccountID": "account_id", "ItemCode": "item_code", "PriceID": "price_id",
				"PeriodStart": "period_start", "Quantity": "quantity", "AmountMicro": "amount_micro",
				"GrantUsedMicro": "grant_used_micro", "UpdatedAt": "updated_at",
			},
		},
		{
			model:   BillingDailyStat{},
			table:   "billing_daily_stats",
			primary: []string{"id"},
			columns: map[string]string{
				"AccountID": "account_id", "Date": "date", "ItemCode": "item_code",
				"AIModelID": "aimodel_id", "VoiceID": "voice_id", "Quantity": "quantity",
				"AmountMicro": "amount_micro", "EventCount": "event_count",
			},
			uniqueIdx: map[string][]string{
				"idx_billing_daily_scope": {"account_id", "date", "item_code", "aimodel_id", "voice_id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			s, err := schema.Parse(tt.model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("parse schema: %v", err)
			}
			if s.Table != tt.table {
				t.Errorf("table = %q, want %q", s.Table, tt.table)
			}
			if !reflect.DeepEqual(s.PrimaryFieldDBNames, tt.primary) {
				t.Errorf("primary keys = %v, want %v", s.PrimaryFieldDBNames, tt.primary)
			}
			for name, want := range tt.columns {
				field := s.LookUpField(name)
				if field == nil {
					t.Errorf("field %s not found", name)
					continue
				}
				if field.DBName != want {
					t.Errorf("field %s: column = %q, want %q", name, field.DBName, want)
				}
			}

			indexes := map[string]*schema.Index{}
			for _, idx := range s.ParseIndexes() {
				indexes[idx.Name] = idx
			}
			for name, want := range tt.uniqueIdx {
				idx := indexes[name]
				if idx == nil {
					t.Errorf("index %s not found", name)
					continue
				}
				if idx.Class != "UNIQUE" {
					t.Errorf("index %s class = %q, want UNIQUE", name, idx.Class)
				}
				got := make([]string, 0, len(idx.Fields))
				for _, f := range idx.Fields {
					got = append(got, f.DBName)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("index %s columns = %v, want %v", name, got, want)
				}
			}
		})
	}
}
