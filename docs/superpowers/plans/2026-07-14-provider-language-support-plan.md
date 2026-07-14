# Provider Language Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `language.Code` enum package as the single source of truth, wire it into Manager APIs (replacing the DB Language table), extend AIModel with language filtering, and add Provider self-declaration + config validation + Agent runtime language adaptation on the orion-x CLI side.

**Architecture:** `internal/language/` is a new zero-dependency Go package with enum constants, a built-in registry, and utility functions (Normalize/Match/Codes). Manager's `/api/languages` and `/api/available` switch to this package; AIModel gains a `Langs` field; the old `Language` DB model + `LanguageStore` + seed data + admin CRUD routes are removed. On the CLI side, ASR/TTS/LLM `ProviderMeta` gains `Models map[string]ModelInfo` (with `SupportedLanguages`), config gains `Language` fields with startup validation, pipeline metadata gains a `Language` field, and the Agent gets `SetLanguage()` to dynamically adjust the system prompt.

**Tech Stack:** Go 1.22+, GORM (PostgreSQL), gin-gonic

## Global Constraints

- `language.Code` is a string enum; all references use the constants (e.g. `language.ZH`), never raw strings
- `internal/language/` MUST NOT import any project-internal packages (zero deps beyond stdlib)
- Manager's `AIModel.Langs` and `ModelVoice.Langs` store codes as `pq.StringArray` in PostgreSQL; validate with `language.Exists()`
- Backward compatibility: nil/empty `ProviderMeta.Models`, nil/empty `SupportedLanguages`, empty `config.Language`, empty `AIModel.Langs`, and empty `Metadata.Language` all mean "skip/no restriction"
- Follow existing code patterns: factory registration pattern for providers, `config.Validate()` pattern for validation, TDD with inline mock structs in `*_test.go`
- Provider-to-vendor language mapping is each provider's own responsibility (private map inside the provider package)

---

### Task 1: `internal/language/` — unified language package

**Files:**

- Create: `internal/language/language.go`
- Create: `internal/language/language_test.go`

**Interfaces:**

- Produces: `language.Code` type, constants (`ZH`, `EN`, ...), `Info` struct, `All()`, `Get()`, `Exists()`, `Normalize()`, `Match()`, `Codes()`

- [ ] **Step 1: Write the test file**

`internal/language/language_test.go`:

```go
package language

import (
 "testing"
)

func TestAllNotEmpty(t *testing.T) {
 all := All()
 if len(all) == 0 {
  t.Fatal("All() returned empty")
 }
 for _, info := range all {
  if info.Code == "" || info.Name == "" {
   t.Fatalf("All() entry has empty code or name: %+v", info)
  }
 }
}

func TestGetHitAndMiss(t *testing.T) {
 if got := Get(ZH); got == nil || got.Code != ZH {
  t.Fatalf("Get(ZH) = %v, want non-nil with code ZH", got)
 }
 if got := Get(Code("xx")); got != nil {
  t.Fatalf("Get(xx) = %v, want nil", got)
 }
}

func TestExists(t *testing.T) {
 if !Exists(ZH) {
  t.Fatal("Exists(ZH) = false, want true")
 }
 if Exists(Code("xx")) {
  t.Fatal("Exists(xx) = true, want false")
 }
}

func TestNormalize(t *testing.T) {
 tests := []struct{ raw, want Code }{
  {"zh", ZH},
  {"zh-CN", ZH},
  {"zh-TW", ZH},
  {"ZH", ZH},
  {"Chinese", ZH},
  {"中文", ZH},
  {"en", EN},
  {"en-US", EN},
  {"English", EN},
  {"ja", JA},
  {"Japanese", JA},
  {"ko", KO},
  {"Korean", KO},
  {"ru", RU},
  {"Russian", RU},
  {"", ""},
  {"xx", Code("xx")}, // unknown passes through
 }
 for _, tc := range tests {
  got := Normalize(tc.raw)
  if got != tc.want {
   t.Errorf("Normalize(%q) = %q, want %q", tc.raw, got, tc.want)
  }
 }
}

func TestMatch(t *testing.T) {
 tests := []struct {
  want      string
  supported []Code
  expected  bool
 }{
  {"zh", []Code{ZH, EN, RU}, true},
  {"zh-CN", []Code{ZH, EN}, true},
  {"en-US", []Code{ZH, EN}, true},
  {"ja", []Code{ZH, EN}, false},
  {"zh", nil, false},
  {"zh", []Code{}, false},
 }
 for _, tc := range tests {
  got := Match(tc.want, tc.supported)
  if got != tc.expected {
   t.Errorf("Match(%q, %v) = %v, want %v", tc.want, tc.supported, got, tc.expected)
  }
 }
}

func TestCodes(t *testing.T) {
 got := Codes([]string{"zh", "en", "xx", "ja"})
 if len(got) != 3 {
  t.Fatalf("Codes() len=%d, want 3 (xx filtered)", len(got))
 }
 if got[0] != ZH || got[1] != EN || got[2] != JA {
  t.Fatalf("Codes() = %v, want [ZH EN JA]", got)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/language/ -v`
