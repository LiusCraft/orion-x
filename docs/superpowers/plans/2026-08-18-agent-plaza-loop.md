# 智能体广场闭环完善 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让"基于此创建"的智能体携带真实模板配置（prompt）且运行时生效，杜绝空壳 voicebot。

**Architecture:** 服务端三处改动：create 时校验/归一化 config_json（空壳归 DefaultConfig）、运行时 DeviceConfig 按"顶层是否有 provider 键"决定 AgentConfig 组装或旧式原样返回、种子模板补 AgentConfig 形状的 prompt；前端清理广场页死代码并补错误提示。

**Tech Stack:** Go (gin, gorm), React (Vite + TS), 现有 `cmd/manager/handler/` 纯函数测试惯例（见 `device_test.go`）。

**验证命令：**
- `go test ./...`
- `golangci-lint run ./...`
- `cd web/manager && npm run lint && npm run build`

---

### Task 1: Create 校验与归一化

**Files:**
- Modify: `cmd/manager/handler/voicebot.go:34-61`
- Create: `cmd/manager/handler/voicebot_test.go`
- Test: `go test ./cmd/manager/handler/ -run TestNormalizeCreateConfig -v`

- [ ] **Step 1: Write the failing test**

Create `cmd/manager/handler/voicebot_test.go`:

