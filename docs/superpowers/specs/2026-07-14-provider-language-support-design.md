# Provider Language Support — ASR/TTS/LLM 模型语言能力声明

## 背景

ASR 和 TTS 模型有语言支持限制（如 `fun-asr-realtime` 只支持 zh/en/ru），但目前系统缺乏结构化方式声明这些能力。Manager 后台的 `ModelVoice.Langs` 已实现音色按语言过滤，但 `AIModel` 缺少 `Langs` 字段，且 orion-x CLI 侧没有 Provider 语言能力声明和运行时语言感知能力。

现有 `Language` DB 表 + `LanguageStore` 本质上是硬编码的语言字典，不适合放数据库。改为 `internal/language/` Go 包作为**唯一语言数据源**。

## 目标

1. **统一语言包** — `internal/language/` 是系统语言信息的唯一入口，orion-x CLI 和 Manager 共用
2. **Manager：AIModel 按语言过滤** — 智能体配置页选择语言后，ASR 模型列表只展示支持该语言的
3. **orion-x CLI：Provider 自声明** — ASR/TTS 模型注册时声明支持的语言，启动时校验
4. **orion-x CLI：Agent 自适应** — ASR 检测到用户语言后，动态调整 system prompt

## 设计决策

| 决策 | 选项 |
| --- | --- |
| 语言数据源 | **硬编码**（`internal/language/` Go 包），不存数据库 |
| 声明粒度 | **模型级别**（provider+model） |
| LLM Language 字段 | **不加**（LLM 天生多语言，运行时自适应） |

---

## Part A：`internal/language/` 统一语言包

系统语言的**唯一数据源**，orion-x CLI 和 Manager 都从此获取。不依赖数据库。

```go
package language

// Code 是系统内部语言编码，使用枚举常量保证编译期安全。
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

// Info 是系统内部语言描述。
type Info struct {
    Code Code   // 如 ZH, EN
    Name string // 前端展示名称，如 "中文", "English"
}

// All 返回系统支持的全部语言（内置硬编码注册表）。
func All() []Info

// Get 按 Code 获取 Info，不存在返回 nil。
func Get(code Code) *Info

// Exists 检查 Code 是否在注册表中。
func Exists(code Code) bool

// Normalize 将任意格式字符串归一化为系统内部 Code。
//  "zh-CN" → ZH, "zh" → ZH, "Chinese" → ZH, "" → ""
func Normalize(raw string) Code

// Match 前缀匹配：want 是否匹配 supported 中的任一项。
//  Match(ZH,   []Code{ZH, EN, RU})  → true
//  Match("zh-CN", []Code{ZH, EN})    → true
//  Match(JA,   []Code{ZH, EN})       → false
func Match(want string, supported []Code) bool

// Codes 将字符串切片转为 Code 切片，过滤非法值。
func Codes(raw []string) []Code
```

**内置注册表（`All()` 返回值）：**

| Code | Name |
| --- | --- |
| `language.ZH` | 中文 |
| `language.EN` | English |
| `language.JA` | 日本語 |
| `language.KO` | 한국어 |
| `language.RU` | Русский |
| `language.FR` | Français |
| `language.DE` | Deutsch |
| `language.ES` | Español |
| `language.PT` | Português |
| `language.AR` | العربية |
| `language.IT` | Italiano |
| `language.NL` | Nederlands |
| `language.PL` | Polski |
| `language.TR` | Türkçe |
| `language.VI` | Tiếng Việt |
| `language.TH` | ไทย |
| `language.ID` | Bahasa Indonesia |

后续按需在 `All()` 中添加语言。方言变体（zh-CN、en-US 等）通过 `Normalize()` 映射到主语言。

---

## Part B：Manager 侧 — 切换语言 API 数据源 + AIModel 按语言过滤

### B.1 重构：语言 API 改用 `internal/language/`

**现状：**

- `GET /api/languages` 读 `LanguageStore`（`Language` DB 表）
- `ModelVoiceStore.validateLangs()` 校验通过 `LanguageStore.Exists()`
- `LanguageStore` 的种子数据是硬编码插入数据库的

**改为：**

- `GET /api/languages` 直接返回 `language.All()`
- `ModelVoiceStore.validateLangs()` 改用 `language.Exists(lang)`
- 删除 `LanguageStore`、`Language` 模型、种子数据、`/internal/languages` admin API
- Manager 不再需要 DB 存储语言

### B.2 新增：AIModel 按语言过滤

**`internal/store/models.go` — AIModel 加 Langs 字段**

```go
type AIModel struct {
    // ... 现有字段不变
    Langs pq.StringArray `gorm:"type:text[]" json:"langs,omitempty"` // 新增
}
```

**`internal/store/aimodel.go` — List 增加 lang 参数**

```go
func (s *AIModelStore) List(userID string, modelType ModelType, lang string) ([]AIModel, error)
```

`lang` 非空时加 `WHERE langs @> pq.StringArray{lang}`。

**`cmd/manager/handler/available.go` — ASR 模型按 lang 过滤**

`AvailableHandler.List` 中 `resp.ASR` 构建时，从 `c.Query("lang")` 取语言，传给 `List(userID, ModelTypeSpeech, lang)`。

### B.3 级联选择流

```
1. 前端调 GET /api/languages → language.All() → 渲染语言下拉
   用户选择 "zh"
2. 前端调 GET /api/available?lang=zh
   → resp.asr    = 只有 langs 包含 "zh" 的 speech 模型
   → resp.voices = 只有 langs 包含 "zh" 的音色（已实现）
3. 保存到 AgentConfig.Language = "zh"
```

