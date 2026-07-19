---
layout: false
---

<script setup>
const managerOrigin = import.meta.env.DEV
  ? "http://localhost:5173"
  : "https://dash.orion-x.org";
const loginURL = `${managerOrigin}/login`;
const agentsURL = `${managerOrigin}/agents`;
</script>

<main class="site-shell">
  <header class="site-header">
    <a class="brand" href="/" aria-label="Orion-X 首页"><span class="brand-mark"></span>Orion-X</a>
    <nav class="header-nav" aria-label="主导航">
      <a href="#build">塑造智能体</a>
      <a href="#continuity">跨渠道连续对话</a>
      <a href="/api-docs">API 文档</a>
      <a href="https://github.com/LiusCraft/orion-x">GitHub</a>
      <a class="nav-cta" :href="loginURL">登录</a>
    </nav>
  </header>

  <section class="hero">
    <div class="hero-grid">
      <div>
        <p class="eyebrow">A conversational agent that stays with you</p>
        <h1>让每一次对话，成为同一段关系的延续。</h1>
        <p class="hero-copy">Orion-X 让你创建会说话、会倾听、会记住的智能体。无论用户通过语音、设备，还是通信渠道联系它，智能体都能带着相同的角色、知识与上下文继续交流。</p>
        <div class="hero-actions">
          <a class="button button-primary" :href="agentsURL">进入控制台</a>
          <a class="button button-secondary" href="#continuity">了解跨渠道对话</a>
        </div>
        <p class="proof">实时语音会话已支持 WebSocket 与 Telegram。更多通信渠道将以同一智能体和同一上下文接入。</p>
      </div>
      <div class="conversation-scene" aria-label="跨渠道对话示例">
        <div class="scene-glow"></div>
        <img class="scene-asset" src="/assets/manager-hero.png" alt="Orion-X 管理能力示意" />
        <div class="chat-window">
          <div class="chat-top"><span class="agent-avatar">OX</span><span class="chat-title">小安</span><span class="chat-sub">VOICE ACTIVE</span></div>
          <div class="chat-body">
            <div class="message user"><span class="wave"><i></i><i></i><i></i><i></i></span>我下午要去北京，提醒我带上演示资料。</div>
            <div class="message agent">好的。我会在出发前提醒你，并把今天会议里的<strong>演示资料清单</strong>一起发给你。</div>
            <div class="context-line">CONTEXT SAVED · 旅行计划 · 演示资料</div>
          </div>
        </div>
      </div>
    </div>
  </section>

  <section id="build" class="section">
    <div class="section-inner">
      <div class="section-lead">
        <p class="section-kicker">Make it yours</p>
        <h2>不是一个固定的机器人，而是你可以持续塑造的对话智能体。</h2>
        <p class="section-desc">从性格与表达方式，到能执行的工具和可检索的知识，都围绕同一个智能体配置。你决定它如何说话、知道什么、又能为用户完成什么。</p>
      </div>
      <div class="build-grid">
        <article class="build-card"><span class="card-index">01</span><h3>角色与规则</h3><p>定义语气、目标和边界，让每次交流都保持一致。</p></article>
        <article class="build-card"><span class="card-index">02</span><h3>声音与表达</h3><p>为智能体选择音色与语音参数，建立可识别的声音形象。</p></article>
        <article class="build-card"><span class="card-index">03</span><h3>工具与能力</h3><p>连接 MCP 服务和业务工具，让对话能够推进到实际行动。</p></article>
        <article class="build-card"><span class="card-index">04</span><h3>知识与记忆</h3><p>把业务知识和长期记忆带入对话，持续理解每位用户。</p></article>
      </div>
    </div>
  </section>

  <section id="continuity" class="section continuity">
    <div class="section-inner">
      <div class="section-lead">
        <p class="section-kicker">One agent, every conversation</p>
        <h2>用户换了渠道，智能体不必从头认识他。</h2>
        <p class="section-desc">渠道只是入口。设备下的会话上下文、记忆与知识库仍属于同一个智能体，让语音通话和文字消息延续为一段完整的对话。</p>
      </div>
      <div class="continuity-layout">
        <div class="channel-list" aria-label="接入渠道">
          <div class="channel"><span class="channel-badge">WS</span><span class="channel-name">网页、App 与语音设备</span><span class="channel-status">AVAILABLE</span></div>
          <div class="channel"><span class="channel-badge">TG</span><span class="channel-name">Telegram Bot</span><span class="channel-status">AVAILABLE</span></div>
          <div class="channel"><span class="channel-badge">QQ</span><span class="channel-name">QQ</span><span class="channel-status planned">PLANNED</span></div>
          <div class="channel"><span class="channel-badge">WX</span><span class="channel-name">微信与企业微信</span><span class="channel-status planned">PLANNED</span></div>
          <div class="channel"><span class="channel-badge">DC</span><span class="channel-name">Discord</span><span class="channel-status planned">PLANNED</span></div>
          <p class="continuity-note">当前版本已实现 WebSocket 实时语音会话和 Telegram 接入。QQ、微信、企业微信及 Discord 是计划中的渠道连接器。</p>
        </div>
        <div class="memory-map" aria-label="统一上下文、记忆与知识库">
          <div class="memory-node node-a">语音通话</div><div class="memory-node node-b">消息对话</div><div class="memory-node node-c">智能设备</div><div class="memory-node node-d">网页与 App</div>
          <div class="memory-core"><b>同一个智能体</b><span>上下文 · 记忆 · 知识库</span></div>
        </div>
      </div>
    </div>
  </section>

  <section class="closing">
    <div class="closing-inner">
      <p class="section-kicker">Orion-X</p>
      <h2>把智能体放进每一次重要的对话里。</h2>
      <p>从一段语音对话开始，逐步接入工具、知识和更多沟通渠道，创建真正能陪伴用户持续交流的智能体。</p>
      <a class="button button-primary" :href="agentsURL">进入控制台</a>
    </div>
    <footer class="site-footer"><span>Orion-X · Conversational agents that continue</span><span>Open source</span></footer>
  </section>
</main>
