# Language Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Language table, admin/public API, and voice langs validation.

**Architecture:** New `Language` model (self-referencing parent_code for dialects) + `LanguageStore` for CRUD. `ModelVoiceStore` gains a `languageStore` dependency to validate `Langs` on create/update. Two handler groups: `/internal/languages` (admin, no auth) and `/api/languages` (JWT-protected).

**Tech Stack:** Go, GORM, PostgreSQL (pgx), Gin

---

### Task 1: Language model

**Files:**
- Modify: `internal/store/models.go`

- [ ] **Add Language struct before BaseModel references:**

```go
// Language TTS 音色语言标签字典（两级：语言→方言）
type Language struct {
	Code       string      `gorm:"primaryKey;type:varchar(16)" json:"code"`
	Name       string      `gorm:"not null;type:varchar(64)" json:"name"`
	ParentCode *string     `gorm:"type:varchar(16);index" json:"parent_code,omitempty"`
	Parent     *Language   `gorm:"foreignKey:ParentCode;references:Code" json:"parent,omitempty"`
	Children   []*Language `gorm:"foreignKey:ParentCode;references:Code" json:"children,omitempty"`
	IsSystem   bool        `gorm:"not null;default:false" json:"is_system"`
	BaseModel
}
```

Place it after `ModelVoice` (line ~121), before the file end.

---

### Task 2: LanguageStore

**Files:**
- Create: `internal/store/language.go`

- [ ] **Create the file with full implementation:**

```go
package store

import (
	"fmt"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

type LanguageStore struct{ db *gorm.DB }

func NewLanguageStore(db *gorm.DB) *LanguageStore { return &LanguageStore{db: db} }

func (s *LanguageStore) List(parentCode string) ([]Language, error) {
	q := s.db.Order("code ASC")
	if parentCode == "null" {
		q = q.Where("parent_code IS NULL")
	} else if parentCode != "" {
		q = q.Where("parent_code = ?", parentCode)
	}
	var list []Language
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("language store: list: %w", err)
	}
	return list, nil
}

func (s *LanguageStore) GetByCode(code string) (*Language, error) {
	var lang Language
	if err := s.db.Preload("Children").First(&lang, "code = ?", code).Error; err != nil {
		return nil, err
	}
	return &lang, nil
}

func (s *LanguageStore) Exists(code string) (bool, error) {
	var count int64
	if err := s.db.Model(&Language{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, fmt.Errorf("language store: exists: %w", err)
	}
	return count > 0, nil
}

type CreateLanguageParams struct {
	Code       string
	Name       string
	ParentCode *string
	IsSystem   bool
}

func (s *LanguageStore) Create(p CreateLanguageParams) (*Language, error) {
	lang := &Language{
		Code:       p.Code,
		Name:       p.Name,
		ParentCode: p.ParentCode,
		IsSystem:   p.IsSystem,
	}
	if err := s.db.Create(lang).Error; err != nil {
		return nil, fmt.Errorf("language store: create: %w", err)
	}
	return lang, nil
}

func (s *LanguageStore) Update(code string, updates map[string]any) (*Language, error) {
	if err := s.db.Model(&Language{}).Where("code = ?", code).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("language store: update: %w", err)
	}
	return s.GetByCode(code)
}

func (s *LanguageStore) Delete(code string) error {
	var lang Language
	if err := s.db.First(&lang, "code = ?", code).Error; err != nil {
		return err
	}
	if lang.IsSystem {
		return ErrSystemRecord
	}
	// delete children first
	if err := s.db.Where("parent_code = ?", code).Delete(&Language{}).Error; err != nil {
		return fmt.Errorf("language store: delete children: %w", err)
	}
	if err := s.db.Delete(&Language{}, "code = ?", code).Error; err != nil {
		return fmt.Errorf("language store: delete: %w", err)
	}
	return nil
}
```

---

### Task 3: DB migration + seeds

**Files:**
- Modify: `internal/store/db.go`

- [ ] **Add Language to AutoMigrate + seed function:**

