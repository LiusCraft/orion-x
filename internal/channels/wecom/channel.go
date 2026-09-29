package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/channels/platform"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	providerpool "github.com/liuscraft/orion-x/internal/provider"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
	"github.com/liuscraft/orion-x/internal/tools"
)

const (
	// pollInterval 与 TG 通道一致：每 30s 从 manager 刷新一次设备列表。
	pollInterval = 30 * time.Second

	// sessionIdleTTL 是会话保留窗口：窗口内的后续消息带上会话历史。
	sessionIdleTTL = 30 * time.Minute
	// sweepInterval 是空闲会话回收的检查间隔。
	sweepInterval = time.Minute

	// replyThrottle 是流式刷新间隔。协议限制单会话 30 条/分钟，3s 一次留足余量。
	replyThrottle = 3 * time.Second
	// agentTimeout 是单条消息处理上限（流式消息本身 10 分钟自动结束）。
	agentTimeout = 5 * time.Minute
	// welcomeTimeout 是 enter_chat 事件后发欢迎语的窗口（协议 5s）。
	welcomeTimeout = 5 * time.Second

	// maxReplyBytes 是单条回复内容上限（协议 20480 字节）。
	maxReplyBytes = 20480
	// maxLogTextBytes 是日志里截断后的用户文本长度。
	maxLogTextBytes = 200

	welcomeText     = "你好，我是 Orion-X 智能助手，有什么可以帮你的吗？"
	emptyText       = "请输入文字或语音消息。"
	unsupportedText = "暂不支持该消息类型，请发送文字或语音。"
	replyEmptyText  = "（无响应）"
	configErrorText = "设备配置加载失败，请稍后再试。"
	agentErrorText  = "处理消息时出错，请稍后再试。"
)

// WeComChannel 实现 channels.Channel：轮询 manager 拿到绑定了机器人凭证的设备，
// 每台设备维持一条企业微信长连接，把消息交给 Agent 并用流式消息回复。
type WeComChannel struct {
	deps      *channels.Dependencies
	toolsMgr  *tools.Manager
	memSvc    *memory.Service
	sessions  *session.Manager
	tasks     *task.Registry
	providers *providerpool.Pool

	mu    sync.Mutex
	bots  map[string]*botState  // device_id → 长连接
	chats map[string]*chatState // 会话键 → 会话状态

	rootCtx    context.Context
	rootCancel context.CancelFunc
	wg         sync.WaitGroup
	done       chan struct{}
}

// botState 是一台设备的长连接。
type botState struct {
	deviceID string
	client   *Client
	cancel   context.CancelFunc
}

// chatState 是一个会话（单聊用户或群聊）的处理状态；mu 保证同会话消息串行，
// 也是会话历史的写锁。
type chatState struct {
	key      string
	mu       sync.Mutex
	sess     *session.Session
	lastUsed time.Time
}

// NewWeComChannel 创建企业微信智能机器人通道。
func NewWeComChannel(deps *channels.Dependencies, toolsMgr *tools.Manager, memSvc *memory.Service) *WeComChannel {
	sessions := wecomSessions(deps)
	return &WeComChannel{
		deps:      deps,
		toolsMgr:  toolsMgr,
		memSvc:    memSvc,
		sessions:  sessions,
		tasks:     wecomTasks(deps, sessions),
		providers: wecomProviders(deps),
		bots:      make(map[string]*botState),
		chats:     make(map[string]*chatState),
		done:      make(chan struct{}),
	}
}

func wecomSessions(deps *channels.Dependencies) *session.Manager {
	if deps != nil && deps.Sessions != nil {
		return deps.Sessions
	}
	return session.NewManager()
}

func wecomTasks(deps *channels.Dependencies, sessions *session.Manager) *task.Registry {
	if deps != nil && deps.Tasks != nil {
		return deps.Tasks
	}
	return task.NewRegistry(sessions)
}

func wecomProviders(deps *channels.Dependencies) *providerpool.Pool {
	if deps != nil && deps.Providers != nil {
		return deps.Providers
	}
	return providerpool.NewPool()
}

// Name 返回通道标识。
func (c *WeComChannel) Name() string { return platform.WeCom }

// Info 返回通道元信息。
func (c *WeComChannel) Info() channels.ChannelInfo {
	return channels.NewChannelInfo(
		platform.WeCom,
		"企业微信智能机器人",
		channels.ChannelClient,
		[]channels.Capability{channels.CapText},
	)
}

// Start 启动设备轮询与空闲会话回收。
func (c *WeComChannel) Start(ctx context.Context) error {
	c.rootCtx, c.rootCancel = context.WithCancel(ctx)
	logging.Infof("wecom channel: starting, will poll manager for devices with bot credentials")
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		c.refreshLoop()
	}()
	go func() {
		defer c.wg.Done()
		c.sweepLoop()
	}()
	go func() {
		c.wg.Wait()
		close(c.done)
	}()
	return nil
}