Expected: compilation errors (package doesn't exist)

- [ ] **Step 3: Write the implementation**

`internal/language/language.go`:

```go
// Package language provides a unified, hardcoded language registry.
// It is the single source of truth for language codes and names system-wide.
package language

import "strings"

// Code is a ISO 639-1 language code constant.
type Code string

const (
 ZH Code = "zh"
 EN Code = "en"
 JA Code = "ja"
 KO Code = "ko"
 RU Code = "ru"
 FR Code = "fr"
 DE Code = "de"
 ES Code = "es"
 PT Code = "pt"
 AR Code = "ar"
 IT Code = "it"
 NL Code = "nl"
 PL Code = "pl"
 TR Code = "tr"
 VI Code = "vi"
 TH Code = "th"
 ID Code = "id"
)

// Info pairs a language code with its display name.
type Info struct {
 Code Code
 Name string
}

// registry maps every known Code to its display name.
var registry = map[Code]string{
 ZH: "中文",
 EN: "English",
 JA: "日本語",
 KO: "한국어",
 RU: "Русский",
 FR: "Français",
 DE: "Deutsch",
 ES: "Español",
 PT: "Português",
 AR: "العربية",
 IT: "Italiano",
 NL: "Nederlands",
 PL: "Polski",
 TR: "Türkçe",
 VI: "Tiếng Việt",
 TH: "ไทย",
 ID: "Bahasa Indonesia",
}

// All returns every known language, in no particular order.
func All() []Info {
 out := make([]Info, 0, len(registry))
 for code, name := range registry {
  out = append(out, Info{Code: code, Name: name})
 }
 return out
}

// Get returns the Info for a Code, or nil if unknown.
func Get(code Code) *Info {
 name, ok := registry[code]
 if !ok {
  return nil
 }
 return &Info{Code: code, Name: name}
}

// Exists reports whether code is a known language.
func Exists(code Code) bool {
 _, ok := registry[code]
 return ok
}

// normalizer maps common raw strings to canonical Code values.
var normalizer = map[string]Code{
 "zh":       ZH,
 "zh-cn":    ZH,
 "zh-tw":    ZH,
 "zh-hk":    ZH,
 "chinese":  ZH,
 "中文":       ZH,
 "en":       EN,
 "en-us":    EN,
 "en-gb":    EN,
 "english":  EN,
 "ja":       JA,
 "japanese": JA,
 "日本語":      JA,
 "ko":       KO,
 "korean":   KO,
 "한국어":      KO,
 "ru":       RU,
 "russian":  RU,
 "Русский":    RU,
 "fr":       FR,
 "french":   FR,
 "français":   FR,
 "de":       DE,
 "german":   DE,
 "deutsch":    DE,
 "es":       ES,
 "spanish":  ES,
 "español":    ES,
 "pt":       PT,
 "portuguese": PT,
 "português":  PT,
 "ar":       AR,
 "arabic":   AR,
 "العربية":    AR,
 "it":       IT,
 "italian":  IT,
 "italiano":   IT,
 "nl":       NL,
 "dutch":    NL,
 "nederlands": NL,
 "pl":       PL,
 "polish":   PL,
 "polski":     PL,
 "tr":       TR,
 "turkish":  TR,
 "türkçe":     TR,
 "vi":       VI,
 "vietnamese": VI,
 "tiếng việt": VI,
 "th":       TH,
 "thai":     TH,
 "ไทย":        TH,
 "id":       ID,
 "indonesian": ID,
 "bahasa indonesia": ID,
}

// Normalize maps a raw language string to a canonical Code.
// Unknown inputs are passed through as-is so callers can still attempt to use them.
func Normalize(raw string) Code {
 raw = strings.TrimSpace(raw)
 if raw == "" {
  return ""
 }
 lower := strings.ToLower(raw)
 if c, ok := normalizer[lower]; ok {
  return c
 }
 return Code(strings.ToLower(raw))
}

// Match reports whether want is matched by any entry in supported.
// Matching uses prefix logic: "zh-CN" matches "zh".
func Match(want string, supported []Code) bool {
 if len(supported) == 0 {
  return false
 }
 want = strings.TrimSpace(want)
 if want == "" {
  return false
 }
 wantLower := strings.ToLower(want)
 for _, s := range supported {
  sc := strings.ToLower(string(s))
  if strings.HasPrefix(wantLower, sc) || strings.HasPrefix(sc, wantLower) {
   return true
  }
 }
 return false
}

// Codes converts raw strings to Code values, filtering out unknown codes.
func Codes(raw []string) []Code {
 out := make([]Code, 0, len(raw))
 for _, r := range raw {
  c := Normalize(r)
  if c != "" && Exists(c) {
   out = append(out, c)
  }
 }
 return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/language/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/language/language.go internal/language/language_test.go
git commit -m "feat(language): add unified language Code enum with built-in registry"
```

---

### Task 2: Manager — remove `LanguageStore`, switch to `language` package

**Files:**

- Modify: `internal/store/models.go` — remove `Language` model, add `AIModel.Langs`
- Modify: `internal/store/aimodel.go` — add `lang string` parameter to `List()`
- Delete: `internal/store/language.go`
- Modify: `internal/store/voice.go` — remove `languageStore`, use `language.Exists()`
- Modify: `internal/store/db.go` — remove Language from AutoMigrate and seed
- Modify: `cmd/manager/handler/language.go` — use `language.All()` / `language.Get()`
- Modify: `cmd/manager/server.go` — remove `/internal/languages` routes
- Modify: `cmd/manager/main.go` — remove LanguageStore wiring

**Interfaces:**

- Consumes: `language.All()`, `language.Get()`, `language.Exists()`
- Produces: Updated `AIModelStore.List()`, `ModelVoiceStore` without LanguageStore dep, `LanguageHandler` without LanguageStore

- [ ] **Step 1: Update `internal/store/models.go`**

Remove the `Language` struct entirely. Add `Langs` to `AIModel`:

```go
type AIModel struct {
 ID         string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
 ProviderID string            `gorm:"not null;index;type:varchar(36)" json:"provider_id"`
 Provider   *Provider         `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
 Name       string            `gorm:"not null;type:varchar(128)" json:"name"`
 Type       ModelType         `gorm:"not null;type:varchar(16);index" json:"type"`
 BaseURL    string            `gorm:"type:varchar(512)" json:"base_url"`
 ModelID    string            `gorm:"not null;type:varchar(128)" json:"model_id"`
 IsSystem   bool              `gorm:"not null;default:false;index" json:"is_system"`
 Langs      pq.StringArray    `gorm:"type:text[]" json:"langs,omitempty"`
 Extra      datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
 Voices     []ModelVoice      `gorm:"foreignKey:ModelID" json:"voices,omitempty"`
 BaseModel
}
```

Delete the `Language` struct (lines ~93-101 in original).

- [ ] **Step 2: Update `internal/store/aimodel.go`**

Change `List` signature to include `lang`:

```go
func (s *AIModelStore) List(userID string, modelType ModelType, lang string) ([]AIModel, error) {
 q := s.db.Preload("Provider").Where("is_system = true OR creator = ?", userID)
 if modelType != "" {
  q = q.Where("type = ?", modelType)
 }
 if lang != "" {
  q = q.Where("langs @> ?", pq.StringArray{lang})
 }
 var list []AIModel
 if err := q.Find(&list).Error; err != nil {
  return nil, fmt.Errorf("ai_model store: list: %w", err)
 }
 return list, nil
}
```

- [ ] **Step 3: Check and fix callers of models.List**

Run: `grep -rn "\.models\.List\|\.List(" cmd/manager/ internal/store/ --include="*.go" | grep -v _test.go`
Find every call that passes `userID, modelType` and update to include `""` (empty lang = no filter) for non-ASR calls.

`cmd/manager/handler/model.go:32`:

```go
list, err := h.models.List(middleware.UserID(c), modelType)
```

Change to:

```go
list, err := h.models.List(middleware.UserID(c), modelType, "")
```

Also check if `List` is called from anywhere else (e.g., in `aimodel_test.go`).

- [ ] **Step 4: Delete `internal/store/language.go`**

```bash
rm internal/store/language.go
```

- [ ] **Step 5: Update `internal/store/voice.go`**

Remove `languageStore` field and dependency. Change `validateLangs`:

```go
type ModelVoiceStore struct {
 db *gorm.DB
}

