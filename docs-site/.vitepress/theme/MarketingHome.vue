<script setup>
import { withBase } from 'vitepress'

const features = [
  { icon: '🎤', title: '实时语音识别', desc: 'ASR 引擎，支持 Silero VAD + 阿里云 DashScope 实时语音转文字' },
  { icon: '🔊', title: '自然语音合成', desc: '多音色可选，异步管道架构实现句间零等待，流畅对话体验' },
  { icon: '🧠', title: '多模型 LLM Agent', desc: 'OpenAI 兼容模型，支持流式输出和完整的工具调用循环' },
  { icon: '🔌', title: 'MCP 工具生态', desc: '标准 MCP 协议扩展，stdio/SSE/Streamable 传输，即插即用' },
  { icon: '💾', title: '智能会话记忆', desc: '短时缓冲 + 长期记忆，Session 管理与上下文持续追踪' },
  { icon: '⚙️', title: '管理控制台', desc: 'Agent / MCP / 模型 / 语音 / 计费一站式管理' },
]

const useCases = [
  { icon: '🎧', title: '智能客服', desc: '语音交互客服系统，自动应答常见问题，转接人工时携带完整上下文' },
  { icon: '🏠', title: '语音助手 / 智能硬件', desc: '嵌入式语音交互，支持自定义唤醒词与技能扩展，资源占用低' },
  { icon: '📝', title: '会议转写与摘要', desc: '实时语音转文字，结合 LLM 自动生成会议摘要和待办事项' },
]

const plans = [
  { name: '开源版', price: '免费', desc: '核心功能全部开源，自建部署', features: ['完整 ASR/TTS 管道', '多模型 LLM 支持', 'MCP 工具扩展', '社区支持'] },
  { name: '企业版', price: '待定', desc: '企业级部署 + 技术支持', features: ['开源版全部功能', '管理控制台', '专属技术支持', '定制开发'] },
]

function scrollTo(id) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })
}
</script>

