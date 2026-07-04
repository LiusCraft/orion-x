# Language Management — Voice 语言标签管理

## 背景

`ModelVoice` 已有 `Langs` 字段（`pq.StringArray`）存储音色支持的语言标签，但缺少：

1. **受控的语言字典** — 没有预定义的系统支持语言列表
2. **校验** — 创建音色时可随意填写 `langs`，不受约束

## 目标

1. 新增 `Language` 表，提供两级（语言 + 方言/区域变体）字典
2. 提供内部 Admin CRUD API + 公开只读 API
3. 音色创建/更新时校验 `Langs` 值必须存在于 `Language` 表

## Language 模型

```go
type Language struct {
    Code       string         `gorm:"primaryKey;type:varchar(16)" json:"code"`
    Name       string         `gorm:"not null;type:varchar(64)" json:"name"`
    ParentCode *string        `gorm:"type:varchar(16);index" json:"parent_code,omitempty"`
    Parent     *Language      `gorm:"foreignKey:ParentCode;references:Code" json:"parent,omitempty"`
    IsSystem   bool           `gorm:"not null;default:false" json:"is_system"`
    BaseModel
}
```

- `code` = BCP-47 风格标签（`zh`、`zh-CN`、`en-US`）
- `parent_code` 自引用，顶级语言为 nil
- `is_system` 标记系统内置行

## Store 层

`internal/store/language.go`:

- `List(parentCode string) ([]Language, error)` — 可选按 parent 过滤
- `GetByCode(code string) (*Language, error)`
- `Exists(code string) (bool, error)` — 供 Voice 校验
- `Create(p CreateLanguageParams) (*Language, error)`
- `Update(code string, updates map[string]any) (*Language, error)`
- `Delete(code string) error` — 保护 system 行

## 种子数据

初始化时自动插入以下两级语言：

| 顶级 | 子级 |
|---|---|
| zh (Chinese) | zh-CN, zh-TW, zh-HK |
| en (English) | en-US, en-GB, en-AU |
| ja (Japanese) | - |
| ko (Korean) | - |
| fr (French) | fr-FR, fr-CA |
| de (German) | de-DE |
| es (Spanish) | es-ES, es-MX |
| pt (Portuguese) | pt-BR, pt-PT |
| ar (Arabic) | ar-SA |
| ru (Russian) | - |
| it (Italian) | it-IT |
| nl (Dutch) | nl-NL |
| pl (Polish) | - |
| tr (Turkish) | - |
| vi (Vietnamese) | - |
| th (Thai) | th-TH |
| id (Indonesian) | - |

## API

### 内部 Admin API（`/internal/` 前缀）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/internal/languages` | 列表，`?parent_code=zh` |
| `POST` | `/internal/languages` | 创建 language |
| `PUT` | `/internal/languages/:code` | 更新 |
| `DELETE` | `/internal/languages/:code` | 删除（保护 system 行） |

### 公开 API（`/api/` 前缀，JWT）

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/languages` | 列表，`?parent_code=zh` |
| `GET` | `/api/languages/:code` | 单条 + 子级 |

## Voice 校验

在 `ModelVoiceStore.Create` / `CreateSystem` / `CreateCloned` / `Update` 中，遍历 `Langs` 数组并调用 `LanguageStore.Exists(lang)` 校验。不存在则返回 400 错误。

## DB 迁移

`db.AutoMigrate` 新增 `Language` 模型。Seeds 在 `AutoMigrate` 后执行：查询 `Language` 表行数，如为 0 则插入全部种子数据。

## 注入关系

`ModelVoiceStore` 新增构造函数参数 `languageStore *LanguageStore`。Voice handler 层在初始化时创建 `LanguageStore` 并传入 `ModelVoiceStore`。

## 文件变更清单

- `internal/store/models.go` — 新增 `Language` 模型
- `internal/store/language.go` — LanguageStore
- `internal/store/db.go` — 注册 AutoMigrate + seed
- `internal/store/voice.go` — 注入 LanguageStore 做 Langs 校验
- `cmd/manager/handler/language.go` — 新 handler
- `cmd/manager/server.go` — 注册路由
