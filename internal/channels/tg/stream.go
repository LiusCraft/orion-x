package tg

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// draftThrottle 是私聊草稿更新的最小间隔。草稿不受消息发送配额限制，走的是
	// typing 类限流（官方 AI 文档：与 messages.setTyping 同等），300ms 接近逐字观感。
	draftThrottle = 300 * time.Millisecond
	// editThrottleGroup 是群聊原地刷新的最小间隔：群聊没有草稿接口，且限制
	// 20 条/分钟（Bot FAQ），4s 一次留出余量。
	editThrottleGroup = 4 * time.Second

	// maxMessageUnits 是单条消息与草稿的字符上限（Bot API 限制 4096，按 UTF-16 码元计）。
	maxMessageUnits = 4096

	// draftFailureLimit 是一轮里草稿连续失败几次后放弃：草稿只是预览，收尾的
	// sendMessage 才是交付，不必反复重试。
	draftFailureLimit = 2

	// replyEmptyText 是 Agent 没有任何产出时的占位回复。
	replyEmptyText = "（无响应）"
)

// replyStream 是一轮回复的流式输出面：start 开启输出，push 追加文本增量，
// finish 收尾。
type replyStream interface {
	start()
	push(chunk string)
	finish()
}

// newReplyStream 按聊天类型选择实现：草稿接口只支持私聊，其他聊天退回原地编辑。
func newReplyStream(bot *tgbotapi.BotAPI, deviceID string, chat *tgbotapi.Chat) replyStream {
	if chat.IsPrivate() {
		return newDraftReply(bot, deviceID, chat)
	}
	return newEditReply(bot, deviceID, chat)
}

// draftSeq 给每轮回复分配非零 draft_id：同一个 id 的草稿更新在客户端带动画。
var draftSeq atomic.Int64

// draftReply 用 sendMessageDraft 把生成中的内容作为私聊草稿预览。草稿是临时的
// 30 秒预览，不留在会话里，收尾时必须再用 sendMessage 把完整文本落地。
// 只由处理该消息的单个 goroutine 使用，不是并发安全的。
type draftReply struct {
	bot      *tgbotapi.BotAPI
	deviceID string
	chatID   int64
	draftID  int64
	throttle time.Duration

	text   strings.Builder // 本轮已产生的全部文本
	shown  string          // 草稿里已展示的内容（按上限截断后的头部）
	last   time.Time       // 上次成功更新草稿的时间
	opened bool            // 是否已经成功发过草稿

	failures int  // 连续失败次数
	disabled bool // 连续失败超限后放弃本轮剩余的草稿
}

// newDraftReply 创建私聊草稿流。
func newDraftReply(bot *tgbotapi.BotAPI, deviceID string, chat *tgbotapi.Chat) *draftReply {
	return &draftReply{
		bot:      bot,
		deviceID: deviceID,
		chatID:   chat.ID,
		draftID:  draftSeq.Add(1),
		throttle: draftThrottle,
	}
}

// start 在生成开始前先发一条空草稿：客户端把它渲染成「Thinking…」占位，
// 用户不必等第一个 token 就有反馈。
func (s *draftReply) start() {
	s.flushDraft()
}

// push 追加一个文本增量：首块立即出草稿，其余按节流更新。
func (s *draftReply) push(chunk string) {
	if chunk == "" {
		return
	}
	s.text.WriteString(chunk)
	if s.opened && time.Since(s.last) < s.throttle {
		return
	}
	s.flushDraft()
}

// finish 收尾：草稿不会留在会话里，用 sendMessage 把完整文本落地；超过单条上限
// 时拆成多条发送。
func (s *draftReply) finish() {
	content := s.text.String()
	if content == "" {
		content = replyEmptyText
	}
	for content != "" {
		if units(content) <= maxMessageUnits {
			s.sendMessage(content)
			return
		}
		head, tail := splitToUnits(content, maxMessageUnits)
		if !s.sendMessage(head) {
			return
		}
		content = tail
	}
}

// flushDraft 把当前全文（头部截断到上限）作为草稿预览推给客户端；内容没变则跳过。
// 连续失败达到上限后放弃本轮草稿：收尾的 sendMessage 仍会把全文送达。
func (s *draftReply) flushDraft() {
	if s.disabled {
		return
	}
	content := truncateToUnits(s.text.String(), maxMessageUnits)
	if s.opened && content == s.shown {
		return
	}
	params := tgbotapi.Params{
		"chat_id":  strconv.FormatInt(s.chatID, 10),
		"draft_id": strconv.FormatInt(s.draftID, 10),
		"text":     content,
	}
	if _, err := s.bot.MakeRequest("sendMessageDraft", params); err != nil {
		s.failures++
		if s.failures >= draftFailureLimit {
			s.disabled = true
			logging.Warnf("tg[%s]: draft streaming to chat %d disabled after %d failures: %v", s.deviceID, s.chatID, s.failures, err)
			return
		}
		logging.Warnf("tg[%s]: draft reply to chat %d: %v", s.deviceID, s.chatID, err)
		return
	}
	if !s.opened {
		logging.Infof("tg[%s]: draft streaming started (chat %d)", s.deviceID, s.chatID)
	}
	s.shown = content
	s.last = time.Now()
	s.opened = true
	s.failures = 0
}

