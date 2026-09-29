package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	// dialTimeout 是 WS 建连（含 TLS 握手）超时。
	dialTimeout = 10 * time.Second
	// subscribeTimeout 是等待 aibot_subscribe 回执的超时。
	subscribeTimeout = 15 * time.Second
	// writeTimeout 是单帧写超时。
	writeTimeout = 10 * time.Second
	// healthySession 之后重置重连退避，避免长期正常连接后偶发断线仍从 1s 起步。
	healthySession = time.Minute
)

// 默认时序；Client 上的同名字段可覆盖，测试用。
const (
	// ackTimeout 是回复帧等待回执的超时。
	ackTimeout = 5 * time.Second

	defaultHeartbeatEvery   = 30 * time.Second // 协议建议的心跳间隔
	defaultHeartbeatTimeout = 10 * time.Second
	defaultMinBackoff       = time.Second
	defaultMaxBackoff       = 30 * time.Second
	// defaultKickedBackoff 是被新连接踢掉后的重连间隔：立刻重连会与对方互踢。
	defaultKickedBackoff = 60 * time.Second
)

// frameHandler 消费一帧回调；实现方自行决定是否另起 goroutine。
type frameHandler func(*frame)

// ClientOptions 构造长连接客户端。
type ClientOptions struct {
	URL       string // 长连接地址；空则用 defaultWSURL
	BotID     string
	Secret    string
	Label     string // 日志前缀，通常是设备 ID
	OnMessage frameHandler
	OnEvent   frameHandler
}

// Client 是一个机器人的长连接客户端：Run 内部完成订阅、心跳与断线重连，
// 回复通过 Reply 复用回调帧的 req_id 发回同一条连接。
type Client struct {
	wsURL   string
	botID   string
	secret  string
	label   string
	onMsg   frameHandler
	onEvent frameHandler

	heartbeatEvery   time.Duration
	heartbeatTimeout time.Duration
	minBackoff       time.Duration
	maxBackoff       time.Duration
	kickedBackoff    time.Duration

	mu   sync.Mutex
	conn *conn

	kicked atomic.Bool
}

// NewClient 构造客户端；URL 为空时使用官方地址。
func NewClient(opts ClientOptions) *Client {
	wsURL := opts.URL
	if wsURL == "" {
		wsURL = defaultWSURL
	}
	return &Client{
		wsURL:            wsURL,
		botID:            opts.BotID,
		secret:           opts.Secret,
		label:            opts.Label,
		onMsg:            opts.OnMessage,
		onEvent:          opts.OnEvent,
		heartbeatEvery:   defaultHeartbeatEvery,
		heartbeatTimeout: defaultHeartbeatTimeout,
		minBackoff:       defaultMinBackoff,
		maxBackoff:       defaultMaxBackoff,
		kickedBackoff:    defaultKickedBackoff,
	}
}

// Run 阻塞地把连接维持在可用状态：建连 → 订阅 → 心跳保活，任何一步失败都按
// 指数退避（1s→2s→…→30s）重连，被新连接踢掉后退避 60s。ctx 取消即返回。
func (c *Client) Run(ctx context.Context) {
	delay := c.minBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		started := time.Now()
		err := c.serve(ctx)
		if ctx.Err() != nil {
			return
		}

		next := min(delay*2, c.maxBackoff)
		if time.Since(started) >= healthySession {
			next = c.minBackoff
		}
		if c.kicked.Swap(false) {
			next = c.kickedBackoff
		}
		logging.Warnf("wecom[%s]: connection lost (%v); reconnecting in %s", c.label, err, next)

		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
		delay = next
	}
}

// Reply 发送一帧回复，复用回调帧的 req_id；同一 req_id 的回复串行发送并等待
// 回执，nil 表示开放平台已受理。未连接或等待回执超时返回 error。
func (c *Client) Reply(ctx context.Context, reqID, cmd string, body any, timeout time.Duration) error {
	c.mu.Lock()
	cn := c.conn
	c.mu.Unlock()
	if cn == nil {
		return errors.New("wecom: not connected")
	}
	return cn.reply(ctx, reqID, cmd, body, timeout)
}

// noteKicked 记录「被新连接踢下线」，影响下一次重连的退避。
func (c *Client) noteKicked() { c.kicked.Store(true) }

func (c *Client) setConn(cn *conn) {
	c.mu.Lock()
	c.conn = cn
	c.mu.Unlock()
}

// serve 跑一轮完整连接，返回即代表这一轮连接结束。
func (c *Client) serve(ctx context.Context) error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = dialTimeout

	ws, _, err := dialer.DialContext(ctx, c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.wsURL, err)
	}

	cn := newConn(ws, c)
	c.setConn(cn)

	// 读循环必须是唯一读方，而且从建连起就跑：订阅回执、心跳回执、回调帧
	// 都靠它分发。先读后订阅的顺序不能反。
	readErr := make(chan error, 1)
	go func() { readErr <- cn.readLoop(ctx) }()

	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		// ctx 取消时关连接，把阻塞在 ReadMessage 上的读循环放出来。
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-stopWatch:
		}
	}()

	defer func() {
		c.setConn(nil)
		_ = ws.Close()
	}()

	if err := cn.request(ctx, cmdSubscribe, subscribeBody{BotID: c.botID, Secret: c.secret}, subscribeTimeout); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	logging.Infof("wecom[%s]: subscribed (bot_id=%s)", c.label, c.botID)

	hbCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go cn.heartbeat(hbCtx)

	return <-readErr
}