<template>
  <div class="marketing-wrap">
    <!-- Nav -->
    <nav class="mkt-nav">
      <div class="mkt-nav-inner">
        <a :href="withBase('/')" class="mkt-logo">Orion-X</a>
        <div class="mkt-nav-links">
          <a :href="withBase('/features/')">产品功能</a>
          <a :href="withBase('/use-cases/')">应用场景</a>
          <a :href="withBase('/pricing/')">定价</a>
          <a :href="withBase('/guide/getting-started')">文档</a>
          <a href="https://github.com/LiusCraft/orion-x" class="mkt-nav-gh">GitHub</a>
          <a :href="withBase('/')" class="mkt-nav-cta">控制台 →</a>
        </div>
      </div>
    </nav>

    <!-- Hero -->
    <section class="mkt-hero">
      <div class="mkt-hero-bg"></div>
      <div class="mkt-hero-inner">
        <div class="mkt-hero-text">
          <h1 class="mkt-hero-title">企业级实时语音 AI 平台</h1>
          <p class="mkt-hero-sub">开源 · 低延迟 · 模块化管道架构</p>
          <p class="mkt-hero-desc">从 ASR 到 TTS 端到端可控，基于 Go 构建的生产级实时语音交互平台</p>
          <div class="mkt-hero-actions">
            <a :href="withBase('/guide/getting-started')" class="mkt-btn mkt-btn-primary">开始使用 →</a>
            <a :href="withBase('/architecture/overview')" class="mkt-btn mkt-btn-secondary">查看架构文档 📖</a>
          </div>
          <div class="mkt-hero-metrics">
            <span>⚡ &lt;50ms 语音处理</span>
            <span>✦ 100% 模块化</span>
            <span>★ 完全开源</span>
          </div>
        </div>
        <div class="mkt-hero-visual">
          <div class="pipeline">
            <div class="pipeline-node" style="--c: #3b82f6"><span class="p-node-icon">🎤</span><span>ASR</span></div>
            <div class="pipeline-arrow">→</div>
            <div class="pipeline-node" style="--c: #6366f1"><span class="p-node-icon">🧠</span><span>Agent</span></div>
            <div class="pipeline-arrow">→</div>
            <div class="pipeline-node" style="--c: #8b5cf6"><span class="p-node-icon">🔊</span><span>TTS</span></div>
          </div>
          <div class="pipeline-sub">
            <div class="pipeline-node sm" style="--c: #a855f7"><span class="p-node-icon">🔧</span><span>MCP</span></div>
            <div class="pipeline-node sm" style="--c: #c084fc"><span class="p-node-icon">💾</span><span>Memory</span></div>
          </div>
        </div>
      </div>
    </section>

    <!-- Features -->
    <section id="features" class="mkt-section mkt-features">
      <h2 class="mkt-section-title">核心功能</h2>
      <p class="mkt-section-desc">从语音输入到智能响应，全链路自研可控</p>
      <div class="mkt-grid-3">
        <div v-for="f in features" :key="f.title" class="mkt-card">
          <div class="mkt-card-icon">{{ f.icon }}</div>
          <h3>{{ f.title }}</h3>
          <p>{{ f.desc }}</p>
        </div>
      </div>
    </section>

    <!-- Use Cases -->
    <section id="use-cases" class="mkt-section mkt-cases">
      <h2 class="mkt-section-title">应用场景</h2>
      <p class="mkt-section-desc">灵活适配多种业务场景</p>
      <div class="mkt-grid-3">
        <div v-for="u in useCases" :key="u.title" class="mkt-card mkt-card-case">
          <div class="mkt-card-icon mkt-icon-lg">{{ u.icon }}</div>
          <h3>{{ u.title }}</h3>
          <p>{{ u.desc }}</p>
        </div>
      </div>
    </section>

    <!-- Pricing -->
    <section id="pricing" class="mkt-section mkt-pricing">
      <h2 class="mkt-section-title">定价</h2>
      <p class="mkt-section-desc">选择最适合你的方案</p>
      <div class="mkt-pricing-grid">
        <div v-for="p in plans" :key="p.name" class="mkt-pcard" :class="{ 'mkt-pcard-featured': p.name === '企业版' }">
          <h3>{{ p.name }}</h3>
          <div class="mkt-price">{{ p.price }}</div>
          <p class="mkt-price-desc">{{ p.desc }}</p>
          <ul class="mkt-price-features">
            <li v-for="f in p.features" :key="f">{{ f }}</li>
          </ul>
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="mkt-cta">
      <h2>准备好开始了吗？</h2>
      <p>快速部署，立即体验实时语音 AI 的强大能力</p>
      <a :href="withBase('/guide/getting-started')" class="mkt-btn mkt-btn-primary mkt-btn-lg">快速开始 →</a>
    </section>

    <!-- Footer -->
    <footer class="mkt-footer">
      <div class="mkt-footer-inner">
        <div class="mkt-footer-brand">
          <strong>Orion-X</strong>
          <p>企业级实时语音 AI 平台</p>
        </div>
        <div class="mkt-footer-links">
          <a :href="withBase('/features/')">产品功能</a>
          <a :href="withBase('/use-cases/')">应用场景</a>
          <a :href="withBase('/guide/getting-started')">文档</a>
          <a href="https://github.com/LiusCraft/orion-x">GitHub</a>
        </div>
      </div>
      <div class="mkt-footer-bottom">© 2026 Orion-X. MIT License.</div>
    </footer>
  </div>
</template>

<style scoped>
.marketing-wrap {
  min-height: 100vh;
  font-family: var(--vp-font-family-base, system-ui, sans-serif);
  color: var(--vp-c-text-1);
  background: var(--vp-c-bg);
}

