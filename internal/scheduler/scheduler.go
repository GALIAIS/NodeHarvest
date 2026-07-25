package scheduler

import (
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/local/node-hunter/internal/config"
	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/service"
	"github.com/local/node-hunter/internal/timex"
)

// Scheduler 周期性触发采集/测速/全流程
type Scheduler struct {
	cfg *config.Config
	svc *service.Service

	mu       sync.Mutex
	stopCh   chan struct{}
	running  bool
	lastRun  *time.Time
	lastJob  *model.Job
	lastErr  string
	nextRun  *time.Time
	runCount int
}

func New(cfg *config.Config, svc *service.Service) *Scheduler {
	return &Scheduler{cfg: cfg, svc: svc}
}

// Start 后台启动；cfg.Schedule.Enabled=false 时 no-op
func (s *Scheduler) Start() {
	if s.cfg == nil || !s.cfg.Schedule.Enabled {
		slog.Info("scheduler disabled")
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	interval := s.cfg.ScheduleInterval()
	slog.Info("scheduler started",
		"interval_min", s.cfg.Schedule.IntervalMin,
		"job", s.cfg.Schedule.Job,
		"run_on_start", s.cfg.Schedule.RunOnStart,
		"skip_ai", s.cfg.Schedule.SkipAI,
	)

	go s.loop(interval)
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stopCh)
	s.running = false
}

func (s *Scheduler) loop(interval time.Duration) {
	// 启动抖动
	jitter := time.Duration(s.cfg.Schedule.JitterSec) * time.Second
	if jitter > 0 {
		d := time.Duration(rand.Int63n(int64(jitter) + 1))
		s.setNext(time.Now().Add(d))
		select {
		case <-s.stopCh:
			return
		case <-time.After(d):
		}
	}

	if s.cfg.Schedule.RunOnStart {
		s.fire("startup")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.setNext(time.Now().Add(interval))

	for {
		select {
		case <-s.stopCh:
			return
		case t := <-ticker.C:
			s.fire("interval")
			s.setNext(t.Add(interval))
		}
	}
}

func (s *Scheduler) setNext(t time.Time) {
	s.mu.Lock()
	s.nextRun = &t
	s.mu.Unlock()
}

func (s *Scheduler) fire(reason string) {
	opts := map[string]any{
		"max_test":  float64(s.cfg.Schedule.MaxTest),
		"rounds":    float64(s.cfg.Schedule.Rounds),
		"scheduled": true,
		"reason":    reason,
	}
	if s.cfg.Schedule.SkipAI {
		opts["skip_ai"] = true
	}

	jobType := s.cfg.Schedule.Job
	var (
		j   *model.Job
		err error
	)
	switch jobType {
	case "fetch":
		j, err = s.svc.StartFetch(opts)
	case "quality":
		j, err = s.svc.StartQuality(opts)
	default:
		j, err = s.svc.StartFull(opts)
		jobType = "full"
	}

	now := time.Now()
	s.mu.Lock()
	s.lastRun = &now
	s.runCount++
	if err != nil {
		s.lastErr = err.Error()
		s.lastJob = nil
		s.mu.Unlock()
		slog.Warn("scheduler skip", "reason", reason, "job", jobType, "err", err)
		return
	}
	s.lastErr = ""
	s.lastJob = j
	s.mu.Unlock()
	slog.Info("scheduler fired", "reason", reason, "job", jobType, "id", j.ID)
}

// Status 供 API 展示
func (s *Scheduler) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{
		"enabled":      s.cfg != nil && s.cfg.Schedule.Enabled,
		"running":      s.running,
		"interval_min": 0,
		"job":          "",
		"run_count":    s.runCount,
		"last_error":   s.lastErr,
	}
	if s.cfg != nil {
		out["interval_min"] = s.cfg.Schedule.IntervalMin
		out["job"] = s.cfg.Schedule.Job
		out["run_on_start"] = s.cfg.Schedule.RunOnStart
		out["skip_ai"] = s.cfg.Schedule.SkipAI
		out["max_test"] = s.cfg.Schedule.MaxTest
	}
	if s.lastRun != nil {
		out["last_run_at"] = timex.FormatRFC3339(*s.lastRun)
	}
	if s.nextRun != nil {
		out["next_run_at"] = timex.FormatRFC3339(*s.nextRun)
	}
	out["tz"] = "Asia/Shanghai"
	if s.lastJob != nil {
		out["last_job"] = map[string]any{
			"id":       s.lastJob.ID,
			"type":     s.lastJob.Type,
			"status":   s.lastJob.Status,
			"progress": s.lastJob.Progress,
			"message":  s.lastJob.Message,
		}
	}
	return out
}
