package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GALIAIS/NodeHarvest/internal/db"
)

type Runner func(context.Context, *db.QueuedTask) error

type Options struct {
	Workers   int
	Lease     time.Duration
	Poll      time.Duration
	RetryBase time.Duration
	WorkerID  string
}

// Queue consumes durable leased tasks. Multiple processes can safely share one database.
type Queue struct {
	db     *db.Store
	runner Runner
	opt    Options
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	active atomic.Int64
	start  sync.Once
	stop   sync.Once
}

func New(store *db.Store, runner Runner, opt Options) (*Queue, error) {
	if store == nil {
		return nil, fmt.Errorf("database is required")
	}
	if runner == nil {
		return nil, fmt.Errorf("runner is required")
	}
	if opt.Workers <= 0 {
		opt.Workers = 1
	}
	if opt.Workers > 64 {
		return nil, fmt.Errorf("workers must not exceed 64")
	}
	if opt.Lease <= 0 {
		opt.Lease = 2 * time.Minute
	}
	if opt.Poll <= 0 {
		opt.Poll = 500 * time.Millisecond
	}
	if opt.RetryBase <= 0 {
		opt.RetryBase = 5 * time.Second
	}
	if opt.WorkerID == "" {
		opt.WorkerID = defaultWorkerID()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Queue{db: store, runner: runner, opt: opt, ctx: ctx, cancel: cancel}, nil
}

func (q *Queue) Start() {
	q.start.Do(func() {
		for i := 0; i < q.opt.Workers; i++ {
			q.wg.Add(1)
			go q.loop(fmt.Sprintf("%s-%d", q.opt.WorkerID, i+1))
		}
	})
}

func (q *Queue) Stop() {
	q.stop.Do(func() {
		q.cancel()
		q.wg.Wait()
	})
}

func (q *Queue) loop(workerID string) {
	defer q.wg.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-timer.C:
		}
		task, err := q.db.LeaseTask(q.ctx, workerID, q.opt.Lease)
		if errors.Is(err, sql.ErrNoRows) {
			timer.Reset(q.opt.Poll)
			continue
		}
		if err != nil {
			if q.ctx.Err() != nil {
				return
			}
			slog.Error("lease queue task", "worker", workerID, "err", err)
			timer.Reset(q.opt.Poll)
			continue
		}
		q.run(workerID, task)
		timer.Reset(0)
	}
}

func (q *Queue) run(workerID string, task *db.QueuedTask) {
	q.active.Add(1)
	defer q.active.Add(-1)
	ctx, cancel := context.WithCancel(q.ctx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go q.heartbeat(ctx, cancel, heartbeatDone, workerID, task.ID)
	err := q.runner(ctx, task)
	cancel()
	<-heartbeatDone
	if err == nil {
		if completeErr := q.db.CompleteTask(task.ID, workerID); completeErr != nil {
			slog.Error("complete queue task", "task", task.ID, "worker", workerID, "err", completeErr)
		}
		return
	}
	if failErr := q.db.FailTask(task.ID, workerID, err.Error(), q.opt.RetryBase); failErr != nil {
		slog.Error("fail queue task", "task", task.ID, "worker", workerID, "err", failErr)
	}
}

func (q *Queue) heartbeat(ctx context.Context, cancel context.CancelFunc, done chan<- struct{}, workerID, taskID string) {
	defer close(done)
	interval := q.opt.Lease / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := q.db.RenewTaskLease(taskID, workerID, q.opt.Lease)
			if err != nil {
				slog.Error("renew task lease", "task", taskID, "worker", workerID, "err", err)
				continue
			}
			if !ok {
				cancel()
				return
			}
		}
	}
}

func (q *Queue) Status() map[string]any {
	stats, err := q.db.QueueStats()
	out := map[string]any{
		"workers": q.opt.Workers,
		"active":  q.active.Load(),
	}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["tasks"] = stats
	}
	return out
}

func defaultWorkerID() string {
	return fmt.Sprintf("worker-%d", time.Now().UnixNano())
}