// conn 是一轮连接：读写、回执关联、按 req_id 串行回复都挂在这里。
type conn struct {
	ws     *websocket.Conn
	client *Client

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan frame

	repliesMu sync.Mutex
	replies   map[string]*replyLock
}

// replyLock 保证同一 req_id 的回复串行；refs 归零后从 map 移除，避免长连接上
// 每来一条回调就留一个锁。
type replyLock struct {
	mu   sync.Mutex
	refs int
}

func newConn(ws *websocket.Conn, client *Client) *conn {
	return &conn{
		ws:      ws,
		client:  client,
		pending: make(map[string]chan frame),
		replies: make(map[string]*replyLock),
	}
}

func (cn *conn) readLoop(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, data, err := cn.ws.ReadMessage()
		if err != nil {
			return err
		}
		var f frame
		if err := json.Unmarshal(data, &f); err != nil {
			logging.Warnf("wecom[%s]: malformed frame: %v", cn.client.label, err)
			continue
		}
		cn.dispatch(&f)
	}
}

func (cn *conn) dispatch(f *frame) {
	if f.Cmd == "" || f.Cmd == "pong" {
		cn.resolve(f.Headers.ReqID, *f)
		return
	}
	switch f.Cmd {
	case cmdMsgCallback:
		if cn.client.onMsg != nil {
			cn.client.onMsg(f)
		}
	case cmdEventCallback:
		if cn.client.onEvent != nil {
			cn.client.onEvent(f)
		}
	default:
		// 未知命令：带 req_id 的当回执消解，避免调用方一直等到超时。
		cn.resolve(f.Headers.ReqID, *f)
		logging.Warnf("wecom[%s]: ignoring frame cmd=%q", cn.client.label, f.Cmd)
	}
}

// heartbeat 定期 ping；连续等不到回执就关连接，让 serve 返回并重连。
func (cn *conn) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(cn.client.heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cn.request(ctx, cmdPing, nil, cn.client.heartbeatTimeout); err != nil {
				if ctx.Err() != nil {
					return
				}
				logging.Warnf("wecom[%s]: heartbeat failed (%v); dropping connection", cn.client.label, err)
				_ = cn.ws.Close()
				return
			}
		}
	}
}

// request 用自生成的 req_id 发一帧并等待回执。非 0 errcode、超时、写失败都算失败。
func (cn *conn) request(ctx context.Context, cmd string, body any, timeout time.Duration) error {
	reqID := newReqID(cmd)
	ackCh := cn.registerPending(reqID)
	defer cn.clearPending(reqID)

	if err := cn.write(reqID, cmd, body); err != nil {
		return err
	}
	return cn.waitAck(ctx, cmd, ackCh, timeout)
}

// reply 透传回调的 req_id 发送回复；同一 req_id 串行，保证流式刷新按序到达。
func (cn *conn) reply(ctx context.Context, reqID, cmd string, body any, timeout time.Duration) error {
	if reqID == "" {
		return errors.New("wecom: reply without req_id")
	}
	lock := cn.acquireReplyLock(reqID)
	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		cn.releaseReplyLock(reqID, lock)
	}()

	ackCh := cn.registerPending(reqID)
	defer cn.clearPending(reqID)
	if err := cn.write(reqID, cmd, body); err != nil {
		return err
	}
	return cn.waitAck(ctx, cmd, ackCh, timeout)
}

func (cn *conn) write(reqID, cmd string, body any) error {
	payload, err := json.Marshal(outFrame{Cmd: cmd, Headers: frameHeaders{ReqID: reqID}, Body: body})
	if err != nil {
		return fmt.Errorf("marshal %s frame: %w", cmd, err)
	}
	cn.writeMu.Lock()
	defer cn.writeMu.Unlock()
	if err := cn.ws.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("write %s: %w", cmd, err)
	}
	if err := cn.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("write %s: %w", cmd, err)
	}
	return nil
}

func (cn *conn) waitAck(ctx context.Context, cmd string, ackCh <-chan frame, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case f := <-ackCh:
		if f.ErrCode != 0 {
			return fmt.Errorf("%s rejected: errcode=%d errmsg=%s", cmd, f.ErrCode, f.ErrMsg)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("%s: ack timeout after %s", cmd, timeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (cn *conn) registerPending(reqID string) <-chan frame {
	ch := make(chan frame, 1)
	cn.pendingMu.Lock()
	cn.pending[reqID] = ch
	cn.pendingMu.Unlock()
	return ch
}

func (cn *conn) clearPending(reqID string) {
	cn.pendingMu.Lock()
	delete(cn.pending, reqID)
	cn.pendingMu.Unlock()
}

func (cn *conn) resolve(reqID string, f frame) {
	if reqID == "" {
		return
	}
	cn.pendingMu.Lock()
	ch, ok := cn.pending[reqID]
	cn.pendingMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- f:
	default:
	}
}

// acquireReplyLock / releaseReplyLock 管理某个 req_id 的回复锁与引用计数。
func (cn *conn) acquireReplyLock(reqID string) *replyLock {
	cn.repliesMu.Lock()
	defer cn.repliesMu.Unlock()
	lock, ok := cn.replies[reqID]
	if !ok {
		lock = &replyLock{}
		cn.replies[reqID] = lock
	}
	lock.refs++
	return lock
}

func (cn *conn) releaseReplyLock(reqID string, lock *replyLock) {
	cn.repliesMu.Lock()
	defer cn.repliesMu.Unlock()
	lock.refs--
	if lock.refs == 0 && cn.replies[reqID] == lock {
		delete(cn.replies, reqID)
	}
}

func newReqID(cmd string) string {
	return cmd + ":" + uuid.NewString()
}