---

## Part C：orion-x CLI 侧 — Provider 语言声明 + 校验 + Agent 自适应

### C.1 Provider 工厂层

#### `internal/provider/asr/factory.go`

```go
type ModelInfo struct {
    SupportedLanguages []language.Code
}

type ProviderMeta struct {
    Name           string
    DefaultBaseURL string
    Models         map[string]ModelInfo
}

func SupportsLanguage(providerType, model string, lang language.Code) bool
```

TTS / LLM 同理。

#### Provider 注册示例

```go
// asr/aliyun/dashscope.go — import "github.com/liuscraft/orion-x/internal/language"
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

// tts/aliyun/dashscope.go
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

LLM 的 `SupportedLanguages` 留空（全语言）。

### C.2 配置层

`ASRConfig` / `TTSConfig` 新增 `Language string` 字段（默认 `"zh"`）。

`Validate()` 调用 `language.Normalize()` + `asr.SupportsLanguage()` / `tts.SupportsLanguage()` 做前缀匹配校验。空值跳过（向后兼容）。

### C.3 Pipeline — 语言透传

`pipeline.Message.Metadata` 新增 `Language string`。`ASRStage` 解析 DashScope 返回事件中的 `language` 字段并透传。

### C.4 Agent — 动态语言指令

```go
func (a *Agent) SetLanguage(lang string)
```

`AgentStage` 收到 `Metadata.Language` 后调用，把 SoulPrompt 中的语言指令替换。仅语言变更时重建。

---

## 数据流

```
Manager 后台                      orion-x CLI
────────────────                 ──── 启动时 ────
language.All() → 语言下拉         config.Language ─→ Validate() → SupportsLanguage()
      ↓
GET /available?lang=zh            ──── 运行时 ────
→ ASR 模型（Langs 含 zh）         用户说话
→ TTS 音色（Langs 含 zh）              ↓
      ↓                          ASRProcessor → DashScope（LanguageHints）
保存 AgentConfig.Language              ↓
      ↓                          ASR 返回 language 字段
下发到设备                             ↓
                                 Message.Metadata.Language
                                      ↓
                                 Agent.SetLanguage()
                                      ↓
                                 LLM 用对应语言回复
```

---

## 语言编码约定

| 层 | 编码 |
| --- | --- |
| `language.All()` / `language.Exists()` | Code = ISO 639-1：zh, en, ja... |
| `AIModel.Langs` / `ModelVoice.Langs` | 与 `language.Exists()` 校验 |
| `ProviderMeta.SupportedLanguages` | 系统内部编码：zh, en, ru |
| `config.Language` | 系统内部编码：zh |
| Provider 对接厂商 | 内部自行适配 |
| `Pipeline Metadata.Language` | 系统内部编码 |

`language.Normalize()` 负责归一化，`Match` 前缀匹配。

### 厂商适配方式

`language.Code` 底层是 `string`（值为 `"zh"`、`"en"` 等）。直接接受 ISO 639-1 的厂商直接用 `string(code)`。需要特殊格式的厂商，在 provider 内部维护私有映射表：

```go
// provider/asr/aliyun/dashscope.go 内部
var dashScopeLangMap = map[language.Code]string{
    language.ZH: "zh-CN",
    language.EN: "en-US",
    // 其余语言直接用 string(code)
}
```

这个映射是厂商自己的事，不属于 `internal/language/` 包的职责。

## 向后兼容

- `ProviderMeta.Models` 为 nil/空 → 跳过校验
- `SupportedLanguages` 为 nil/空 → 全语言通过
- `config.Language` 为空 → 跳过校验
- `AIModel.Langs` 为空 → 全语言通过
- `Metadata.Language` 为空 → Agent 保持默认

---

## 文件变更清单

### 新增

| 文件 | 说明 |
| --- | --- |
| `internal/language/language.go` | 语言标准化 + 内置注册表 |
| `internal/language/language_test.go` | 单元测试 |

### 修改 — Manager 侧

| 文件 | 说明 |
| --- | --- |
| `cmd/manager/handler/language.go` | 改用 `language.All()` 替代 `LanguageStore.List` |
| `cmd/manager/server.go` | 删除 `/internal/languages` admin 路由 |
| `internal/store/models.go` | `AIModel.Langs` 字段；删除 `Language` 模型 |
| `internal/store/language.go` | 删除（`LanguageStore`） |
| `internal/store/aimodel.go` | `List()` 增加 lang 参数 |
| `internal/store/voice.go` | `validateLangs` 改用 `language.Exists()` |
| `internal/store/db.go` | 删除 Language 表注册 + 种子数据 |
| `cmd/manager/handler/available.go` | ASR 列表按语言过滤 |

### 修改 — orion-x CLI 侧

| 文件 | 说明 |
| --- | --- |
| `internal/provider/asr/factory.go` | ModelInfo + ProviderMeta.Models + SupportsLanguage |
| `internal/provider/tts/factory.go` | 同上 |
| `internal/llm/provider/provider.go` | 同上 |
| `internal/config/config.go` | ASRConfig/TTSConfig.Language + Validate |
| `internal/provider/asr/aliyun/dashscope.go` | init 声明模型语言 |
| `internal/provider/tts/aliyun/dashscope.go` | init 声明模型语言 |
| `internal/pipeline/message.go` | Metadata.Language |
| `internal/pipeline/stages/asr.go` | 透传语言到 Message |
| `internal/agent/agent.go` | SetLanguage() 动态 prompt |