/* Nav */
.mkt-nav {
  position: sticky;
  top: 0;
  z-index: 100;
  background: rgba(255,255,255,.8);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--vp-c-divider);
}
.dark .mkt-nav { background: rgba(26,26,26,.8); }
.mkt-nav-inner {
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 24px;
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.mkt-logo { font-size: 20px; font-weight: 700; color: var(--vp-c-brand-1); text-decoration: none; }
.mkt-nav-links { display: flex; align-items: center; gap: 24px; }
.mkt-nav-links a { color: var(--vp-c-text-2); text-decoration: none; font-size: 14px; }
.mkt-nav-links a:hover { color: var(--vp-c-brand-1); }
.mkt-nav-cta {
  background: var(--vp-c-brand-1); color: #fff !important;
  padding: 6px 16px; border-radius: 8px; font-weight: 500;
}
.mkt-nav-cta:hover { background: var(--vp-c-brand-2); }

/* Hero */
.mkt-hero {
  position: relative;
  overflow: hidden;
  padding: 96px 24px 64px;
}
.mkt-hero-bg {
  position: absolute; inset: 0;
  background: radial-gradient(ellipse 600px 400px at 70% 50%, var(--vp-c-brand-1) 0%, transparent 70%);
  opacity: .08;
}
.mkt-hero-inner {
  max-width: 1280px; margin: 0 auto;
  display: flex; align-items: center; gap: 64px;
}
.mkt-hero-text { flex: 1; }
.mkt-hero-title {
  font-size: 48px; font-weight: 800; line-height: 1.15;
  letter-spacing: -.02em;
}
.mkt-hero-sub {
  font-size: 20px; color: var(--vp-c-brand-1);
  margin-top: 12px; font-weight: 600;
}
.mkt-hero-desc {
  font-size: 16px; color: var(--vp-c-text-2);
  margin-top: 12px; line-height: 1.7; max-width: 480px;
}
.mkt-hero-actions { margin-top: 32px; display: flex; gap: 12px; }
.mkt-btn {
  display: inline-block; padding: 12px 24px; border-radius: 10px;
  font-weight: 600; font-size: 15px; text-decoration: none; cursor: pointer;
  transition: all .2s;
}
.mkt-btn-primary { background: var(--vp-c-brand-1); color: #fff; }
.mkt-btn-primary:hover { background: var(--vp-c-brand-2); }
.mkt-btn-secondary { border: 1px solid var(--vp-c-divider); color: var(--vp-c-text-1); }
.mkt-btn-secondary:hover { border-color: var(--vp-c-brand-1); color: var(--vp-c-brand-1); }
.mkt-btn-lg { padding: 14px 32px; font-size: 16px; }
.mkt-hero-metrics {
  margin-top: 40px; display: flex; gap: 32px;
  font-size: 14px; color: var(--vp-c-text-2);
}
.mkt-hero-visual { flex: 0 0 400px; }
.pipeline {
  display: flex; align-items: center; justify-content: center; gap: 8px;
  padding: 32px; background: var(--vp-c-bg-soft); border-radius: 16px;
  border: 1px solid var(--vp-c-divider);
}
.pipeline-node {
  display: flex; flex-direction: column; align-items: center; gap: 6px;
  padding: 20px 28px; border-radius: 12px;
  background: color-mix(in srgb, var(--c) 10%, var(--vp-c-bg-soft));
  border: 1px solid color-mix(in srgb, var(--c) 30%, var(--vp-c-divider));
  font-size: 14px; font-weight: 600;
}
.pipeline-node.sm { padding: 12px 20px; font-size: 13px; }
.p-node-icon { font-size: 28px; }
.pipeline-arrow { font-size: 24px; color: var(--vp-c-text-3); font-weight: 300; }
.pipeline-sub {
  display: flex; justify-content: center; gap: 8px; margin-top: 8px;
}

/* Sections */
.mkt-section {
  max-width: 1280px; margin: 0 auto; padding: 80px 24px;
}
.mkt-section-title {
  font-size: 32px; font-weight: 700; text-align: center; margin-bottom: 8px;
}
.mkt-section-desc {
  text-align: center; color: var(--vp-c-text-2); margin-bottom: 48px;
}
.mkt-grid-3 {
  display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px;
}
.mkt-card {
  padding: 32px; border-radius: 16px; border: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-soft); transition: all .25s;
}
.mkt-card:hover { border-color: var(--vp-c-brand-1); transform: translateY(-2px); }
.mkt-card-icon { font-size: 32px; margin-bottom: 16px; }
.mkt-card h3 { font-size: 18px; font-weight: 600; margin-bottom: 8px; }
.mkt-card p { font-size: 14px; color: var(--vp-c-text-2); line-height: 1.6; }
.mkt-icon-lg { font-size: 40px; }
.mkt-card-case { text-align: center; }

/* Cases alt background */
.mkt-cases { max-width: none; padding: 80px 24px; background: var(--vp-c-bg-soft); }

/* Pricing */
.mkt-pricing-grid {
  max-width: 720px; margin: 0 auto;
  display: grid; grid-template-columns: repeat(2, 1fr); gap: 24px;
}
.mkt-pcard {
  padding: 32px; border-radius: 16px; border: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-soft); text-align: center;
}
.mkt-pcard-featured {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--vp-c-brand-1) 20%, transparent);
}
.mkt-price {
  font-size: 36px; font-weight: 800; color: var(--vp-c-brand-1);
  margin: 16px 0 8px;
}
.mkt-price-desc { font-size: 14px; color: var(--vp-c-text-2); }
.mkt-price-features {
  list-style: none; padding: 0; margin: 24px 0 0;
  text-align: left; font-size: 14px;
}
.mkt-price-features li::before { content: '✓ '; color: var(--vp-c-brand-1); font-weight: 600; }
.mkt-price-features li + li { margin-top: 10px; }

