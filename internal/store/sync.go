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

// SyncSystemProviders 对比代码中注册的 ProviderMeta 与数据库中 IsSystem=true 的记录，
// 使用 meta_hash 判断是否需要更新，并自动新增/更新 Provider、Model、Voice。
//
// 规则：
//   - 新增：代码中有但数据库中没有 → CREATE
//   - 更新：hash 不一致 → UPDATE
//   - 保留：hash 一致 → 跳过
//   - 删除：数据库中 IsSystem=true 但代码中没有 → DELETE（级联删除其 Models 和 Voices）
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
			slug:        "tts/" + key,
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
			slug:     "llm/" + key,
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
			slug:     "asr/" + key,
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

	// 建立索引
	dbProvBySlug := make(map[string]*Provider, len(dbProviders))
	for i := range dbProviders {
		dbProvBySlug[dbProviders[i].Slug] = &dbProviders[i]
	}
	dbModelByKey := make(map[string]*AIModel, len(dbModels)) // key = "providerID|modelID"
	for i := range dbModels {
		dbModelByKey[dbModels[i].ProviderID+"|"+dbModels[i].ModelID] = &dbModels[i]
	}
	dbVoiceByKey := make(map[string]*ModelVoice, len(dbVoices)) // key = "modelID|voiceID"
	for i := range dbVoices {
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
			if existing.MetaHash != cp.metaHash || existing.Name != cp.name || existing.BaseURL != cp.baseURL {
				if err := db.Model(&Provider{}).Where("id = ?", provID).Updates(map[string]any{
					"name":      cp.name,
					"base_url":  cp.baseURL,
					"meta_hash": cp.metaHash,
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
				if existing.MetaHash != cm.metaHash || existing.Name != cm.name {
					if err := db.Model(&AIModel{}).Where("id = ?", modelID).Updates(map[string]any{
						"name":      cm.name,
						"base_url":  cm.baseURL,
						"langs":     pq.StringArray(cm.langs),
						"meta_hash": cm.metaHash,
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
					if existing.MetaHash != voiceHash || existing.Name != vi.Name {
						updates := map[string]any{
							"name":        vi.Name,
							"description": vi.Description,
							"gender":      mapVoiceGender(vi.Gender),
							"preview_url": vi.SampleURL,
							"tags":        pq.StringArray(vi.Tags),
							"langs":       voiceLangArray(vi.Languages),
							"meta_hash":   voiceHash,
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

	// ── 4. 删除数据库中多余的 system 记录 ──

	// 删除 providers
	for _, existing := range dbProviders {
		if !codeProvSlugs[existing.Slug] {
			if err := db.Where("id = ?", existing.ID).Delete(&Provider{}).Error; err != nil {
				return fmt.Errorf("sync: delete stale provider %s: %w", existing.Slug, err)
			}
			logging.Infof("store: sync removed stale provider %s", existing.Slug)
		}
	}

	// 删除 models
	for _, existing := range dbModels {
		key := existing.ProviderID + "|" + existing.ModelID
		if !codeModelKeys[key] {
			// 级联删除关联的 voices
			if err := db.Where("model_id = ?", existing.ID).Delete(&ModelVoice{}).Error; err != nil {
				return fmt.Errorf("sync: delete stale model voices %s: %w", existing.ModelID, err)
			}
			if err := db.Where("id = ?", existing.ID).Delete(&AIModel{}).Error; err != nil {
				return fmt.Errorf("sync: delete stale model %s: %w", existing.ModelID, err)
			}
			logging.Infof("store: sync removed stale model %s", existing.ModelID)
		}
	}

	// 删除 voices
	for _, existing := range dbVoices {
		key := existing.ModelID + "|" + existing.VoiceID
		if !codeVoiceKeys[key] {
			if err := db.Where("id = ?", existing.ID).Delete(&ModelVoice{}).Error; err != nil {
				return fmt.Errorf("sync: delete stale voice %s: %w", existing.VoiceID, err)
			}
			logging.Infof("store: sync removed stale voice %s", existing.VoiceID)
		}
	}

	logging.Infof("store: sync done — providers (+%d ~%d) models (+%d ~%d) voices (+%d ~%d)",
		addedProvs, updatedProvs, addedModels, updatedModels, addedVoices, updatedVoices)
	return nil
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