```go
// Add to imports:
//	"gorm.io/gorm/clause"

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &Language{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	logging.Infof("store: migration done (users, voicebots, devices, providers, ai_models, model_voices, languages)")

	if err := seedLanguages(db); err != nil {
		return nil, fmt.Errorf("store: seed languages: %w", err)
	}
	return db, nil
}

var seedLanguagesEntries = []Language{
	{Code: "zh", Name: "Chinese", IsSystem: true},
	{Code: "zh-CN", Name: "Chinese (Simplified)", ParentCode: strPtr("zh"), IsSystem: true},
	{Code: "zh-TW", Name: "Chinese (Traditional)", ParentCode: strPtr("zh"), IsSystem: true},
	{Code: "zh-HK", Name: "Chinese (Hong Kong)", ParentCode: strPtr("zh"), IsSystem: true},
	{Code: "en", Name: "English", IsSystem: true},
	{Code: "en-US", Name: "English (US)", ParentCode: strPtr("en"), IsSystem: true},
	{Code: "en-GB", Name: "English (UK)", ParentCode: strPtr("en"), IsSystem: true},
	{Code: "en-AU", Name: "English (Australia)", ParentCode: strPtr("en"), IsSystem: true},
	{Code: "ja", Name: "Japanese", IsSystem: true},
	{Code: "ko", Name: "Korean", IsSystem: true},
	{Code: "fr", Name: "French", IsSystem: true},
	{Code: "fr-FR", Name: "French (France)", ParentCode: strPtr("fr"), IsSystem: true},
	{Code: "fr-CA", Name: "French (Canada)", ParentCode: strPtr("fr"), IsSystem: true},
	{Code: "de", Name: "German", IsSystem: true},
	{Code: "de-DE", Name: "German (Germany)", ParentCode: strPtr("de"), IsSystem: true},
	{Code: "es", Name: "Spanish", IsSystem: true},
	{Code: "es-ES", Name: "Spanish (Spain)", ParentCode: strPtr("es"), IsSystem: true},
	{Code: "es-MX", Name: "Spanish (Mexico)", ParentCode: strPtr("es"), IsSystem: true},
	{Code: "pt", Name: "Portuguese", IsSystem: true},
	{Code: "pt-BR", Name: "Portuguese (Brazil)", ParentCode: strPtr("pt"), IsSystem: true},
	{Code: "pt-PT", Name: "Portuguese (Portugal)", ParentCode: strPtr("pt"), IsSystem: true},
	{Code: "ar", Name: "Arabic", IsSystem: true},
	{Code: "ar-SA", Name: "Arabic (Saudi Arabia)", ParentCode: strPtr("ar"), IsSystem: true},
	{Code: "ru", Name: "Russian", IsSystem: true},
	{Code: "it", Name: "Italian", IsSystem: true},
	{Code: "it-IT", Name: "Italian (Italy)", ParentCode: strPtr("it"), IsSystem: true},
	{Code: "nl", Name: "Dutch", IsSystem: true},
	{Code: "nl-NL", Name: "Dutch (Netherlands)", ParentCode: strPtr("nl"), IsSystem: true},
	{Code: "pl", Name: "Polish", IsSystem: true},
	{Code: "tr", Name: "Turkish", IsSystem: true},
	{Code: "vi", Name: "Vietnamese", IsSystem: true},
	{Code: "th", Name: "Thai", IsSystem: true},
	{Code: "th-TH", Name: "Thai (Thailand)", ParentCode: strPtr("th"), IsSystem: true},
	{Code: "id", Name: "Indonesian", IsSystem: true},
}

func strPtr(s string) *string { return &s }

func seedLanguages(db *gorm.DB) error {
	var count int64
	if err := db.Model(&Language{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, l := range seedLanguagesEntries {
		if err := db.Create(&l).Error; err != nil {
			return fmt.Errorf("seed language %s: %w", l.Code, err)
		}
	}
	logging.Infof("store: seeded %d languages", len(seedLanguagesEntries))
	return nil
}
```

---

### Task 4: Voice validation

**Files:**
- Modify: `internal/store/voice.go`

- [ ] **Add `languageStore` field to `ModelVoiceStore`, update constructor, and validate langs:**

```go
type ModelVoiceStore struct {
	db            *gorm.DB
	languageStore *LanguageStore
}

func NewModelVoiceStore(db *gorm.DB, languageStore *LanguageStore) *ModelVoiceStore {
	return &ModelVoiceStore{db: db, languageStore: languageStore}
}

// validateLangs checks all lang codes exist in the Language table.
func (s *ModelVoiceStore) validateLangs(langs pq.StringArray) error {
	for _, lang := range langs {
		ok, err := s.languageStore.Exists(lang)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("voice store: unknown language: %s", lang)
		}
	}
	return nil
}
```

