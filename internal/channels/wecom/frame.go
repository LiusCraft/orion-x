// Package wecom implements the WeCom (企业微信) smart-robot channel over the
// open-platform WebSocket long connection: one client per device that has
// bot credentials, subscribed to aibot_msg_callback / aibot_event_callback.
package wecom

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// defaultWSURL 是企业微信开放平台长连接地址。
const defaultWSURL = "wss://openws.work.weixin.qq.com"

// 开发者 → 企业微信 的命令。
const (
	cmdSubscribe      = "aibot_subscribe"           // 认证订阅
	cmdPing           = "ping"                      // 心跳
	cmdRespondMsg     = "aibot_respond_msg"         // 回复消息（含流式）
	cmdRespondWelcome = "aibot_respond_welcome_msg" // 回复欢迎语
)

// 企业微信 → 开发者 的命令。
const (
	cmdMsgCallback   = "aibot_msg_callback"   // 消息回调
	cmdEventCallback = "aibot_event_callback" // 事件回调
)

// 消息类型。
const (
	msgTypeText   = "text"
	msgTypeVoice  = "voice"  // 语音消息，协议已转成文本
	msgTypeMixed  = "mixed"  // 图文混排，只取其中的文本项
	msgTypeStream = "stream" // 流式回复
	msgTypeImage  = "image"
	msgTypeFile   = "file"
	msgTypeVideo  = "video"
)

// 事件类型。
const (
	eventEnterChat    = "enter_chat"         // 用户当天首次进入机器人单聊会话
	eventDisconnected = "disconnected_event" // 新连接建立，本连接被踢
)

// 会话类型。
const chatTypeGroup = "group"

// frame 是长连接上收到的一帧。回调帧带 Cmd；回执帧只有 Headers + errcode/errmsg。
type frame struct {
	Cmd     string          `json:"cmd,omitempty"`
	Headers frameHeaders    `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

// outFrame 是写出的帧，Body 允许任意可序列化值。
type outFrame struct {
	Cmd     string       `json:"cmd"`
	Headers frameHeaders `json:"headers"`
	Body    any          `json:"body,omitempty"`
}

type frameHeaders struct {
	ReqID string `json:"req_id"`
}

type subscribeBody struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

// callbackBody 是消息回调与事件回调的 body，按 msgtype 取用其中一段。
type callbackBody struct {
	MsgID    string        `json:"msgid"`
	AibotID  string        `json:"aibotid"`
	ChatID   string        `json:"chatid"`
	ChatType string        `json:"chattype"`
	From     callbackFrom  `json:"from"`
	MsgType  string        `json:"msgtype"`
	Text     *textContent  `json:"text"`
	Voice    *textContent  `json:"voice"`
	Mixed    *mixedContent `json:"mixed"`
	Event    *eventContent `json:"event"`
}

type callbackFrom struct {
	UserID string `json:"userid"`
}

type textContent struct {
	Content string `json:"content"`
}

type mixedContent struct {
	MsgItem []mixedItem `json:"msg_item"`
}

type mixedItem struct {
	MsgType string       `json:"msgtype"`
	Text    *textContent `json:"text"`
}

type eventContent struct {
	EventType string `json:"eventtype"`
}

// streamBody 是流式回复（aibot_respond_msg + msgtype=stream）。
type streamBody struct {
	MsgType string        `json:"msgtype"`
	Stream  streamContent `json:"stream"`
}

type streamContent struct {
	ID      string `json:"id"`
	Finish  bool   `json:"finish"`
	Content string `json:"content"`
}

// textBody 是文本回复（欢迎语等）。
type textBody struct {
	MsgType string      `json:"msgtype"`
	Text    textContent `json:"text"`
}

// text 返回消息里的用户文本；ok=false 表示该消息类型本次不支持。
// 语音消息协议已转写为文本，图文混排只取文本项。
func (b *callbackBody) text() (string, bool) {
	switch b.MsgType {
	case msgTypeText:
		if b.Text == nil {
			return "", true
		}
		return b.Text.Content, true
	case msgTypeVoice:
		if b.Voice == nil {
			return "", true
		}
		return b.Voice.Content, true
	case msgTypeMixed:
		var parts []string
		if b.Mixed != nil {
			for _, item := range b.Mixed.MsgItem {
				if item.MsgType != msgTypeText || item.Text == nil {
					continue
				}
				if strings.TrimSpace(item.Text.Content) == "" {
					continue
				}
				parts = append(parts, item.Text.Content)
			}
		}
		return strings.Join(parts, "\n"), true
	default:
		return "", false
	}
}

// conversationID 返回会话标识：群聊用 chatid，单聊用发送者 userid。
func (b *callbackBody) conversationID() string {
	if b.ChatType == chatTypeGroup && b.ChatID != "" {
		return b.ChatID
	}
	return b.From.UserID
}

// conversationKey 是会话状态在进程内的键，形如 wecom:<device>:<会话 ID>。
func conversationKey(deviceID string, body *callbackBody) string {
	return "wecom:" + deviceID + ":" + body.conversationID()
}

// stripMention 去掉群聊消息开头的 @机器人名。
func stripMention(s string) string {
	if !strings.HasPrefix(s, "@") {
		return s
	}
	i := strings.IndexFunc(s, isSpaceRune)
	if i <= 0 {
		return s
	}
	return strings.TrimSpace(s[i:])
}

func isSpaceRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\u00a0', '\u2005', '\u3000':
		return true
	}
	return false
}

// truncateUTF8 把 s 截到不超过 maxBytes 字节，且不切断 UTF-8 字符。
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