// Stop 断开所有长连接并停止后台循环。
func (c *WeComChannel) Stop(ctx context.Context) error {
	if c.rootCancel != nil {
		c.rootCancel()
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *WeComChannel) refreshLoop() {
	c.refresh()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.rootCtx.Done():
			return
		case <-ticker.C:
			c.refresh()
		}
	}
}

func (c *WeComChannel) sweepLoop() {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.rootCtx.Done():
			return
		case <-ticker.C:
			c.sweepSessions()
		}
	}
}

// refresh 拉取有配置的设备并对齐长连接：新增的建连，消失的断开。
func (c *WeComChannel) refresh() {
	devices, err := c.deps.DeviceCfgLoader.ListDeviceChannels(platform.WeCom)
	if err != nil {
		logging.Errorf("wecom channel: refresh failed: %v", err)
		return
	}

	current := make(map[string]bool, len(devices))
	for _, d := range devices {
		current[d.DeviceID] = true
		c.ensureBot(d)
	}

	c.mu.Lock()
	for devID, st := range c.bots {
		if !current[devID] {
			logging.Infof("wecom channel: stopping bot for device %q", devID)
			st.cancel()
			delete(c.bots, devID)
		}
	}
	c.mu.Unlock()
}

// ensureBot 保证设备的长连接与 manager 下发的凭证一致；凭证变更时重建连接。
func (c *WeComChannel) ensureBot(dev channels.DeviceChannelInfo) {
	botID, secret := dev.Config["bot_id"], dev.Config["bot_secret"]
	if botID == "" || secret == "" {
		return
	}

	c.mu.Lock()
	if st, ok := c.bots[dev.DeviceID]; ok {
		if st.client.botID == botID && st.client.secret == secret {
			c.mu.Unlock()
			return
		}
		logging.Infof("wecom channel: bot credentials changed for device %q, reconnecting", dev.DeviceID)
		st.cancel()
		delete(c.bots, dev.DeviceID)
	}

	botCtx, cancel := context.WithCancel(c.rootCtx)
	client := NewClient(ClientOptions{
		URL:   wsURL(),
		BotID: botID, Secret: secret, Label: dev.DeviceID,
		OnMessage: func(f *frame) { go c.handleMessage(botCtx, dev.DeviceID, f) },
		OnEvent:   func(f *frame) { go c.handleEvent(botCtx, dev.DeviceID, f) },
	})
	c.bots[dev.DeviceID] = &botState{deviceID: dev.DeviceID, client: client, cancel: cancel}
	c.mu.Unlock()

	logging.Infof("wecom channel: starting bot for device %q (bot_id=%s)", dev.DeviceID, botID)
	go client.Run(botCtx)
}

// wsURL 返回长连接地址；私有化部署可用 WECOM_WS_URL 覆盖。
func wsURL() string {
	if v := strings.TrimSpace(os.Getenv("WECOM_WS_URL")); v != "" {
		return v
	}
	return defaultWSURL
}

// handleMessage 处理 aibot_msg_callback：抽取文本 → 跑 Agent → 流式回复。
// 由 OnMessage 在独立 goroutine 里调用，会话内再串行。
func (c *WeComChannel) handleMessage(ctx context.Context, deviceID string, f *frame) {
	var body callbackBody
	if err := json.Unmarshal(f.Body, &body); err != nil {
		logging.Warnf("wecom[%s]: malformed message body: %v", deviceID, err)
		return
	}
	client := c.clientForDevice(deviceID)
	if client == nil {
		return
	}

	text, supported := body.text()
	if !supported {
		logging.Infof("wecom[%s]: unsupported message type %q", deviceID, body.MsgType)
		c.replyText(ctx, client, f.Headers.ReqID, unsupportedText)
		return
	}
	if body.ChatType == chatTypeGroup {
		text = stripMention(text)
	}
	if strings.TrimSpace(text) == "" {
		c.replyText(ctx, client, f.Headers.ReqID, emptyText)
		return
	}

	logging.Infof("wecom[%s]: %s message from %q: %q",
		deviceID, body.ChatType, body.From.UserID, truncateUTF8(text, maxLogTextBytes))
	c.runAgent(ctx, deviceID, f, &body, text)
}