- [ ] **Add validation call at top of `Create`, `CreateSystem`, `CreateCloned`:**

```go
func (s *ModelVoiceStore) Create(p CreateVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	// ... rest unchanged
```

```go
func (s *ModelVoiceStore) CreateSystem(p CreateVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	// ... rest unchanged
```

```go
func (s *ModelVoiceStore) CreateCloned(p CloneVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	// ... rest unchanged
```

- [ ] **Add validation in `Update` before the `Updates` call:**

```go
func (s *ModelVoiceStore) Update(id string, updates map[string]any) (*ModelVoice, error) {
	if langs, ok := updates["langs"]; ok {
		if arr, ok := langs.(pq.StringArray); ok {
			if err := s.validateLangs(arr); err != nil {
				return nil, err
			}
		}
	}
	// ... rest unchanged
```

---

### Task 5: Language handler

**Files:**
- Create: `cmd/manager/handler/language.go`

- [ ] **Create the file:**

```go
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type LanguageHandler struct {
	languages *store.LanguageStore
}

func NewLanguageHandler(languages *store.LanguageStore) *LanguageHandler {
	return &LanguageHandler{languages: languages}
}

// GET /api/languages [?parent_code=zh]
func (h *LanguageHandler) List(c *gin.Context) {
	list, err := h.languages.List(c.Query("parent_code"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/languages/:code
func (h *LanguageHandler) Get(c *gin.Context) {
	lang, err := h.languages.GetByCode(c.Param("code"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lang)
}

// --- Internal admin handlers (no JWT) ---

type createLanguageRequest struct {
	Code       string  `json:"code" binding:"required"`
	Name       string  `json:"name" binding:"required"`
	ParentCode *string `json:"parent_code"`
}

// POST /internal/languages
func (h *LanguageHandler) AdminCreate(c *gin.Context) {
	var req createLanguageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lang, err := h.languages.Create(store.CreateLanguageParams{
		Code:       req.Code,
		Name:       req.Name,
		ParentCode: req.ParentCode,
		IsSystem:   false,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, lang)
}

// PUT /internal/languages/:code
func (h *LanguageHandler) AdminUpdate(c *gin.Context) {
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lang, err := h.languages.Update(c.Param("code"), updates)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lang)
}

// DELETE /internal/languages/:code
func (h *LanguageHandler) AdminDelete(c *gin.Context) {
	if err := h.languages.Delete(c.Param("code")); err != nil {
		if errors.Is(err, store.ErrSystemRecord) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system language"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
```

---

### Task 6: Route registration

**Files:**
- Modify: `cmd/manager/main.go` — create LanguageStore and pass to ModelVoiceStore + LanguageHandler
- Modify: `cmd/manager/server.go` — add language routes

- [ ] **In `cmd/manager/main.go`, find the lines that create stores and add LanguageStore:**

Look for:
```go
	voices := store.NewModelVoiceStore(db)
```

Replace with:
```go
	languages := store.NewLanguageStore(db)
	voices := store.NewModelVoiceStore(db, languages)
```

Also add:
```go
	langH := handler.NewLanguageHandler(languages)
```

to the handler initializations block (around line 77), and pass it to `newRouter`.

Update `newRouter` call signature to include `langH`.

- [ ] **In `cmd/manager/server.go`, add routes:**

Add `langH` parameter to `newRouter`:
```go
func newRouter(
	jwtSecret []byte,
	users *store.UserStore,
	voicebots *store.VoicebotStore,
	devices *store.DeviceStore,
	providers *store.ProviderStore,
	models *store.AIModelStore,
	voices *store.ModelVoiceStore,
	langH *handler.LanguageHandler,
	signToken func(userID string) (string, error),
) *gin.Engine {
```

Add public routes after the sessions placeholder:
```go
		// 语言字典（只读，公开）
		api.GET("/languages", jwtMw, langH.List)
		api.GET("/languages/:code", jwtMw, langH.Get)
```

Add internal routes after the voices internal routes:
```go
	r.GET("/internal/languages", langH.List)
	r.POST("/internal/languages", langH.AdminCreate)
	r.PUT("/internal/languages/:code", langH.AdminUpdate)
	r.DELETE("/internal/languages/:code", langH.AdminDelete)
```