/* CTA */
.mkt-cta {
  text-align: center; padding: 80px 24px;
  background: var(--vp-c-bg-soft);
}
.mkt-cta h2 { font-size: 28px; font-weight: 700; }
.mkt-cta p { color: var(--vp-c-text-2); margin: 8px 0 24px; }

/* Footer */
.mkt-footer {
  border-top: 1px solid var(--vp-c-divider); padding: 48px 24px 24px;
}
.mkt-footer-inner {
  max-width: 1280px; margin: 0 auto;
  display: flex; justify-content: space-between;
}
.mkt-footer-brand strong { font-size: 18px; }
.mkt-footer-brand p { font-size: 13px; color: var(--vp-c-text-2); margin-top: 4px; }
.mkt-footer-links { display: flex; gap: 20px; }
.mkt-footer-links a { color: var(--vp-c-text-2); text-decoration: none; font-size: 14px; }
.mkt-footer-links a:hover { color: var(--vp-c-brand-1); }
.mkt-footer-bottom {
  max-width: 1280px; margin: 24px auto 0;
  padding-top: 16px; border-top: 1px solid var(--vp-c-divider);
  text-align: center; font-size: 13px; color: var(--vp-c-text-3);
}

/* Responsive */
@media (max-width: 768px) {
  .mkt-nav-links { gap: 12px; }
  .mkt-nav-links a { font-size: 13px; }
  .mkt-hero-inner { flex-direction: column; text-align: center; }
  .mkt-hero-title { font-size: 32px; }
  .mkt-hero-actions { justify-content: center; }
  .mkt-hero-metrics { justify-content: center; flex-wrap: wrap; gap: 16px; }
  .mkt-hero-visual { flex: none; width: 100%; }
  .mkt-grid-3 { grid-template-columns: 1fr; }
  .mkt-pricing-grid { grid-template-columns: 1fr; }
  .mkt-footer-inner { flex-direction: column; gap: 16px; text-align: center; }
  .mkt-footer-links { justify-content: center; }
  .pipeline-node { padding: 14px 18px; }
  .pipeline-node.sm { padding: 10px 14px; }
}
@media (max-width: 640px) {
  .pipeline { flex-wrap: wrap; justify-content: center; }
  .pipeline-arrow { transform: rotate(90deg); }
}
</style>