func (c *WeComChannel) handleEvent(ctx context.Context, deviceID string, f *frame) {
	var body callbackBody
	if err := json.Unmarshal(f.Body, &body); err != nil {
		logging.Warnf("wecom[%s]: malformed event body: %v", deviceID, err)
		return
	}
	client := c.clientForDevice(deviceID)
	if client == nil || body.Event == nil {
		return
	}

	switch body.Event.EventType {
	case eventEnterChat:
		c.replyWelcome(ctx, client, f.Headers.ReqID)
	case eventDisconnected:
		logging.Warnf("wecom[%s]: connection dropped by a newer one (bot_id=%s)", deviceID, body.AibotID)
		client.noteKicked()
	default:
		logging.Infof("wecom[%s]: ignoring event %q", deviceID, body.Event.EventType)
	}
}

// runAgent 组装 Agent、执行一轮对话，并把流式输出转成 stream 消息。
func (c *WeComChannel) runAgent(ctx context.Context, deviceID string, f *frame, body *callbackBody, text string) {
	client := c.clientForDevice(deviceID)
	if client == nil {
		return
	}
	deviceCfg, err := c.deps.DeviceCfgLoader.LoadConfig(deviceID)
	if err != nil {
		logging.Errorf("wecom[%s]: load device config: %v", deviceID, err)
		c.replyText(ctx, client, f.Headers.ReqID, configErrorText)
		return
	}
	if deviceCfg == nil {
		logging.Errorf("wecom[%s]: device not registered", deviceID)
		return
	}

	cs := c.chat(deviceID, body, deviceCfg.Provider.LLM.OpenAI.Model)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastUsed = time.Now()

	ctx, cancel := context.WithTimeout(ctx, agentTimeout)
	defer cancel()

	agt, err := c.newAgent(ctx, deviceCfg, cs.sess.ID)
	if err != nil {
		logging.Errorf("wecom[%s]: create agent: %v", deviceID, err)
		c.replyText(ctx, client, f.Headers.ReqID, agentErrorText)
		return
	}

	cs.sess.Add(session.Message{Role: session.RoleUser, Content: text})
	events, err := agt.Run(ctx, cs.sess)
	if err != nil {
		logging.Errorf("wecom[%s]: agent run: %v", deviceID, err)
		c.replyText(ctx, client, f.Headers.ReqID, agentErrorText)
		return
	}

	stream := newStreamReply(client, f.Headers.ReqID)
	for event := range events {
		switch e := event.(type) {
		case *agent.TextChunkEvent:
			stream.push(ctx, e.Chunk)
		case *agent.FinishedEvent:
			if e.Error != nil {
				logging.Warnf("wecom[%s]: agent finished with error: %v", deviceID, e.Error)
			}
		}
	}
	stream.finish(ctx)
}

// newAgent 组装一次回答所用的 Agent：克隆工具集、挂上会话任务、复用进程级 LLM 连接池。
func (c *WeComChannel) newAgent(ctx context.Context, deviceCfg *config.AppConfig, sessionID string) (*agent.Agent, error) {
	connMgr := c.toolsMgr.Clone()
	agentCfg := agent.Config{
		Provider:        deviceCfg.Provider.LLM.Type,
		APIKey:          deviceCfg.Provider.LLM.OpenAI.APIKey,
		BaseURL:         deviceCfg.Provider.LLM.OpenAI.BaseURL,
		Model:           deviceCfg.Provider.LLM.OpenAI.Model,
		SoulPrompt:      deviceCfg.Provider.LLM.OpenAI.SoulPrompt,
		RulesPrompt:     deviceCfg.Provider.LLM.OpenAI.RulesPrompt,
		ExtraFields:     deviceCfg.Provider.LLM.OpenAI.ExtraFields,
		ProviderOptions: deviceCfg.Provider.LLM.OpenAI.Options,
		Thinking:        deviceCfg.Provider.LLM.OpenAI.Thinking,
		MaxOutputTokens: deviceCfg.Provider.LLM.OpenAI.MaxOutputTokens,
	}
	llmClient, err := c.providers.GetOrCreateLLM(ctx, deviceCfg.Provider.LLM.Type, deviceCfg.Provider.LLM.OpenAI)
	if err != nil {
		return nil, fmt.Errorf("get llm client: %w", err)
	}
	for _, spec := range c.tasks.ToolSpecs(sessionID, func(_ context.Context, taskRecord *task.Task) error {
		taskMgr := connMgr.Clone()
		taskAgent, err := agent.NewWithClient(agentCfg, llmClient, taskMgr, c.memSvc)
		if err != nil {
			return err
		}
		taskSession := session.New(session.SessionMeta{Model: agentCfg.Model})
		taskSession.Add(session.Message{Role: session.RoleUser, Content: taskRecord.Title})
		subAgent := agent.NewSubAgent("sub_"+taskRecord.ID, taskRecord.ID, taskAgent)
		if err := subAgent.Start(c.rootCtx, taskSession); err != nil {
			return err
		}
		return c.tasks.AttachSubAgent(taskRecord.ID, subAgent)
	}) {
		connMgr.Registry().Add(spec)
	}
	return agent.NewWithClient(agentCfg, llmClient, connMgr, c.memSvc)
}

