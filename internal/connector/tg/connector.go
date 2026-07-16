// Package tg implements a multi-device Telegram Bot connector.
// Each device can bind its own TG Bot Token. The connector polls the manager
// for all devices with tokens and manages one Bot polling instance per device.
package tg

import (
	"context"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/connector"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/session"
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

// TGConnector implements connector.Connector for Telegram Bot.
// It polls the manager for devices with TG Bot Tokens and manages one
// polling goroutine per token.
type TGConnector struct {
	deps  *connector.Dependencies
	toolsMgr *tools.Manager
	memSvc   *memory.Service

	mu    sync.Mutex
	bots  map[string]*tgBotState // device_id → bot state

	rootCtx    context.Context
	rootCancel context.CancelFunc
	done       chan struct{}
}

// NewTGConnector creates a new Telegram Bot connector.
func NewTGConnector(deps *connector.Dependencies, toolsMgr *tools.Manager, memSvc *memory.Service) *TGConnector {
	return &TGConnector{
		deps:     deps,
		toolsMgr: toolsMgr,
		memSvc:   memSvc,
		bots:     make(map[string]*tgBotState),
		done:     make(chan struct{}),
	}
}

func (c *TGConnector) Name() string { return "tg" }

func (c *TGConnector) Info() connector.ConnectorInfo {
	return connector.NewConnectorInfo(
		"tg",
		"Telegram Bot",
		connector.ConnectorPolling,
		[]connector.Capability{connector.CapText, connector.CapVoiceFile},
	)
}

// Start begins refreshing the device list and starting bots.
func (c *TGConnector) Start(ctx context.Context) error {
	c.rootCtx, c.rootCancel = context.WithCancel(ctx)
	logging.Infof("tg connector: starting, will poll manager for devices with TG bot tokens")
	go c.refreshLoop()
	return nil
}

// Stop stops all bot instances and the refresh loop.
func (c *TGConnector) Stop(ctx context.Context) error {
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
func (c *TGConnector) refreshLoop() {
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

func (c *TGConnector) refresh() {
	devices, err := c.deps.DeviceCfgLoader.ListDevicesWithTGBot()
	if err != nil {
		logging.Errorf("tg connector: refresh failed: %v", err)
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
			logging.Infof("tg connector: stopping bot for device %q", devID)
			state.cancel()
			delete(c.bots, devID)
		}
	}
	c.mu.Unlock()
}

// ensureBotDevice starts a bot for a device if not already running.
func (c *TGConnector) ensureBotDevice(dev connector.DeviceTGBotInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.bots[dev.DeviceID]; ok {
		return // already running
	}

	bot, err := tgbotapi.NewBotAPI(dev.TgBotToken)
	if err != nil {
		logging.Errorf("tg connector: init bot for device %q: %v", dev.DeviceID, err)
		return
	}

	botCtx, cancel := context.WithCancel(c.rootCtx)
	state := &tgBotState{
		deviceID: dev.DeviceID,
		bot:      bot,
		cancel:   cancel,
	}
	c.bots[dev.DeviceID] = state

	logging.Infof("tg connector: bot started for device %q (@%s)", dev.DeviceID, bot.Self.UserName)
	go c.pollBot(botCtx, state)
}

// pollBot runs the polling loop for a single bot instance.
func (c *TGConnector) pollBot(ctx context.Context, state *tgBotState) {
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
func (c *TGConnector) handleUpdate(deviceID string, update tgbotapi.Update) {
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

func (c *TGConnector) handleCommand(ctx context.Context, deviceID string, chatID int64, tgUserID int64, msg *tgbotapi.Message, sess *session.Session, deviceCfg *config.AppConfig) {
	switch msg.Command() {
	case "start":
		c.reply(deviceID, chatID, "你好！我是 Orion-X 智能助手。")
	default:
		c.reply(deviceID, chatID, "未知命令。支持的命令：/start")
	}
}

func (c *TGConnector) handleText(ctx context.Context, deviceID string, chatID int64, tgUserID int64, text string, sess *session.Session, deviceCfg *config.AppConfig) {
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
	agt, err := agent.New(ctx, agentCfg, connMgr, c.memSvc)
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

func (c *TGConnector) reply(deviceID string, chatID int64, text string) {
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
func (c *TGConnector) botForDevice(deviceID string) *tgbotapi.BotAPI {
	c.mu.Lock()
	defer c.mu.Unlock()
	if state, ok := c.bots[deviceID]; ok {
		return state.bot
	}
	return nil
}

// sessionsMu guards the sessions map. Re-initialised per-device.

// getOrCreateSession returns an existing session for this (device, chat) pair.
func (c *TGConnector) getOrCreateSession(deviceID string, chatID int64, deviceCfg *config.AppConfig) *session.Session {
	// Use a simple map for now. For MVP this is fine.
	return session.New(session.SessionMeta{
		Model: deviceCfg.Provider.LLM.OpenAI.Model,
	})
}
