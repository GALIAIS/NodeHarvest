package queue

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/local/node-hunter/internal/model"
)

// Task 队列任务
type Task struct {
	Type     string // fetch|quality|ai|geo|full
	Options  map[string]any
	Priority int // higher first
	Enqueued time.Time
}

// Runner 执行函数
type Runner func(ctx context.Context, typ string, opts map[string]any) (*model.Job, error)

// Queue 内存优先级队列 + 单 worker（可扩展多 worker）
// 解决「同时只能一个 job」时的排队体验：返回 queued job 占位由 service 处理；
// 这里提供串行执行与积压观测。
type Queue struct {
	mu      sync.Mutex
	pending []Task
	running bool
	runner  Runner
	stopCh  chan struct{}
	wg      sync.WaitGroup
	maxSize int
}

func New(runner Runner, maxSize int) *Queue {
	if maxSize <= 0 {
		maxSize = 32
	}
	q := &Queue{runner: runner, stopCh: make(chan struct{}), maxSize: maxSize}
	q.wg.Add(1)
	go q.loop()
	return q
}

func (q *Queue) Stop() {
	close(q.stopCh)
	q.wg.Wait()
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *Queue) Enqueue(typ string, opts map[string]any, priority int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) >= q.maxSize {
		return errFull
	}
	q.pending = append(q.pending, Task{Type: typ, Options: opts, Priority: priority, Enqueued: time.Now()})
	// sort by priority desc
	for i := 0; i < len(q.pending); i++ {
		for j := i + 1; j < len(q.pending); j++ {
			if q.pending[j].Priority > q.pending[i].Priority {
				q.pending[i], q.pending[j] = q.pending[j], q.pending[i]
			}
		}
	}
	return nil
}

var errFull = &QueueFullError{}

type QueueFullError struct{}

func (e *QueueFullError) Error() string { return "job queue is full" }

func (q *Queue) loop() {
	defer q.wg.Done()
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-q.stopCh:
			return
		case <-t.C:
			task, ok := q.pop()
			if !ok {
				continue
			}
			q.mu.Lock()
			q.running = true
			q.mu.Unlock()
			ctx := context.Background()
			if _, err := q.runner(ctx, task.Type, task.Options); err != nil {
				slog.Warn("queue task", "type", task.Type, "err", err)
			}
			q.mu.Lock()
			q.running = false
			q.mu.Unlock()
		}
	}
}

func (q *Queue) pop() (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return Task{}, false
	}
	t := q.pending[0]
	q.pending = q.pending[1:]
	return t, true
}

func (q *Queue) Status() map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	return map[string]any{
		"pending":  len(q.pending),
		"running":  q.running,
		"max_size": q.maxSize,
	}
}
