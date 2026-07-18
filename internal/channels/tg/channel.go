// Package tg implements a multi-device Telegram Bot channel.
// Each device can bind its own TG Bot Token. The channel polls the manager
// for all devices with tokens and manages one Bot polling instance per device.
package tg

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	providerpool "github.com/liuscraft/orion-x/internal/provider"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
	"github.com/liuscraft/orion-x/internal/tools"
)

// pollInterval is how often we refresh the device list from the manager.
const pollInterval = 30 * time.Second

// tgBotState holds one active Telegram Bot instance for a single device.
type tgBotState struct {
	deviceID string
	bot      *tgbotapi.BotAPI
	cancel   context.CancelFunc // stops this bot's polling goroutine
}

// TGChannel implements channels.Channel for Telegram Bot.
// It polls the manager for devices with TG Bot Tokens and manages one
// polling goroutine per token.
type TGChannel struct {
	deps      *channels.Dependencies
	toolsMgr  *tools.Manager
	memSvc    *memory.Service
	sessions  *session.Manager
	tasks     *task.Registry
	providers *providerpool.Pool

	mu   sync.Mutex
	bots map[string]*tgBotState // device_id → bot state

	rootCtx    context.Context
	rootCancel context.CancelFunc
	done       chan struct{}
}

// NewTGChannel creates a new Telegram Bot channel.
func NewTGChannel(deps *channels.Dependencies, toolsMgr *tools.Manager, memSvc *memory.Service) *TGChannel {
	sessions := tgSessions(deps)
	return &TGChannel{
		deps:      deps,
		toolsMgr:  toolsMgr,
		memSvc:    memSvc,
		sessions:  sessions,
		tasks:     tgTasks(deps, sessions),
		providers: tgProviders(deps),
		bots:      make(map[string]*tgBotState),
		done:      make(chan struct{}),
	}
}

func tgSessions(deps *channels.Dependencies) *session.Manager {
	if deps != nil && deps.Sessions != nil {
		return deps.Sessions
	}
	return session.NewManager()
}

func tgTasks(deps *channels.Dependencies, sessions *session.Manager) *task.Registry {
	if deps != nil && deps.Tasks != nil {
		return deps.Tasks
	}
	return task.NewRegistry(sessions)
}

func tgProviders(deps *channels.Dependencies) *providerpool.Pool {
	if deps != nil && deps.Providers != nil {
		return deps.Providers
	}
	return providerpool.NewPool()
}

func (c *TGChannel) Name() string { return "tg" }

func (c *TGChannel) Info() channels.ChannelInfo {
	return channels.NewChannelInfo(
		"tg",
		"Telegram Bot",
		channels.ChannelPolling,
		[]channels.Capability{channels.CapText, channels.CapVoiceFile},
	)
}

// Start begins refreshing the device list and starting bots.
func (c *TGChannel) Start(ctx context.Context) error {
	c.rootCtx, c.rootCancel = context.WithCancel(ctx)
	logging.Infof("tg channel: starting, will poll manager for devices with TG bot tokens")
	go c.refreshLoop()
	return nil
}

// Stop stops all bot instances and the refresh loop.
func (c *TGChannel) Stop(ctx context.Context) error {
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

// refreshLoop periodically fetches all devices with tg_bot_token from the
// manager and reconciles the active bot instances.
func (c *TGChannel) refreshLoop() {
	defer close(c.done)

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

func (c *TGChannel) refresh() {
	devices, err := c.deps.DeviceCfgLoader.ListDevicesWithTGBot()
	if err != nil {
		logging.Errorf("tg channel: refresh failed: %v", err)
		return
	}

	// Build set of current devices
	current := make(map[string]bool, len(devices))
	for _, d := range devices {
		current[d.DeviceID] = true
		c.ensureBotDevice(d)
	}

	// Stop bots for devices that no longer have tokens
	c.mu.Lock()
	for devID, state := range c.bots {
		if !current[devID] {
			logging.Infof("tg channel: stopping bot for device %q", devID)
			state.cancel()
			delete(c.bots, devID)
		}
	}
	c.mu.Unlock()
}

// ensureBotDevice starts a bot for a device if not already running.
func (c *TGChannel) ensureBotDevice(dev channels.DeviceTGBotInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.bots[dev.DeviceID]; ok {
		return // already running
	}

	bot, err := tgbotapi.NewBotAPI(dev.TgBotToken)
	if err != nil {
		logging.Errorf("tg channel: init bot for device %q: %v", dev.DeviceID, err)
		return
	}

	botCtx, cancel := context.WithCancel(c.rootCtx)
	state := &tgBotState{
		deviceID: dev.DeviceID,
		bot:      bot,
		cancel:   cancel,
	}
	c.bots[dev.DeviceID] = state

	logging.Infof("tg channel: bot started for device %q (@%s)", dev.DeviceID, bot.Self.UserName)
	go c.pollBot(botCtx, state)
}

// pollBot runs the polling loop for a single bot instance.
func (c *TGChannel) pollBot(ctx context.Context, state *tgBotState) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := state.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			c.handleUpdate(state.deviceID, update)
		}
	}
}