func NewModelVoiceStore(db *gorm.DB) *ModelVoiceStore {
 return &ModelVoiceStore{db: db}
}
```

Update `validateLangs`:

```go
import "github.com/liuscraft/orion-x/internal/language"

func (s *ModelVoiceStore) validateLangs(langs pq.StringArray) error {
 for _, langStr := range langs {
  code := language.Normalize(langStr)
  if code == "" || !language.Exists(code) {
   return fmt.Errorf("voice store: unknown language: %s", langStr)
  }
 }
 return nil
}
```

- [ ] **Step 6: Update `internal/store/db.go`**

Remove `&Language{}` from AutoMigrate. Remove `seedLanguages` call, `seedLanguageEntries` var, and `strPtr` helper:

```go
if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}); err != nil {
```

Remove the `seedLanguages(db)` block (lines ~25-27) and all the seed data (lines ~36-104). Remove `strPtr` if no longer used.

- [ ] **Step 7: Update `cmd/manager/handler/language.go`**

Rewrite to use `language` package:

```go
package handler

import (
 "net/http"
 "github.com/gin-gonic/gin"
 "github.com/liuscraft/orion-x/internal/language"
)

type LanguageHandler struct{}

func NewLanguageHandler() *LanguageHandler {
 return &LanguageHandler{}
}

// GET /api/languages
func (h *LanguageHandler) List(c *gin.Context) {
 c.JSON(http.StatusOK, language.All())
}

// GET /api/languages/:code
func (h *LanguageHandler) Get(c *gin.Context) {
 info := language.Get(language.Code(c.Param("code")))
 if info == nil {
  c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
  return
 }
 c.JSON(http.StatusOK, info)
}
```

Remove `AdminCreate`, `AdminUpdate`, `AdminDelete`, `createLanguageRequest`.

- [ ] **Step 8: Update `cmd/manager/server.go`**

Remove the `langH` field from router/middleware struct. Remove routes:

- Line ~139-140: Remove `/api/languages` and `/api/languages/:code` from jwt group (keep the List/Get if they're still needed, just update handler)
- Lines ~150-153: Remove all `/internal/languages` routes

The api routes stay but with updated handler. Remove the internal admin routes.

Update the router function to not require `langH`:

```go
// Remove langH *handler.LanguageHandler from newRouter params
```

- [ ] **Step 9: Update `cmd/manager/main.go`**

Remove LanguageStore creation:

```go
// DELETE: languages := store.NewLanguageStore(db)
```

Update `ModelVoiceStore` creation:

```go
voices := store.NewModelVoiceStore(db)  // was: store.NewModelVoiceStore(db, languages)
```

Update `LanguageHandler` creation:

```go
langH := handler.NewLanguageHandler()  // was: handler.NewLanguageHandler(languages)
```

Remove language parameter from `newRouter` call.

- [ ] **Step 10: Build and verify**

Run: `go build ./cmd/manager/`
Expected: compilation succeeds

Run: `go vet ./internal/store/ ./cmd/manager/...`
Expected: no errors

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "refactor(manager): replace LanguageStore with language package, add AIModel.Langs"
```

---

### Task 3: Manager — ASR model filtering by language in AvailableHandler

**Files:**

- Modify: `cmd/manager/handler/available.go`

**Interfaces:**

- Consumes: `h.models.List(userID, ModelTypeSpeech, lang)` (updated signature from Task 2)

- [ ] **Step 1: Update `AvailableHandler.List`**

In `cmd/manager/handler/available.go`, the `allModels, err := h.models.List(userID, "")` line is already obtaining all models. Change to filter by lang for ASR:

```go
func (h *AvailableHandler) List(c *gin.Context) {
 userID := middleware.UserID(c)
 lang := c.Query("lang")

 // ... slug registries (unchanged) ...
 // ... providers (unchanged) ...
 // ... providerByID (unchanged) ...

 allModels, err := h.models.List(userID, "", "") // all models, no type filter, no lang filter
 if err != nil {
  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
  return
 }

 var resp AvailableResourcesResponse

 // ── ASR models ──
 seenASR := map[string]bool{}
 for _, m := range allModels {
  if m.Type != ModelTypeSpeech {
   continue
  }
  // Filter by language if specified
  if lang != "" && len(m.Langs) > 0 {
   hasLang := false
   for _, l := range m.Langs {
    if l == lang {
     hasLang = true
     break
    }
   }
   if !hasLang {
    continue
   }
  }
  p, ok := providerByID[m.ProviderID]
  if !ok {
   continue
  }
  cat, key := extractCategory(p.Slug)
  cats := []string{}
  if cat != "" {
   cats = append(cats, cat)
  } else if c := slugCats[key]; len(c) > 0 {
   cats = c
  }
  for _, c := range cats {
   if c == "asr" && !seenASR[m.ID] {
    seenASR[m.ID] = true
    resp.ASR = append(resp.ASR, ResourceOption{ID: m.ID, Name: m.Name})
   }
  }
 }
 // ... rest unchanged (voices) ...
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/manager/`
Expected: compilation succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/manager/handler/available.go
git commit -m "feat(manager): filter ASR models by language in available endpoint"
```

---

### Task 4: orion-x — extend ProviderMeta with model language info

**Files:**

- Modify: `internal/provider/asr/factory.go`
- Modify: `internal/provider/tts/factory.go`
- Modify: `internal/llm/provider/provider.go`
- Modify: `internal/provider/asr/aliyun/dashscope.go`
- Modify: `internal/provider/tts/aliyun/dashscope.go`

**Interfaces:**

- Consumes: `language.Code`
- Produces: `ModelInfo` struct, updated `ProviderMeta`, `SupportsLanguage()`, model language declarations

- [ ] **Step 1: Update `internal/provider/asr/factory.go`**

Add import and new types:

```go
import "github.com/liuscraft/orion-x/internal/language"

type ModelInfo struct {
 SupportedLanguages []language.Code
}

type ProviderMeta struct {
 Name           string
 DefaultBaseURL string
 Models         map[string]ModelInfo
}

type registration struct {
 constructor Constructor
 meta        ProviderMeta
}
```

Add `SupportsLanguage` function:

```go
func SupportsLanguage(providerType, model string, lang language.Code) bool {
 reg, ok := constructors[normalizeType(providerType, "")]
 if !ok {
  return false
 }
 info, ok := reg.meta.Models[model]
 if !ok {
  return true // model not declared → no restriction
 }
 if len(info.SupportedLanguages) == 0 {
  return true // empty list → all languages
 }
 for _, s := range info.SupportedLanguages {
  if s == lang {
   return true
  }
 }
 return false
}
```

- [ ] **Step 2: Update `internal/provider/tts/factory.go`**

Same changes as ASR (ModelInfo, ProviderMeta, SupportsLanguage, import language).

- [ ] **Step 3: Update `internal/llm/provider/provider.go`**

Add import and types to `ProviderMeta` + `registration`:

```go
import "github.com/liuscraft/orion-x/internal/language"

type ModelInfo struct {
 SupportedLanguages []language.Code
}

type ProviderMeta struct {
 Name           string
 DefaultBaseURL string
 Models         map[string]ModelInfo
}

type registration struct {
 constructor AdapterConstructor
 meta        ProviderMeta
}
```

Add `SupportsLanguage` to `Registry`:

```go
func (r *Registry) SupportsLanguage(providerType, model string, lang language.Code) bool {
 reg, ok := r.adapters[strings.ToLower(strings.TrimSpace(providerType))]
 if !ok {
  return false
 }
 info, ok := reg.meta.Models[model]
 if !ok {
  return true
 }
 if len(info.SupportedLanguages) == 0 {
  return true
 }
 for _, s := range info.SupportedLanguages {
  if s == lang {
   return true
  }
 }
 return false
}
```

- [ ] **Step 4: Update `internal/provider/asr/aliyun/dashscope.go`**

Add language import and update `init()`:

```go
import "github.com/liuscraft/orion-x/internal/language"

func init() {
 asr.Register(asr.TypeAliyun, func(cfg asr.Config) (asr.Recognizer, error) {
  return NewDashScopeRecognizer(cfg)
 }, asr.ProviderMeta{
  Name:           "阿里云 Dashscope",
  DefaultBaseURL: defaultDashScopeEndpoint,
  Models: map[string]asr.ModelInfo{
   "fun-asr-realtime": {SupportedLanguages: []language.Code{language.ZH, language.EN, language.RU}},
   "fun-asr":          {SupportedLanguages: []language.Code{language.ZH, language.EN, language.JA, language.KO}},
  },
 })
}
```

- [ ] **Step 5: Update `internal/provider/tts/aliyun/dashscope.go`**

Same pattern:

```go
import "github.com/liuscraft/orion-x/internal/language"

func init() {
 tts.Register(tts.TypeAliyun, func(cfg tts.Config) (tts.Provider, error) {
  return NewDashScopeProvider(cfg)
 }, tts.ProviderMeta{
  Name:           "阿里云 Dashscope",
  DefaultBaseURL: defaultDashScopeEndpoint,
  Models: map[string]tts.ModelInfo{
   "cosyvoice-v3-flash": {SupportedLanguages: []language.Code{language.ZH, language.EN}},
  },
 })
}
```

- [ ] **Step 6: Build and verify**

Run: `go build ./internal/provider/... ./internal/llm/...`
Expected: compilation succeeds

Run: `go test ./internal/provider/asr/... ./internal/provider/tts/...`
Expected: existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add internal/provider/asr/factory.go internal/provider/tts/factory.go internal/llm/provider/provider.go internal/provider/asr/aliyun/dashscope.go internal/provider/tts/aliyun/dashscope.go
git commit -m "feat(provider): add ModelInfo with SupportedLanguages to ProviderMeta"
```

---

### Task 5: orion-x — config Language fields + validation

**Files:**

- Modify: `internal/config/config.go`

**Interfaces:**

- Consumes: `language.Normalize()`, `asr.SupportsLanguage()`, `tts.SupportsLanguage()`

- [ ] **Step 1: Add Language fields to config structs**

In `internal/config/config.go`:

```go
type ASRConfig struct {
 APIKey   string `json:"api_key"`
 Model    string `json:"model"`
 Endpoint string `json:"endpoint"`
 Language string `json:"language"` // system language code, e.g. "zh"
}

type TTSConfig struct {
 APIKey               string            `json:"api_key"`
 Endpoint             string            `json:"endpoint"`
 Workspace            string            `json:"workspace"`
 Model                string            `json:"model"`
 Voice                string            `json:"voice"`
 Format               string            `json:"format"`
 SampleRate           int               `json:"sample_rate"`
 Volume               int               `json:"volume"`
 Rate                 float64           `json:"rate"`
 Pitch                float64           `json:"pitch"`
 EnableSSML           bool              `json:"enable_ssml"`
 TextType             string            `json:"text_type"`
 EnableDataInspection *bool             `json:"enable_data_inspection"`
 VoiceMap             map[string]string `json:"voice_map"`
 Language             string            `json:"language"` // system language code
}
```

- [ ] **Step 2: Add default Language values**

In `DefaultConfig()`:

```go
defaultASR := ASRConfig{
 Model:    "fun-asr-realtime",
 Language: "zh",
}
defaultTTS := TTSConfig{
 Model:                "cosyvoice-v3-flash",
 Voice:                "longanyang",
 Format:               "pcm",
 SampleRate:           16000,
 Volume:               50,
 Rate:                 1.0,
 Pitch:                1.0,
 TextType:             "PlainText",
 EnableDataInspection: &enableDataInspection,
 Language:             "zh",
 VoiceMap: map[string]string{
  "happy":   "longanyang",
  "sad":     "zhichu",
  "angry":   "zhimeng",
  "calm":    "longxiaochun",
  "excited": "longanyang",
  "default": "longanyang",
 },
}
```

- [ ] **Step 3: Add validateASRLanguage and validateTTSLanguage**

Add imports:

```go
import (
 // ... existing imports
 "github.com/liuscraft/orion-x/internal/language"
 asrprovider "github.com/liuscraft/orion-x/internal/provider/asr"
 ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
)
```

Add to `Validate()`:

```go
if err := c.validateASRLanguage(); err != nil {
 return err
}
if err := c.validateTTSLanguage(); err != nil {
 return err
}
```

Add methods:

```go
func (c *AppConfig) validateASRLanguage() error {
 lang := language.Normalize(c.Provider.ASR.Aliyun.Language)
 if lang == "" {
  return nil
 }
 model := c.Provider.ASR.Aliyun.Model
 providerType := c.Provider.ASR.Type
 if !asrprovider.SupportsLanguage(providerType, model, lang) {
  return fmt.Errorf("ASR model %q does not support language %q", model, lang)
 }
 return nil
}

func (c *AppConfig) validateTTSLanguage() error {
 lang := language.Normalize(c.Provider.TTS.Aliyun.Language)
 if lang == "" {
  return nil
 }
 model := c.Provider.TTS.Aliyun.Model
 providerType := c.Provider.TTS.Type
 if !ttsprovider.SupportsLanguage(providerType, model, lang) {
  return fmt.Errorf("TTS model %q does not support language %q", model, lang)
 }
 return nil
}
```

- [ ] **Step 4: Build and run tests**

Run: `go build ./internal/config/`
Expected: compilation succeeds

Run: `go test ./internal/config/ -v`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add Language fields and validation for ASR/TTS"
```

---

### Task 6: orion-x — Pipeline language metadata + Agent SetLanguage

**Files:**

- Modify: `internal/pipeline/message.go`
- Modify: `internal/pipeline/stages/asr.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/pipeline/stages/agent.go`

**Interfaces:**

- Consumes: `language` package (for Agent prompt construction)
- Produces: `Metadata.Language`, `Agent.SetLanguage()`, ASR stage language passthrough

- [ ] **Step 1: Add Language to pipeline Metadata**

`internal/pipeline/message.go`:

```go
type Metadata struct {
 TurnID    int64
 TraceID   string
 Emotion   string
 Language  string // ASR-detected language code, e.g. "zh"
 Timestamp time.Time
 Error     error
 Extra     map[string]interface{}
}
```

- [ ] **Step 2: Pass language through in ASRStage**

`internal/pipeline/stages/asr.go` — in the ASR result handler, check if the ASR Recognizer returns a language field. Currently DashScope may not return it explicitly; we can start by passing `config.Language` as a signal, or leave it as `""` until DashScope starts returning language info.

For now, when emitting the ASR result, if the platform's config language is known, we pass it. But since ASRStage doesn't have config access, we keep it simple: just make the field exist.

In the `OnResult` callback, the current code already emits `pipeline.Message`. No change needed for the first pass — the `Language` field defaults to `""` which means "no change".

- [ ] **Step 3: Add SetLanguage to Agent**

`internal/agent/agent.go` — add a `currentLang` field and `SetLanguage` method:

```go
type Agent struct {
 client      llm.Client
 registry    *tools.Registry
 model       string
 memorySvc   *memory.Service
 maxSteps    int
 soulPrompt  string
 rulesPrompt string
 currentLang string // language code currently reflected in system prompt
}

func (a *Agent) SetLanguage(lang string) {
 if lang == "" || lang == a.currentLang {
  return
 }
 a.currentLang = lang
}
```

The actual prompt construction happens in `context_builder.go`. We add a helper to build the language-aware system prompt:

```go
// languageAwareSoulPrompt replaces the hardcoded "用中文交流" with the
// appropriate language instruction based on the detected language.
func languageAwareSoulPrompt(soul, lang string) string {
 if lang == "" || lang == "zh" {
  return soul // default is already Chinese
 }
 // Look up language name for the prompt
 info := language.Get(language.Code(lang))
 langName := lang
 if info != nil {
  langName = info.Name
 }
 // Replace the Chinese instruction with the detected language
 replacer := strings.NewReplacer(
  "用中文交流", fmt.Sprintf("用%s交流", langName),
  "用中文", fmt.Sprintf("用%s", langName),
 )
 return replacer.Replace(soul)
}
```

Update `context_builder.go` `buildSystemPrompt` to call `languageAwareSoulPrompt` when `a.currentLang` is set.

- [ ] **Step 4: Wire AgentStage to call SetLanguage**

`internal/pipeline/stages/agent.go` — when processing an incoming message, check `msg.Metadata.Language`:

```go
case pipeline.MessageTypeData:
 text, ok := msg.Payload.(string)
 if !ok {
  continue
 }
 if msg.Metadata.Language != "" {
  a.agent.SetLanguage(msg.Metadata.Language)
 }
 // ... rest of handling
```

- [ ] **Step 5: Build and test**

Run: `go build ./internal/agent/... ./internal/pipeline/...`
Expected: compilation succeeds

Run: `go test ./internal/agent/... ./internal/pipeline/... -v`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/pipeline/message.go internal/pipeline/stages/asr.go internal/agent/agent.go internal/agent/context_builder.go internal/pipeline/stages/agent.go
git commit -m "feat(agent): add Metadata.Language and Agent.SetLanguage for runtime language adaptation"
```

---

### Task 7: Final integration — build, lint, and test

**Files:**

- All project files

- [ ] **Step 1: Full build**

```bash
make build
```

Expected: both CLI and Manager compile successfully

- [ ] **Step 2: Run all tests**

```bash
make test
```

Expected: all tests pass

- [ ] **Step 3: Run linter**

```bash
golangci-lint run ./...
```

Expected: no issues

- [ ] **Step 4: Fix any issues found, then commit**

```bash
git add -A
git commit -m "chore: final integration fixes for language support"
```
