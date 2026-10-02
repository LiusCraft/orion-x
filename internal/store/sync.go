package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/language"
	"github.com/liuscraft/orion-x/internal/logging"
	asrprovider "github.com/liuscraft/orion-x/internal/provider/asr"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"

	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
)

// SyncSystemProviders 对比代码中注册的 ProviderMeta 与数据库中 source=code 的记录，
// 使用 meta_hash 判断是否需要更新，并自动新增/更新 Provider、Model、Voice。
//
// 管理边界：只有 source=code（代码注册表写入）的记录归 sync 管。is_system=true 只表示
// 记录对所有用户可见、后台不允许删除；管理员从控制台创建后标成官方的记录
// （source=manual）不在清理范围内，即使其 slug 不在代码注册表里也不能删。
// 匹配（按 slug / model_id）同样跳过 source=manual：同名的管理员上架记录不会被
// 更新或接管，它与内置记录算两条独立记录（同一 slug 多账号，见 models.go）。
//
// 规则：
//   - 新增：代码中有但数据库中没有 → CREATE（source=code）
//   - 更新：hash 不一致 → UPDATE
//   - 保留：hash 一致 → 跳过
//   - 删除：source=code 但代码中没有 → DELETE（先 voices、再 models、最后 providers）
//
// 清理是尽力而为的收尾动作：记录仍被引用（音色挂在模型下、模型挂在 provider 下）
// 或删除失败时只告警跳过，不返回错误、不阻断启动。
func SyncSystemProviders(db *gorm.DB) error {
	logging.Infof("store: syncing system providers...")

	// ── 1. 收集代码中所有注册的 system provider ──
	type codeModel struct {
		modelID  string
		name     string
		baseURL  string
		metaHash string
		langs    pq.StringArray
		modType  ModelType
		voices   []ttsprovider.VoiceInfo
	}

	type codeProvider struct {
		slug        string
		name        string
		baseURL     string
		description string
		metaHash    string
		// models
		models []codeModel
	}

	var expected []codeProvider

	// TTS providers
	for key, meta := range ttsprovider.ListRegistered() {
		cp := codeProvider{
			slug:        "tts:" + key,
			name:        meta.Name,
			baseURL:     meta.DefaultBaseURL,
			description: meta.Description,
			metaHash:    meta.ContentHash,
		}
		for modelName, info := range meta.Models {
			cm := codeModel{
				modelID:  modelName,
				name:     modelName,
				baseURL:  meta.DefaultBaseURL,
				metaHash: meta.ContentHash,
				modType:  ModelTypeSpeech,
				voices:   info.SystemVoices,
			}
			for _, lang := range info.SupportedLanguages {
				cm.langs = append(cm.langs, string(lang))
			}
			cp.models = append(cp.models, cm)
		}
		expected = append(expected, cp)
	}

	// LLM providers
	for key, meta := range llmprovider.DefaultRegistry().ListRegistered() {
		cp := codeProvider{
			slug:     "llm:" + key,
			name:     meta.Name,
			baseURL:  meta.DefaultBaseURL,
			metaHash: meta.ContentHash,
		}
		for modelName, info := range meta.Models {
			cm := codeModel{
				modelID:  modelName,
				name:     modelName,
				baseURL:  meta.DefaultBaseURL,
				metaHash: meta.ContentHash,
				modType:  ModelTypeText,
			}
			for _, lang := range info.SupportedLanguages {
				cm.langs = append(cm.langs, string(lang))
			}
			cp.models = append(cp.models, cm)
		}
		expected = append(expected, cp)
	}

	// ASR providers
	for key, meta := range asrprovider.ListRegistered() {
		cp := codeProvider{
			slug:     "asr:" + key,
			name:     meta.Name,
			baseURL:  meta.DefaultBaseURL,
			metaHash: meta.ContentHash,
		}
		for modelName, info := range meta.Models {
			cm := codeModel{
				modelID:  modelName,
				name:     modelName,
				baseURL:  meta.DefaultBaseURL,
				metaHash: meta.ContentHash,
				modType:  ModelTypeSpeech,
			}
			for _, lang := range info.SupportedLanguages {
				cm.langs = append(cm.langs, string(lang))
			}
			cp.models = append(cp.models, cm)
		}
		expected = append(expected, cp)
	}

	// ── 2. 查询数据库现有系统记录 ──
	var dbProviders []Provider
	if err := db.Where("is_system = true").Find(&dbProviders).Error; err != nil {
		return fmt.Errorf("sync: query system providers: %w", err)
	}

	var dbModels []AIModel
	if err := db.Where("is_system = true").Find(&dbModels).Error; err != nil {
		return fmt.Errorf("sync: query system models: %w", err)
	}

	var dbVoices []ModelVoice
	if err := db.Where("is_system = true").Find(&dbVoices).Error; err != nil {
		return fmt.Errorf("sync: query system voices: %w", err)
	}

	// 建立匹配索引。source=manual 的行（管理员上架的官方记录）不参与匹配：sync 不
	// 更新、不接管它们，同 slug / 同 model_id 下各算一条独立记录。source='' 是
	// source 列上线前的遗留行，当 sync 自己的记录处理。
	dbProvBySlug := make(map[string]*Provider, len(dbProviders))
	for i := range dbProviders {
		if dbProviders[i].Source == SourceManual {
			continue
		}
		dbProvBySlug[dbProviders[i].Slug] = &dbProviders[i]
	}
	dbModelByKey := make(map[string]*AIModel, len(dbModels)) // key = "providerID|modelID"
	for i := range dbModels {
		if dbModels[i].Source == SourceManual {
			continue
		}
		dbModelByKey[dbModels[i].ProviderID+"|"+dbModels[i].ModelID] = &dbModels[i]
	}
	dbVoiceByKey := make(map[string]*ModelVoice, len(dbVoices)) // key = "modelID|voiceID"
	for i := range dbVoices {
		if dbVoices[i].Source == SourceManual {
			continue
		}
		dbVoiceByKey[dbVoices[i].ModelID+"|"+dbVoices[i].VoiceID] = &dbVoices[i]
	}

	codeProvSlugs := make(map[string]bool)
	codeModelKeys := make(map[string]bool) // "providerID|modelID"
	codeVoiceKeys := make(map[string]bool) // "modelID|voiceID"

	var addedProvs, updatedProvs, addedModels, updatedModels, addedVoices, updatedVoices int

	// ── 3. 逐 provider 同步 ──
	for _, cp := range expected {
		codeProvSlugs[cp.slug] = true

		// --- Provider ---
		var provID string
		if existing, ok := dbProvBySlug[cp.slug]; ok {
			provID = existing.ID
			if existing.MetaHash != cp.metaHash || existing.Name != cp.name || existing.BaseURL != cp.baseURL || existing.Source != SourceCode {
				if err := db.Model(&Provider{}).Where("id = ?", provID).Updates(map[string]any{
					"name":      cp.name,
					"base_url":  cp.baseURL,
					"meta_hash": cp.metaHash,
					"source":    SourceCode,
				}).Error; err != nil {
					return fmt.Errorf("sync: update provider %s: %w", cp.slug, err)
				}
				updatedProvs++
				logging.Infof("store: sync updated provider %s", cp.slug)
			}
		} else {
			provID = uuid.NewString()
			p := &Provider{
				ID:       provID,
				Name:     cp.name,
				Slug:     cp.slug,
				BaseURL:  cp.baseURL,
				IsSystem: true,
				Source:   SourceCode,
				MetaHash: cp.metaHash,
				BaseModel: BaseModel{
					Creator: "system",
				},
			}
			if err := db.Create(p).Error; err != nil {
				return fmt.Errorf("sync: create provider %s: %w", cp.slug, err)
			}
			addedProvs++
			logging.Infof("store: sync added provider %s", cp.slug)
		}

		// --- Models ---
		for _, cm := range cp.models {
			modelKey := provID + "|" + cm.modelID
			codeModelKeys[modelKey] = true

			var modelID string
			if existing, ok := dbModelByKey[modelKey]; ok {
				modelID = existing.ID
				if existing.MetaHash != cm.metaHash || existing.Name != cm.name || existing.Source != SourceCode {
					if err := db.Model(&AIModel{}).Where("id = ?", modelID).Updates(map[string]any{
						"name":      cm.name,
						"base_url":  cm.baseURL,
						"langs":     pq.StringArray(cm.langs),
						"meta_hash": cm.metaHash,
						"source":    SourceCode,
					}).Error; err != nil {
						return fmt.Errorf("sync: update model %s: %w", cm.modelID, err)
					}
					updatedModels++
					logging.Infof("store: sync updated model %s/%s", cp.slug, cm.modelID)
				}
			} else {
				modelID = uuid.NewString()
				m := &AIModel{
					ID:         modelID,
					ProviderID: provID,
					Name:       cm.name,
					Type:       cm.modType,
					BaseURL:    cm.baseURL,
					ModelID:    cm.modelID,
					IsSystem:   true,
					Source:     SourceCode,
					Langs:      cm.langs,
					MetaHash:   cm.metaHash,
					BaseModel: BaseModel{
						Creator: "system",
					},
				}
				if err := db.Create(m).Error; err != nil {
					return fmt.Errorf("sync: create model %s: %w", cm.modelID, err)
				}
				addedModels++
				logging.Infof("store: sync added model %s/%s", cp.slug, cm.modelID)
			}

			// --- Voices ---
			for _, vi := range cm.voices {
				voiceKey := modelID + "|" + vi.VoiceID
				codeVoiceKeys[voiceKey] = true

				// 计算单条 voice 的 hash
				voiceHash := vi.MetaHash()

				if existing, ok := dbVoiceByKey[voiceKey]; ok {
					if existing.MetaHash != voiceHash || existing.Name != vi.Name || existing.Source != SourceCode {
						updates := map[string]any{
							"name":        vi.Name,
							"description": vi.Description,
							"gender":      mapVoiceGender(vi.Gender),
							"preview_url": vi.SampleURL,
							"tags":        pq.StringArray(vi.Tags),
							"langs":       voiceLangArray(vi.Languages),
							"meta_hash":   voiceHash,
							"source":      SourceCode,
						}
						if len(vi.Emotions) > 0 {
							updates["emotions"] = datatypes.JSONMap{"list": vi.Emotions}
						}
						if err := db.Model(&ModelVoice{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
							return fmt.Errorf("sync: update voice %s/%s: %w", cm.modelID, vi.VoiceID, err)
						}
						updatedVoices++
					}
				} else {
					v := &ModelVoice{
						ID:          uuid.NewString(),
						ModelID:     modelID,
						VoiceID:     vi.VoiceID,
						Name:        vi.Name,
						Description: vi.Description,
						Gender:      mapVoiceGender(vi.Gender),
						PreviewURL:  vi.SampleURL,
						Tags:        pq.StringArray(vi.Tags),
						Langs:       voiceLangArray(vi.Languages),
						IsSystem:    true,
						Source:      SourceCode,
						MetaHash:    voiceHash,
						BaseModel: BaseModel{
							Creator: "system",
						},
					}
					if len(vi.Emotions) > 0 {
						v.Emotions = datatypes.JSONMap{"list": vi.Emotions}
					}
					if err := db.Create(v).Error; err != nil {
						return fmt.Errorf("sync: create voice %s/%s: %w", cm.modelID, vi.VoiceID, err)
					}
					addedVoices++
				}
			}
		}
	}

	// ── 4. 清理多余的 code 记录：先子后父（voices → models → providers）──
	// 顺序反了在有引用时必撞外键；仍被引用或删除失败只告警跳过——清理是
	// 收尾动作，不能拖垮启动（2026-09-30 的启动 FATAL 就是删 provider 撞上的）。
	plan := planStaleCleanup(dbProviders, dbModels, dbVoices, codeProvSlugs, codeModelKeys, codeVoiceKeys)
	var removedProvs, removedModels, removedVoices int

	for _, existing := range plan.voices {
		if err := db.Where("id = ?", existing.ID).Delete(&ModelVoice{}).Error; err != nil {
			logging.Warnf("store: sync keeps stale voice %s/%s: delete: %v", existing.ModelID, existing.VoiceID, err)
			continue
		}
		removedVoices++
		logging.Infof("store: sync removed stale voice %s", existing.VoiceID)
	}

	for _, existing := range plan.models {
		// 还有音色挂着（外键会挡住，而且多半是管理员自建的）时不删模型。
		var voices int64
		if err := db.Model(&ModelVoice{}).Where("model_id = ?", existing.ID).Count(&voices).Error; err != nil {
			logging.Warnf("store: sync keeps stale model %s: count voices: %v", existing.ModelID, err)
			continue
		}
		if voices > 0 {
			logging.Warnf("store: sync keeps stale model %s: still referenced by %d voice(s)", existing.ModelID, voices)
			continue
		}
		if err := db.Where("id = ?", existing.ID).Delete(&AIModel{}).Error; err != nil {
			logging.Warnf("store: sync keeps stale model %s: delete: %v", existing.ModelID, err)
			continue
		}
		removedModels++
		logging.Infof("store: sync removed stale model %s", existing.ModelID)
	}

	for _, existing := range plan.providers {
		// 还有模型挂着（多半是管理员自建的）时不删 provider：删了会撞外键，
		// 也等于静默剁掉管理员的数据。
		var models int64
		if err := db.Model(&AIModel{}).Where("provider_id = ?", existing.ID).Count(&models).Error; err != nil {
			logging.Warnf("store: sync keeps stale provider %s: count models: %v", existing.Slug, err)
			continue
		}
		if models > 0 {
			logging.Warnf("store: sync keeps stale provider %s: still referenced by %d model(s)", existing.Slug, models)
			continue
		}
		if err := db.Where("id = ?", existing.ID).Delete(&Provider{}).Error; err != nil {
			logging.Warnf("store: sync keeps stale provider %s: delete: %v", existing.Slug, err)
			continue
		}
		removedProvs++
		logging.Infof("store: sync removed stale provider %s", existing.Slug)
	}

	logging.Infof("store: sync done — providers (+%d ~%d -%d) models (+%d ~%d -%d) voices (+%d ~%d -%d)",
		addedProvs, updatedProvs, removedProvs,
		addedModels, updatedModels, removedModels,
		addedVoices, updatedVoices, removedVoices)
	return nil
}

// staleRecords 是按删除顺序（先子后父）组织的一批待清理记录。
type staleRecords struct {
	voices    []ModelVoice
	models    []AIModel
	providers []Provider
}

// planStaleCleanup 挑出「归代码注册表管（source=code）但代码中已不存在」的记录。
//
// is_system=true 只表示对所有用户可见，管理员从后台创建、之后标成官方的记录
// （source=manual）不归 sync 管：哪怕 slug/model_id 不在注册表里也一概保留。
func planStaleCleanup(providers []Provider, models []AIModel, voices []ModelVoice, codeProvSlugs, codeModelKeys, codeVoiceKeys map[string]bool) staleRecords {
	var plan staleRecords
	for _, p := range providers {
		if p.Source == SourceCode && !codeProvSlugs[p.Slug] {
			plan.providers = append(plan.providers, p)
		}
	}
	for _, m := range models {
		if m.Source == SourceCode && !codeModelKeys[m.ProviderID+"|"+m.ModelID] {
			plan.models = append(plan.models, m)
		}
	}
	for _, v := range voices {
		if v.Source == SourceCode && !codeVoiceKeys[v.ModelID+"|"+v.VoiceID] {
			plan.voices = append(plan.voices, v)
		}
	}
	return plan
}

func mapVoiceGender(g string) VoiceGender {
	switch g {
	case "male":
		return VoiceGenderMale
	case "female":
		return VoiceGenderFemale
	case "neutral":
		return VoiceGenderNeutral
	default:
		return ""
	}
}

func voiceLangArray(langs []language.Code) pq.StringArray {
	arr := make(pq.StringArray, len(langs))
	for i, c := range langs {
		arr[i] = string(c)
	}
	return arr
}
