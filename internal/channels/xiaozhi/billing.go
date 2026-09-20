package xiaozhi

import (
	"context"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// connectionBilling 是一次连接的计费接线：把管道里的原始事实映射成计费项，维护
// turn 边界 flush，并在本地预算耗尽时告诉通道层中断会话。
//
// 这一层是计费词汇与 audio / agent 的分界线：那两个包只吐事实（多少 rune、多少秒、
// 多少 token），item_code 的对应关系在这里（docs/billing-design.md §6.2 / §13）。
type connectionBilling struct {
	sess channels.BillingSession

	// turnSeq / turnOpen 维护会话内的 turn 序号：一个 turn 从用户说话（或文本注入）
	// 开始，到 agent 吐出 Finished 结束。序号只用于上报的分组维度与排查。
	turnSeq  int64
	turnOpen bool
}

const (
	// flushTimeout 是单次 turn 边界 flush 的时间上限。
	flushTimeout = 5 * time.Second
	// settleTimeout 是会话结束时交账的时间上限。它挡在连接释放前面，不能无限等。
	settleTimeout = 10 * time.Second
)

func newConnectionBilling(sess channels.BillingSession) *connectionBilling {
	if sess == nil {
		return nil
	}
	return &connectionBilling{sess: sess}
}

// start 启动计费句柄（兜底 flush）。
func (b *connectionBilling) start(ctx context.Context) {
	if b == nil {
		return
	}
	b.sess.Start(ctx)
}

// nearLimit 表示估算已经到预冻结的 90%：不再接受新的 listen 窗口，当前这个 turn
// 让它走完。
func (b *connectionBilling) nearLimit() bool {
	if b == nil {
		return false
	}
	return b.sess.NearLimit()
}

// observe 处理一条管道输出消息：映射事实、在 turn 边界 flush、检查本地预算。
// 返回 true 表示估算已经到 100%，调用方应当中断会话（§15.4）。
func (b *connectionBilling) observe(msg pipeline.Message) bool {
	if b == nil {
		return false
	}

	// 1. 原始事实 → 计费项。ASR 时长与 LLM 用量都挂在消息元数据上。
	if seconds, ok := audio.ASRSecondsFromMetadata(msg.Metadata); ok {
		b.openTurn() // 用户说话 = 新一轮
		channels.RecordASRSeconds(b.sess, seconds, nil)
	}
	if usage, ok := agent.UsageFromMetadata(msg.Metadata); ok {
		b.openTurn()
		channels.RecordLLMUsage(b.sess, usage, nil)
	}

	// 2. turn 边界：agent 说完这一轮就把缓冲交出去。
	if msg.Type == pipeline.MessageTypeFinished {
		b.closeTurn()
	}

	// 3. 本地熔断只拦后续 turn：已经发生的用量照常计费。
	return b.sess.Exhausted()
}

// openTurn 在需要时开启新的 turn 序号。
func (b *connectionBilling) openTurn() {
	if b.turnOpen {
		return
	}
	b.turnSeq++
	b.turnOpen = true
	b.sess.SetTurn(b.turnSeq)
}

// closeTurn 在 turn 边界 flush。失败不需要在这里处理：Sink 自己会重试并把缓冲留着，
// 会话结束时的 settle 还会再 flush 一次。
func (b *connectionBilling) closeTurn() {
	if !b.turnOpen {
		return
	}
	b.turnOpen = false
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	if err := b.sess.Flush(ctx); err != nil {
		logging.Warnf("xiaozhi-channel: billing flush at turn %d failed (buffer kept): %v", b.turnSeq, err)
	}
}

// close 交账：先 flush 再 settle。重试耗尽的极端情况由控制面的回收任务兜底（§15.1）。
func (b *connectionBilling) close(reason string) {
	if b == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), settleTimeout)
	defer cancel()
	resp, err := b.sess.Settle(ctx, reason)
	if err != nil {
		logging.Errorf("xiaozhi-channel: billing settle failed (reason=%s): %v", reason, err)
		return
	}
	if resp.Settled {
		logging.Infof("xiaozhi-channel: billing settled (reason=%s, charged=%d, released=%d, balance=%d)",
			reason, resp.ChargedMicro, resp.ReleasedMicro, resp.BalanceMicro)
	}
}