// chat 返回会话状态，不存在则新建并登记到共享的 session.Manager。
// 调用方拿到后必须持有 cs.mu 再读写会话。
func (c *WeComChannel) chat(deviceID string, body *callbackBody, model string) *chatState {
	key := conversationKey(deviceID, body)

	c.mu.Lock()
	if cs, ok := c.chats[key]; ok {
		c.mu.Unlock()
		return cs
	}
	sess := session.New(session.SessionMeta{Model: model})
	sess.ID = key
	cs := &chatState{key: key, sess: sess, lastUsed: time.Now()}
	c.chats[key] = cs
	c.mu.Unlock()

	c.sessions.Add(sess)
	logging.Infof("wecom[%s]: session %s opened", deviceID, key)
	return cs
}

// sweepSessions 回收空闲超过 sessionIdleTTL 的会话；先拿会话锁再删，避免与正在
// 执行的一轮对话抢同一个 session。
func (c *WeComChannel) sweepSessions() {
	cutoff := time.Now().Add(-sessionIdleTTL)

	c.mu.Lock()
	idle := make([]*chatState, 0, len(c.chats))
	for _, cs := range c.chats {
		idle = append(idle, cs)
	}
	c.mu.Unlock()

	for _, cs := range idle {
		cs.mu.Lock()
		if cs.lastUsed.Before(cutoff) {
			c.mu.Lock()
			if c.chats[cs.key] == cs {
				delete(c.chats, cs.key)
			}
			c.mu.Unlock()
			c.sessions.CloseSession(cs.sess.ID)
			logging.Infof("wecom: session %s idle for %s, closed", cs.key, sessionIdleTTL)
		}
		cs.mu.Unlock()
	}
}

func (c *WeComChannel) clientForDevice(deviceID string) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.bots[deviceID]; ok {
		return st.client
	}
	return nil
}

// replyWelcome 在 enter_chat 事件后回复欢迎语。
func (c *WeComChannel) replyWelcome(ctx context.Context, client *Client, reqID string) {
	ctx, cancel := context.WithTimeout(ctx, welcomeTimeout)
	defer cancel()

	body := textBody{MsgType: msgTypeText, Text: textContent{Content: welcomeText}}
	if err := client.Reply(ctx, reqID, cmdRespondWelcome, body, ackTimeout); err != nil {
		logging.Warnf("wecom: welcome reply failed: %v", err)
	}
}

// replyText 用一条已完成的流式消息回复固定文案（错误提示、不支持的类型等）。
func (c *WeComChannel) replyText(ctx context.Context, client *Client, reqID, content string) {
	body := streamBody{
		MsgType: msgTypeStream,
		Stream: streamContent{
			ID:      newStreamID(),
			Finish:  true,
			Content: truncateUTF8(content, maxReplyBytes),
		},
	}
	if err := client.Reply(ctx, reqID, cmdRespondMsg, body, ackTimeout); err != nil {
		logging.Warnf("wecom: reply failed: %v", err)
	}
}

// streamReply 把 Agent 的流式输出转成 aibot_respond_msg 的 stream 帧：首次发送
// 创建消息，之后按 replyThrottle 刷新同一个 stream.id，finish=true 收尾。
type streamReply struct {
	client *Client
	reqID  string
	id     string
	text   strings.Builder
	last   time.Time
}

func newStreamReply(client *Client, reqID string) *streamReply {
	return &streamReply{client: client, reqID: reqID, id: newStreamID()}
}

// push 追加一段增量；距上次发送超过节流间隔才真正发出。
func (s *streamReply) push(ctx context.Context, chunk string) {
	s.text.WriteString(chunk)
	if time.Since(s.last) < replyThrottle {
		return
	}
	s.send(ctx, false)
}

// finish 以 finish=true 收尾；没有任何产出时补一句占位文案。
func (s *streamReply) finish(ctx context.Context) {
	if s.text.Len() == 0 {
		s.text.WriteString(replyEmptyText)
	}
	s.send(ctx, true)
}

func (s *streamReply) send(ctx context.Context, finish bool) {
	content := truncateUTF8(s.text.String(), maxReplyBytes)
	if content == "" {
		return
	}
	body := streamBody{
		MsgType: msgTypeStream,
		Stream:  streamContent{ID: s.id, Finish: finish, Content: content},
	}
	if err := s.client.Reply(ctx, s.reqID, cmdRespondMsg, body, ackTimeout); err != nil {
		logging.Warnf("wecom: stream reply (finish=%v) failed: %v", finish, err)
		return
	}
	s.last = time.Now()
}

func newStreamID() string {
	return "stream:" + uuid.NewString()
}