// sendMessage 发一条落地消息。
func (s *draftReply) sendMessage(content string) bool {
	if _, err := s.bot.Send(tgbotapi.NewMessage(s.chatID, content)); err != nil {
		logging.Warnf("tg[%s]: send reply to chat %d: %v", s.deviceID, s.chatID, err)
		return false
	}
	return true
}

// editReply 在群聊里用原地编辑刷新回复：首个增量创建消息，之后按节流编辑同一条；
// 超过单条上限时定稿当前消息、另起一条继续。只由处理该消息的单个 goroutine 使用，
// 不是并发安全的。
type editReply struct {
	bot      *tgbotapi.BotAPI
	deviceID string
	chatID   int64
	throttle time.Duration

	messageID int             // 当前在编辑的消息；0 = 还没成功发出过
	sent      string          // 当前消息里已经可见的内容
	pending   strings.Builder // 已产生、尚未刷新的增量
	last      time.Time       // 上次成功发送/编辑的时间，节流用
}

// newEditReply 创建群聊原地刷新流。
func newEditReply(bot *tgbotapi.BotAPI, deviceID string, chat *tgbotapi.Chat) *editReply {
	return &editReply{bot: bot, deviceID: deviceID, chatID: chat.ID, throttle: editThrottleGroup}
}

// start 无操作：群聊的消息由首个增量直接创建，等待期的 typing 指示由调用方发送。
func (s *editReply) start() {}

// push 追加一个文本增量：首块立即建消息，其余距上次刷新超过节流间隔才编辑。
func (s *editReply) push(chunk string) {
	if chunk == "" {
		return
	}
	s.pending.WriteString(chunk)
	if s.messageID == 0 || time.Since(s.last) >= s.throttle {
		s.flush()
	}
}

// finish 定稿：把剩余增量刷完；曾刷新失败时把未送达部分另起消息兜底；全程没有
// 任何产出时补占位文案。
func (s *editReply) finish() {
	s.flush()
	if s.pending.Len() > 0 {
		// 刷新失败（网络抖动、消息被删等）：已可见的内容留在原消息，剩余另起一条。
		s.messageID, s.sent = 0, ""
		s.flush()
	}
	if s.messageID == 0 && s.sent == "" && s.pending.Len() == 0 {
		if err := s.sendNew(replyEmptyText); err != nil {
			logging.Warnf("tg[%s]: send empty reply to chat %d: %v", s.deviceID, s.chatID, err)
		}
	}
}

// flush 把 pending 刷成可见内容：能放进当前消息就编辑，超出上限就先按上限切出
// 一段定稿当前消息，剩余内容另起一条继续。失败时保留状态，等下一次触发重试。
func (s *editReply) flush() {
	for s.pending.Len() > 0 {
		content := s.sent + s.pending.String()
		if units(content) > maxMessageUnits {
			head, tail := splitToUnits(content, maxMessageUnits)
			if err := s.sendOrEdit(head); err != nil {
				logging.Warnf("tg[%s]: stream reply to chat %d: %v", s.deviceID, s.chatID, err)
				return
			}
			s.messageID, s.sent = 0, "" // 当前消息已定稿，剩余内容进下一条
			s.pending.Reset()
			s.pending.WriteString(tail)
			continue
		}
		if err := s.sendOrEdit(content); err != nil {
			logging.Warnf("tg[%s]: stream reply to chat %d: %v", s.deviceID, s.chatID, err)
			return
		}
		s.sent = content
		s.pending.Reset()
	}
}

// sendOrEdit 发送或编辑当前消息。内容与已可见内容相同则跳过：Telegram 会以
// "message is not modified" 拒绝这类编辑。
func (s *editReply) sendOrEdit(content string) error {
	if s.messageID == 0 {
		return s.sendNew(content)
	}
	if content == s.sent {
		return nil
	}
	if _, err := s.bot.Send(tgbotapi.NewEditMessageText(s.chatID, s.messageID, content)); err != nil {
		return err
	}
	s.last = time.Now()
	return nil
}

// sendNew 发一条新消息，并把它设为当前消息。
func (s *editReply) sendNew(content string) error {
	msg, err := s.bot.Send(tgbotapi.NewMessage(s.chatID, content))
	if err != nil {
		return err
	}
	s.messageID = msg.MessageID
	s.last = time.Now()
	return nil
}

// units 返回 Telegram 口径的字符数：UTF-16 码元（补充平面字符记 2）。
func units(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// truncateToUnits 把 s 截到不超过 max 个 UTF-16 码元，不劈开 rune。
func truncateToUnits(s string, max int) string {
	n := 0
	for i, r := range s {
		size := utf16.RuneLen(r)
		if n+size > max {
			return s[:i]
		}
		n += size
	}
	return s
}

// splitToUnits 在不超过 max 个 UTF-16 码元处把 s 切成两段，不劈开 rune。
func splitToUnits(s string, max int) (head, tail string) {
	head = truncateToUnits(s, max)
	return head, s[len(head):]
}