```go
package handler

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/config"
)

func TestNormalizeCreateConfigEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"null", "null"},
		{"empty object", "{}"},
		{"whitespace object", "  {  }  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeCreateConfig(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want, _ := json.Marshal(config.DefaultConfig())
			if got != string(want) {
				t.Fatalf("got %s, want default config", got)
			}
		})
	}
}

func TestNormalizeCreateConfigInvalidJSON(t *testing.T) {
	if _, err := normalizeCreateConfig(json.RawMessage(`{bad`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNormalizeCreateConfigPartialAgentConfig(t *testing.T) {
	raw := `{"llm":{"soul_prompt":"你好"}}`
	got, err := normalizeCreateConfig(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != raw {
		t.Fatalf("partial config must be preserved: got %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/manager/handler/ -run TestNormalizeCreateConfig -v`
Expected: FAIL — `undefined: normalizeCreateConfig`

- [ ] **Step 3: Add normalizeCreateConfig**

In `cmd/manager/handler/voicebot.go`, add `"strings"` to imports (keep `encoding/json`, add nothing else new beyond it), and add above `Create`:

```go
// normalizeCreateConfig 校验并归一化创建 voicebot 时的 config_json。
// 空/空对象配置归一化为 DefaultConfig，非法 JSON 返回错误；其余原样保留。
func normalizeCreateConfig(raw json.RawMessage) (string, error) {
	cfgJSON := strings.TrimSpace(string(raw))
	if cfgJSON == "" || cfgJSON == "null" {
		return defaultConfigJSON()
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &m); err != nil {
		return "", err
	}
	if len(m) == 0 {
		return defaultConfigJSON()
	}
	return cfgJSON, nil
}

func defaultConfigJSON() (string, error) {
	b, err := json.Marshal(config.DefaultConfig())
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

- [ ] **Step 4: Wire into Create handler**

Replace the body of `Create` in `cmd/manager/handler/voicebot.go` from `cfgJSON := string(req.ConfigJSON)` through the `if cfgJSON == "" || cfgJSON == "null" { ... }` block with:

```go
	cfgJSON, err := normalizeCreateConfig(req.ConfigJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_json: " + err.Error()})
		return
	}

	userID := middleware.UserID(c)
	v, err := h.voicebots.Create(req.Name, userID, cfgJSON, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
```

Note: `err` is now declared by `normalizeCreateConfig`; the later `if err != nil` reuses it — remove the old `var`/`:=` conflict by ensuring only one `cfgJSON, err :=` declaration remains.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/manager/handler/ -run TestNormalizeCreateConfig -v`
Expected: PASS (4 subcases empty + 1 invalid + 1 partial)

- [ ] **Step 6: Commit**

```bash
git add cmd/manager/handler/voicebot.go cmd/manager/handler/voicebot_test.go
git commit -m "fix: validate and normalize config_json on voicebot create"
```

---

### Task 2: DeviceConfig 组装判定

**Files:**
- Modify: `cmd/manager/handler/internal.go:53-62`
- Create: `cmd/manager/handler/internal_test.go`
- Test: `go test ./cmd/manager/handler/ -run TestHasTopLevelProvider -v`

- [ ] **Step 1: Write the failing test**

Create `cmd/manager/handler/internal_test.go`:

```go
package handler

import "testing"

func TestHasTopLevelProvider(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"legacy AppConfig", `{"provider":{"asr":{"type":"aliyun"}}}`, true},
		{"partial agent config", `{"llm":{"soul_prompt":"x"}}`, false},
		{"empty object", `{}`, false},
		{"invalid json", `{bad`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasTopLevelProvider(tc.json); got != tc.want {
				t.Fatalf("hasTopLevelProvider(%q) = %v, want %v", tc.json, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/manager/handler/ -run TestHasTopLevelProvider -v`
Expected: FAIL — `undefined: hasTopLevelProvider`

- [ ] **Step 3: Add hasTopLevelProvider**

In `cmd/manager/handler/internal.go`, add near the top (after imports):

```go
// hasTopLevelProvider 判断 config_json 是否为旧式完整 AppConfig（含顶层 provider 段）。
// 含 provider 段 → 旧式路径原样返回；否则视为 AgentConfig 走运行时组装。
func hasTopLevelProvider(configJSON string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(configJSON), &m); err != nil {
		return false
	}
	_, ok := m["provider"]
	return ok
}
```

(`encoding/json` is already imported in internal.go.)

- [ ] **Step 4: Wire into DeviceConfig**

Replace `cmd/manager/handler/internal.go:54` condition:

```go
	if err := json.Unmarshal([]byte(v.ConfigJSON), &agentCfg); err == nil && !hasTopLevelProvider(v.ConfigJSON) {
```

`DeviceConfig` 其余逻辑不变：AgentConfig 组装失败/含 provider 键 → 落入旧式 AppConfig 原样返回分支。

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/manager/handler/ -run TestHasTopLevelProvider -v`
Expected: PASS (4 subcases)

- [ ] **Step 6: Commit**

```bash
git add cmd/manager/handler/internal.go cmd/manager/handler/internal_test.go
git commit -m "fix: assemble agent config when provider section absent"
```

---

### Task 3: 模板内容充实

**Files:**
- Modify: `internal/store/agent_template_seed.go`（`defaultTemplates()` 中 8 个空 ConfigJSON 的模板）
- Create: `internal/store/agent_template_seed_test.go`
- Test: `go test ./internal/store/ -run TestDefaultTemplatesConfigs -v`

- [ ] **Step 1: Write the failing test**

Create `internal/store/agent_template_seed_test.go`:

```go
package store

import (
	"encoding/json"
	"testing"
)

func TestDefaultTemplatesConfigs(t *testing.T) {
	tmpls := defaultTemplates()
	if len(tmpls) == 0 {
		t.Fatal("no default templates")
	}
	for _, tp := range tmpls {
		t.Run(tp.Name, func(t *testing.T) {
			if tp.ConfigJSON == nil {
				t.Fatal("template has nil config")
			}
			b, err := json.Marshal(tp.ConfigJSON)
			if err != nil {
				t.Fatalf("marshal config: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("config is not a valid JSON object: %v", err)
			}
			if len(m) == 0 {
				t.Fatal("template config must not be empty")
			}
			llm, ok := m["llm"].(map[string]any)
			if !ok {
				t.Fatalf("config must contain llm section, got %v", m)
			}
			if llm["soul_prompt"] == nil && llm["rules_prompt"] == nil {
				t.Fatal("llm section must contain soul_prompt or rules_prompt")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestDefaultTemplatesConfigs -v`
Expected: FAIL — `template config must not be empty`（8 个模板）

- [ ] **Step 3: Fill in the 8 template configs**

In `internal/store/agent_template_seed.go`, replace each of the 8 `ConfigJSON:  map[string]any{},` (对话/代码/知识库/旅行/健康/数据/语音播报) or `ConfigJSON:  map[string]any{},`（内容创作）with the following. 通用对话助手:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是 Orion，一个通用 AI 助手。回答要准确、简洁、有条理，先理解用户意图再作答；不确定时如实说明，不编造事实。支持多轮对话、工具调用和长期记忆，可在需要时主动询问补充信息。",
				},
			},
```

代码智能体:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是代码专家。回答代码问题时先给出结论，再解释关键思路；默认提供可运行的完整代码并标注关键点。调试问题时先定位原因再给出修复方案，避免猜测式修改。涉及编译或运行命令时直接给出可执行命令。",
				},
			},
```

内容创作助手:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"soul_prompt": "你是专业内容创作者，擅长文案、剧本与故事创作。动笔前先确认风格基调与目标受众；文字要有画面感和情绪层次，避免套话和空洞堆砌。用户给出主题后，主动提供 2-3 个不同角度供选择。",
				},
			},
```

知识库问答:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是知识库问答助手。回答必须基于知识库内容并注明信息来源；知识库没有答案时明确告知，不得编造。回答结构清晰，先给结论再展开细节，并提示用户可通过追问深入检索。",
				},
			},
```

旅行规划师:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是旅行规划师。规划行程时先确认目的地、天数、预算与偏好；按天输出行程，标注交通方式、景点开放时间和用餐建议；最后给出出行提醒（证件、天气、保险）。预算紧张时主动推荐替代方案。",
				},
			},
```

健康问诊助手:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是健康咨询助手，回答仅供参考，不构成专业医疗建议。描述症状时先询问关键细节（持续时间、严重程度、伴随症状），再给出可能原因与日常护理建议；涉及处方药、急重症或持续症状时，明确建议尽快就医。",
				},
			},
```

数据分析师:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是数据分析师。拿到数据先说明数据范围与字段含义，再给出分析思路；结论必须有数据支撑，关键指标给出计算口径；输出图表时说明洞察点，避免仅罗列数字。发现异常数据时先求证再下结论。",
				},
			},
```

语音播报员:

```go
			ConfigJSON: map[string]any{
				"llm": map[string]any{
					"rules_prompt": "你是语音播报型助手，回答用于语音合成播放。用词口语化、句子短、结构简单，避免括号、列表、代码与生僻词；一次回答聚焦一个主题，先说结论再补细节，让听众一遍听懂。",
				},
			},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestDefaultTemplatesConfigs -v`
Expected: PASS（9 个子测试，8 个新填充 + 小暖）

- [ ] **Step 5: Commit**

```bash
git add internal/store/agent_template_seed.go internal/store/agent_template_seed_test.go
git commit -m "feat: enrich agent template configs with prompts"
```

---

### Task 4: 前端清理

**Files:**
- Modify: `web/manager/src/pages/agents/AgentPlazaPage.tsx`
- Test: `cd web/manager && npm run lint && npm run build`

- [ ] **Step 1: Add error state and remove dead code**

In `web/manager/src/pages/agents/AgentPlazaPage.tsx`:

1. 在 state 声明处（第 17 行 `const [using, setUsing] = ...` 之后）加：

```tsx
	const [error, setError] = useState("");
```

2. 删除第 58-59 行：

```tsx
	// 将现有硬编码模板转换为 API 格式，作为 fallback
	const hasData = templates.length > 0;
```

3. 第 96 行 `{hasData && (` 改为 `{categories.length > 1 && (`（`categories` 恒含"全部"，`> 1` 表示存在真实分类）。

- [ ] **Step 2: Add error reporting in handleUseTemplate**

将 `handleUseTemplate`（第 45-56 行）替换为：

```tsx
	const handleUseTemplate = async (tpl: AgentTemplate) => {
		setUsing(tpl.id);
		setError("");
		try {
			const { data } = await agentTemplateApi.use(tpl.id);
			// 用模板的名称和配置创建 voicebot
			const configJSON = JSON.stringify(data.config);
			const res = await voicebotApi.create(data.name, configJSON);
			navigate(`/agents/${res.data.id}`);
		} catch (err) {
			setUsing(null);
			setError(err instanceof Error ? err.message : "创建失败，请重试");
		}
	};
```

- [ ] **Step 3: Remove pointless setUsing and render error**

1. 删除"从零创建"按钮 onClick（第 74 行）中的 `setUsing("_new");`：

```tsx
					<Button
						onClick={() => {
							navigate("/agents");
						}}
```

2. 在网格容器 `<div className="px-8 py-6">` 内、`{loading ?` 之前插入错误提示：

```tsx
			{error && <p className="text-xs text-red-400 mb-3">{error}</p>}
```

- [ ] **Step 4: Lint and build**

Run: `cd web/manager && npm run lint && npm run build`
Expected: 无 lint 错误，build 成功

- [ ] **Step 5: Commit**

```bash
git add web/manager/src/pages/agents/AgentPlazaPage.tsx
git commit -m "refactor: clean up agent plaza page and surface create errors"
```

---

### Task 5: 全量验证

- [ ] **Step 1: Go 全量测试 + lint**

Run: `go test ./...`
Expected: 全部 PASS

Run: `golangci-lint run ./...`
Expected: 无告警

- [ ] **Step 2: 手工验证闭环（可选，需本地 DB + 登录态）**

1. 启动 manager（`go run ./cmd/manager`），登录后打开 `/agents/plaza`
2. 点"小暖 · 女友陪伴"的"基于此创建" → 跳转详情页 → 人设 prompt 已填充到灵魂设定输入框
3. `GET /internal/device-config?device_id=<绑定该 voicebot 的设备>` → 返回含 `soul_prompt` 的完整 provider 配置（走 AgentConfig 组装路径）
4. 用"从零创建"建一个空 agent → 详情页配置非空（DefaultConfig 起点）

- [ ] **Step 3: 提交剩余变更（若有）**

```bash
git status
```

若 `git status` 为空，说明所有变更已提交；否则按改动内容补充提交。