// handleUpdate processes a single Telegram update for the given device.
func (c *TGChannel) handleUpdate(deviceID string, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	ctx, cancel := context.WithTimeout(c.rootCtx, 60*time.Second)
	defer cancel()

	chatID := update.Message.Chat.ID
	tgUserID := update.Message.From.ID

	// Load device config for this device
	deviceCfg, err := c.deps.DeviceCfgLoader.LoadConfig(deviceID)
	if err != nil {
		logging.Errorf("tg[%s]: config load failed: %v", deviceID, err)
		c.reply(deviceID, chatID, "设备配置加载失败，请稍后再试。")
		return
	}
	if deviceCfg == nil {
		logging.Errorf("tg[%s]: device not registered", deviceID)
		return
	}

	sess := c.getOrCreateSession(deviceID, chatID, deviceCfg)

	if update.Message.IsCommand() {
		c.handleCommand(ctx, deviceID, chatID, tgUserID, update.Message, sess, deviceCfg)
		return
	}

	if update.Message.Text != "" {
		c.handleText(ctx, deviceID, chatID, tgUserID, update.Message.Text, sess, deviceCfg)
		return
	}

	if update.Message.Voice != nil {
		c.reply(deviceID, chatID, "语音消息功能开发中，请发送文字消息。")
		return
	}
}

func (c *TGChannel) handleCommand(ctx context.Context, deviceID string, chatID int64, tgUserID int64, msg *tgbotapi.Message, sess *session.Session, deviceCfg *config.AppConfig) {
	switch msg.Command() {
	case "start":
		c.reply(deviceID, chatID, "你好！我是 Orion-X 智能助手。")
	default:
		c.reply(deviceID, chatID, "未知命令。支持的命令：/start")
	}
}

func (c *TGChannel) handleText(ctx context.Context, deviceID string, chatID int64, tgUserID int64, text string, sess *session.Session, deviceCfg *config.AppConfig) {
	defer c.sessions.CloseSession(sess.ID)
	logging.Infof("tg[%s]: text from %d: %q", deviceID, tgUserID, text)

	bot := c.botForDevice(deviceID)
	if bot == nil {
		return
	}
	_, _ = bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))

	connMgr := c.toolsMgr.Clone()
	agentCfg := agent.Config{
		Provider:    deviceCfg.Provider.LLM.Type,
		APIKey:      deviceCfg.Provider.LLM.OpenAI.APIKey,
		BaseURL:     deviceCfg.Provider.LLM.OpenAI.BaseURL,
		Model:       deviceCfg.Provider.LLM.OpenAI.Model,
		SoulPrompt:  deviceCfg.Provider.LLM.OpenAI.SoulPrompt,
		RulesPrompt: deviceCfg.Provider.LLM.OpenAI.RulesPrompt,
		ExtraFields: deviceCfg.Provider.LLM.OpenAI.ExtraFields,
	}
	llmClient, err := c.providers.GetOrCreateLLM(ctx, deviceCfg.Provider.LLM.Type, deviceCfg.Provider.LLM.OpenAI)
	if err != nil {
		logging.Errorf("tg[%s]: get LLM client: %v", deviceID, err)
		c.reply(deviceID, chatID, "初始化失败，请稍后再试。")
		return
	}
	for _, spec := range c.tasks.ToolSpecs(sess.ID, func(_ context.Context, taskRecord *task.Task) error {
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
	agt, err := agent.NewWithClient(agentCfg, llmClient, connMgr, c.memSvc)
	if err != nil {
		logging.Errorf("tg[%s]: create agent: %v", deviceID, err)
		c.reply(deviceID, chatID, "初始化失败，请稍后再试。")
		return
	}

	sess.Add(session.Message{Role: session.RoleUser, Content: text})
	eventChan, err := agt.Run(ctx, sess)
	if err != nil {
		logging.Errorf("tg[%s]: agent run error: %v", deviceID, err)
		c.reply(deviceID, chatID, "处理消息时出错。")
		return
	}

	var response string
	for event := range eventChan {
		switch e := event.(type) {
		case *agent.TextChunkEvent:
			response += e.Chunk
		case *agent.FinishedEvent:
			if e.Error != nil {
				logging.Warnf("tg[%s]: agent finished with error: %v", deviceID, e.Error)
			}
		}
	}

	if response == "" {
		response = "（无响应）"
	}
	c.reply(deviceID, chatID, response)
}

func (c *TGChannel) reply(deviceID string, chatID int64, text string) {
	bot := c.botForDevice(deviceID)
	if bot == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		logging.Errorf("tg[%s]: send message to %d failed: %v", deviceID, chatID, err)
	}
}

// botForDevice returns the BotAPI for a device, or nil.
func (c *TGChannel) botForDevice(deviceID string) *tgbotapi.BotAPI {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state, ok := c.bots[deviceID]; ok {
		return state.bot
	}
	return nil
}

// getOrCreateSession returns an existing session for this (device, chat) pair.
func (c *TGChannel) getOrCreateSession(deviceID string, chatID int64, deviceCfg *config.AppConfig) *session.Session {
	id := fmt.Sprintf("tg:%s:%d", deviceID, chatID)
	if sess, ok := c.sessions.Get(id); ok {
		return sess
	}
	sess := session.New(session.SessionMeta{
		Model: deviceCfg.Provider.LLM.OpenAI.Model,
	})
	sess.ID = id
	c.sessions.Add(sess)
	return sess
}
