package tg

import (
	"strings"
	"time"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// streamThrottlePrivate / streamThrottleGroup 是流式刷新的最小间隔。Telegram
	// 限制单聊约 1 条/秒、群聊 20 条/分钟（Bot FAQ），各自留出余量。
	streamThrottlePrivate = 2 * time.Second
	streamThrottleGroup   = 4 * time.Second

	// maxMessageUnits 是单条消息的字符上限（Bot API 限制 4096，按 UTF-16 码元计）。
	maxMessageUnits = 4096

	// replyEmptyText 是 Agent 没有任何产出时的占位回复。
	replyEmptyText = "（无响应）"
)

// streamReply 把 Agent 的流式文本增量刷进 Telegram：首个增量创建消息，之后按
// 节流编辑同一条；超过单条上限时定稿当前消息、另起一条继续。只由处理该消息的
// 单个 goroutine 使用，不是并发安全的。
type streamReply struct {
	bot      *tgbotapi.BotAPI
	deviceID string
	chatID   int64
	throttle time.Duration

	messageID int             // 当前在编辑的消息；0 = 还没成功发出过
	sent      string          // 当前消息里已经可见的内容
	pending   strings.Builder // 已产生、尚未刷新的增量
	last      time.Time       // 上次成功发送/编辑的时间，节流用
}

// newStreamReply 按聊天类型选择节流间隔：群聊的发送配额更小。
func newStreamReply(bot *tgbotapi.BotAPI, deviceID string, chat *tgbotapi.Chat) *streamReply {
	throttle := streamThrottlePrivate
	if chat.IsGroup() || chat.IsSuperGroup() {
		throttle = streamThrottleGroup
	}
	return &streamReply{bot: bot, deviceID: deviceID, chatID: chat.ID, throttle: throttle}
}

// push 追加一个文本增量：首块立即建消息，其余距上次刷新超过节流间隔才编辑。
func (s *streamReply) push(chunk string) {
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
func (s *streamReply) finish() {
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
func (s *streamReply) flush() {
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
func (s *streamReply) sendOrEdit(content string) error {
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
func (s *streamReply) sendNew(content string) error {
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

// splitToUnits 在不超过 max 个 UTF-16 码元处把 s 切成两段，不劈开 rune。
func splitToUnits(s string, max int) (head, tail string) {
	n := 0
	for i, r := range s {
		size := utf16.RuneLen(r)
		if n+size > max {
			return s[:i], s[i:]
		}
		n += size
	}
	return s, ""
}
