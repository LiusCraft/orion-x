package service

import (
	"context"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

// 结算 worker 与预冻结回收（§16.3 / §16.4）。
//
// worker 就是 manager 进程里的两个 goroutine，多实例部署靠 SKIP LOCKED 天然分片，
// 不需要额外的协调。

// maxSettleBackoff 是连续失败后的退避上限。
const maxSettleBackoff = 30 * time.Second

// RunWorker 阻塞运行结算 worker 与预冻结回收，直到 ctx 结束。
// 退出时等在途的那批跑完：被截断的事务只会回滚，事件还是 pending，下次启动会重放，
// 所以不会丢账。
func (s *Service) RunWorker(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.runSettleLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		s.runReapLoop(ctx)
	}()
	wg.Wait()
}

func (s *Service) runSettleLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.WorkerTick)
	defer ticker.Stop()

	var backoff time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
		processed, err := s.RunOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logging.Errorf("billing: settle worker: %v", err)
			if backoff == 0 {
				backoff = s.cfg.WorkerTick
			} else if backoff < maxSettleBackoff {
				backoff *= 2
			}
			continue
		}
		backoff = 0
		if processed > 0 {
			logging.Infof("billing: settled %d event(s)", processed)
		}
	}
}

func (s *Service) runReapLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if _, err := s.ReapExpiredReservations(ctx); err != nil && ctx.Err() == nil {
			logging.Errorf("billing: reap reservations: %v", err)
		}
		if _, err := s.ExpireGrants(ctx); err != nil && ctx.Err() == nil {
			logging.Errorf("billing: expire grants: %v", err)
		}
	}
}

// RunOnce 认领并结算一批事件，返回处理的条数。worker 与测试共用。
//
// 认领（SKIP LOCKED）和结算分两个事务：认领那一步只是把行读出来、顺带排他锁一下，
// 这样某个账户的事件出问题时只回滚这个账户（每个账户一个事务），不会把整批变成
// 毒批次。重复认领由账户行锁和流水幂等键兜住，不会重复扣钱。
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.WorkerBatchTimeout)
	defer cancel()

	events, err := s.store.ClaimPendingEvents(nil, s.cfg.WorkerBatch)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	if err := s.settleEvents(ctx, events); err != nil {
		return 0, err
	}
	return len(events), nil
}
