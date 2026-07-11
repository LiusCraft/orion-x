# Orion-X 营销官网设计

> 基于 VitePress 构建的产品营销官网 + 技术文档站一体化站点。

## 背景

Orion-X 是一个开源实时语音 AI 平台（Go 语言）。当前已有基于 VitePress 的技术文档站，部署在 GitHub Pages。现需要在其基础上扩展一套营销官网页面，面向企业/产品决策者。

## 目标受众

企业/产品决策者 — 需要语音 AI 方案的管理者，关注产品能力、稳定性、集成方式。

## 技术方案

**基于现有 VitePress 扩展**，而非新建独立站点。

| 决策 | 理由 |
| ------ | ------ |
| VitePress (非 Next.js/Nuxt) | 已有 VitePress + CI/CD pipeline，零新增框架/运维 |
| 同一 repo 同一 deploy | 复现 `.github/workflows/deploy-docs.yml`，同一 `gh-pages` 分支 |
| Markdown + Vue 组件 | 内容用 md 写，复杂交互用 Vue 组件插进去 |
| 自定义首页 Layout | VitePress 支持自定义 `layout`，首页用 marketing 风格替换默认 |

## 站点结构

```
/                          ← 营销首页（自定义 layout）
/features/                 ← 产品功能详情页
/use-cases/                ← 应用场景/案例
/pricing/                  ← 定价
/docs/                     ← 现有文档站（VitePress 默认 theme）
  /guide/
  /architecture/
```

**导航栏**：首页 | 产品功能 | 应用场景 | 定价 | 文档 | 控制台 | GitHub

## 首页设计

### 导航栏

- Logo + 链接列表
- 固定顶部，滚动毛玻璃效果
- 右侧 primary button "控制台" → 指向 `web/manager`

### Hero 区

- 标题：**企业级实时语音 AI 平台**
- 副标题：开源 · 低延迟 · 模块化管道架构 — 从 ASR 到 TTS 端到端可控
- 左侧文案 + 右侧 Pipeline 流程插图（ASR→Agent→TTS 示意图）
- 两个 CTA：[开始使用 →] [查看架构文档]
- 底部三个数据点：⚡ <50ms 语音处理 · 100% 模块化 · 完全开源

### 核心功能 (Features)

6 卡片网格：

| # | Icon | 标题 | 说明 |
| --- | ------ | ------ | ------ |
| 1 | 🎤 | 实时语音识别 | ASR 引擎，Silero VAD + 阿里云 DashScope |
| 2 | 🔊 | 自然语音合成 | 多音色，异步管道零等待 |
| 3 | 🧠 | 多模型 LLM Agent | OpenAI 兼容，工具调用循环 |
| 4 | 🔌 | MCP 工具生态 | 标准协议扩展，即插即用 |
| 5 | 💾 | 智能会话记忆 | 短时缓冲 + 长期记忆 |
| 6 | ⚙️ | 管理控制台 | Agent/MCP/模型/计费一站式管理 |

### 应用场景

3 张场景卡片 + 小插图：

- **智能客服** — 耳机 + 聊天气泡
- **语音助手/智能硬件** — 音箱 + 声波扩散功能图标
- **会议转写** — 多头像 + 实时文字流

### Pricing 区

可选的 2-3 档定价卡片（开源免费 + 企业版等），暂放占位内容。

### Footer

- Logo + 链接
- GitHub 链接
- © 2026 Orion-X

## 视觉风格

| 元素 | 值 |
| ------ | ----- |
| 主色 | Blue-600 (#2563eb) |
| 辅助色 | Slate/灰色系 |
| 字体 | system-ui, sans-serif |
| 圆角 | 12-16px 大圆角 |
| 插画风格 | 线稿 + 渐变填充，简洁技术风 |

## 实施范围

1. `docs-site/.vitepress/theme/` — 自定义首页 Layout 和组件
2. `docs-site/index.md` — 改为只做路由，layout 使用自定义首页
3. `docs-site/features/` — 功能详情页
4. `docs-site/use-cases/` — 应用场景页
5. `docs-site/pricing/` — 定价页
6. `docs-site/.vitepress/config.js` — 更新 nav 和 sidebar

## 插图方案

Hero 主图用内嵌 SVG/HTML 实现 pipeline 流程图，场景小插画用 SVG 内联或 emoji + 文字占位。矢量图后期可以替换为设计师产出。

## 不在此范围

- web/manager 控制台本身的 UI 改造
- 现有文档内容的重新组织
- 多语言/i18n
- 自定义域名/CNAME 配置
