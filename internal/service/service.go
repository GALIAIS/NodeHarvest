package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/GALIAIS/NodeHarvest/internal/aiaccess"
	"github.com/GALIAIS/NodeHarvest/internal/cleaner"
	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/dialer"
	"github.com/GALIAIS/NodeHarvest/internal/exporter"
	"github.com/GALIAIS/NodeHarvest/internal/fetcher"
	"github.com/GALIAIS/NodeHarvest/internal/filter"
	"github.com/GALIAIS/NodeHarvest/internal/geo"
	"github.com/GALIAIS/NodeHarvest/internal/hotcache"
	"github.com/GALIAIS/NodeHarvest/internal/metrics"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/objectstore"
	"github.com/GALIAIS/NodeHarvest/internal/parser"
	"github.com/GALIAIS/NodeHarvest/internal/publish"
	"github.com/GALIAIS/NodeHarvest/internal/purity"
	"github.com/GALIAIS/NodeHarvest/internal/quality"
	"github.com/GALIAIS/NodeHarvest/internal/store"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
)

// Service 业务编排
type Service struct {
	cfg        *config.Config
	runtimeCfg atomic.Pointer[config.Config]
	store      *store.Store
	geo        *geo.Locator
	pub        *publish.Cache
	db         *db.Store
	metrics    *metrics.Registry
	hot        *hotcache.Client
	artifacts  *objectstore.Client
	mu         sync.Mutex
	jobMu      sync.Mutex
	pubMu      sync.Mutex
	sourceMu   sync.RWMutex
	sources    map[string]db.SourceState
	active     map[string]activeJob
	maxJobs    int
}

type activeJob struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Options struct {
	DB        *db.Store
	Metrics   *metrics.Registry
	Hot       *hotcache.Client
	Artifacts *objectstore.Client
}

func New(cfg *config.Config, st *store.Store) *Service {
	return NewWithOptions(cfg, st, Options{})
}

func NewWithOptions(cfg *config.Config, st *store.Store, opt Options) *Service {
	s := &Service{
		cfg: cfg, store: st, db: opt.DB, metrics: opt.Metrics, hot: opt.Hot, artifacts: opt.Artifacts,
		sources: make(map[string]db.SourceState),
		active:  make(map[string]activeJob),
		maxJobs: 1,
	}
	s.runtimeCfg.Store(cfg)
	if cfg != nil && cfg.Queue.EmbeddedWorkers > s.maxJobs {
		s.maxJobs = cfg.Queue.EmbeddedWorkers
	}
	if s.db != nil {
		if version, err := s.db.LatestConfigVersion(); err == nil {
			var patch config.RuntimePatch
			if json.Unmarshal([]byte(version.PatchJSON), &patch) == nil {
				if updated, applyErr := config.ApplyRuntimePatch(cfg, patch); applyErr == nil {
					s.runtimeCfg.Store(updated)
				} else {
					slog.Warn("apply persisted runtime config", "err", applyErr)
				}
			}
		}
		limit := 0
		if cfg != nil {
			limit = cfg.Filter.MaxStoreNodes
		}
		if nodes, err := s.db.LoadNodes(limit); err != nil {
			slog.Warn("load durable nodes", "err", err)
		} else if len(nodes) > 0 {
			if err := st.ReplaceNodes(nodes); err != nil {
				slog.Warn("restore durable nodes", "err", err)
			}
		}
		if states, err := s.db.ListSourceStates(); err == nil {
			for _, state := range states {
				s.sources[state.Name] = state
			}
		}
		if cfg != nil {
			for _, source := range cfg.Sources {
				state, ok := s.sources[source.Name]
				if !ok {
					state = db.SourceState{
						Name: source.Name, URL: source.URL, Priority: source.Priority, HealthScore: 100,
					}
					s.sources[source.Name] = state
				}
				if err := s.db.EnsureSource(state); err != nil {
					slog.Warn("persist source config", "source", source.Name, "err", err)
				}
			}
		}
	}
	if s.metrics == nil {
		s.metrics = metrics.New()
	}
	s.pub = publish.NewCache(cfg.Export.Dir)
	policy := s.publishPolicy()
	if blob := s.pub.Get(); blob != nil && !blob.MatchesPolicy(policy) {
		s.pub.Clear()
	}
	if s.hot != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if blob, err := s.hot.GetBlob(ctx); err == nil && blob.MatchesPolicy(policy) {
			if err := s.pub.Store(blob, false); err != nil {
				slog.Warn("load redis publish cache", "err", err)
			}
		}
		cancel()
	}
	if cfg != nil && cfg.Geo.Enabled {
		s.geo = geo.NewWithASN(cfg.Geo.DBPath, cfg.Geo.ASNDBPath)
		if err := s.geo.Open(); err != nil {
			slog.Info("geoip db not ready yet", "err", err)
		}
		if cfg.Geo.AutoDownload {
			go func() {
				if err := s.geo.EnsureDB(cfg.Geo.DownloadURL); err != nil {
					slog.Warn("geoip ensure failed, name heuristic only", "err", err)
				}
				if err := s.geo.EnsureASNDB(cfg.Geo.ASNDownloadURL); err != nil {
					slog.Warn("ASN database ensure failed", "err", err)
				}
			}()
		}
	}
	// 启动时若已有节点则刷新一次订阅缓存
	if cfg != nil && cfg.Publish.PreRender && st.Count() > 0 {
		go s.RefreshPublishCache()
	}
	return s
}

func (s *Service) Store() *store.Store { return s.store }
func (s *Service) Config() *config.Config {
	if cfg := s.runtimeCfg.Load(); cfg != nil {
		return cfg
	}
	return s.cfg
}
func (s *Service) Geo() *geo.Locator            { return s.geo }
func (s *Service) PublishCache() *publish.Cache { return s.pub }
func (s *Service) DB() *db.Store                { return s.db }
func (s *Service) Metrics() *metrics.Registry   { return s.metrics }
func (s *Service) HotCache() *hotcache.Client   { return s.hot }

func (s *Service) UpdateRuntimeConfig(actor string, patch config.RuntimePatch) (*db.ConfigVersion, error) {
	next, err := config.ApplyRuntimePatch(s.Config(), patch)
	if err != nil {
		return nil, err
	}
	effective := runtimePatchFromConfig(next)
	payload, err := json.Marshal(effective)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	version := &db.ConfigVersion{
		ID: newID(), Actor: actor, Checksum: hex.EncodeToString(sum[:]), PatchJSON: string(payload),
	}
	if s.db != nil {
		if err := s.db.SaveConfigVersion(*version); err != nil {
			return nil, err
		}
		if err := s.db.Audit(actor, "config.update", version.Checksum); err != nil {
			slog.Error("audit runtime config update", "err", err)
		}
	}
	s.runtimeCfg.Store(next)
	s.RefreshPublishCache()
	return version, nil
}

func runtimePatchFromConfig(cfg *config.Config) config.RuntimePatch {
	return config.RuntimePatch{
		PublishMinScore:           &cfg.Publish.MinScore,
		PublishMaxNodes:           &cfg.Publish.MaxNodes,
		PublishAliveOnly:          &cfg.Publish.AliveOnly,
		PublishCacheSec:           &cfg.Publish.CacheSec,
		PublishMaxNodeAgeHours:    &cfg.Publish.MaxNodeAgeHours,
		GovernanceDisableFailures: &cfg.Governance.DisableAfterFailures,
		GovernanceCooldownHours:   &cfg.Governance.CooldownHours,
		GovernanceHQDropPercent:   &cfg.Governance.HQDropPercent,
		GovernanceCountryShare:    &cfg.Governance.CountrySharePercent,
		DialAfterQuality:          &cfg.Dial.AfterQuality,
		DialAfterQualityMax:       &cfg.Dial.AfterQualityMax,
	}
}

func (s *Service) Job(id string) (*model.Job, bool) {
	if id == "" {
		return nil, false
	}
	if s.db != nil {
		job, err := s.db.GetJob(id)
		return job, err == nil
	}
	return s.store.GetJob(id)
}

func (s *Service) replaceNodes(nodes []*model.Node) error {
	if err := s.store.ReplaceNodes(nodes); err != nil {
		return err
	}
	if s.db != nil {
		if err := s.db.ReplaceNodes(nodes); err != nil {
			return fmt.Errorf("persist nodes: %w", err)
		}
	}
	return nil
}

func (s *Service) upsertNodes(nodes []*model.Node, recordMetrics bool) error {
	if err := s.store.UpsertNodes(nodes); err != nil {
		return err
	}
	if s.db != nil {
		if err := s.db.SaveNodes(nodes); err != nil {
			return fmt.Errorf("persist nodes: %w", err)
		}
		if recordMetrics {
			if err := s.db.RecordNodeMetrics(nodes); err != nil {
				return fmt.Errorf("persist node metrics: %w", err)
			}
		}
	}
	return nil
}

func (s *Service) SourceStates() map[string]db.SourceState {
	s.sourceMu.RLock()
	defer s.sourceMu.RUnlock()
	out := make(map[string]db.SourceState, len(s.sources))
	for name, state := range s.sources {
		out[name] = state
	}
	return out
}

func (s *Service) EnabledSources() []config.Source {
	cfg := s.Config()
	if cfg == nil {
		return nil
	}
	sources := cfg.EnabledSources()
	states := s.SourceStates()
	now := time.Now()
	out := sources[:0]
	for _, source := range sources {
		state := states[source.Name]
		if state.ManuallyDisabled {
			continue
		}
		if state.DisabledUntil != "" {
			if until, err := time.Parse(time.RFC3339, state.DisabledUntil); err == nil && until.After(now) {
				continue
			}
		}
		out = append(out, source)
	}
	return out
}

func (s *Service) SetSourceEnabled(name string, enabled bool, actor string) error {
	if s.db == nil {
		return fmt.Errorf("source mutation requires database")
	}
	found := false
	for _, source := range s.Config().Sources {
		if source.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("source %q not found", name)
	}
	if err := s.db.SetSourceEnabled(name, enabled); err != nil {
		return err
	}
	s.sourceMu.Lock()
	state := s.sources[name]
	state.ManuallyDisabled = !enabled
	if enabled {
		state.DisabledUntil = ""
	}
	s.sources[name] = state
	s.sourceMu.Unlock()
	return s.db.Audit(actor, "source.enabled", fmt.Sprintf("%s=%t", name, enabled))
}

func (s *Service) ProbeSource(ctx context.Context, name string) (db.SourceState, error) {
	var source *config.Source
	cfg := s.Config()
	for i := range cfg.Sources {
		if cfg.Sources[i].Name == name {
			source = &cfg.Sources[i]
			break
		}
	}
	if source == nil {
		return db.SourceState{}, fmt.Errorf("source %q not found", name)
	}
	doc := fetcher.New(cfg.FetchTimeout(), cfg.App.UserAgent).FetchOne(ctx, *source)
	err := doc.Err
	if err == nil && len(parser.ParseContent(doc.Body, source.Name)) == 0 {
		err = fmt.Errorf("no supported nodes parsed")
	}
	s.recordSource(doc, err)
	return s.SourceStates()[name], err
}

func (s *Service) recordSource(d fetcher.Document, err error) {
	now := timex.NowRFC3339()
	s.sourceMu.Lock()
	state := s.sources[d.Source.Name]
	state.Name = d.Source.Name
	state.URL = d.Source.URL
	state.Priority = d.Source.Priority
	state.LastAttemptAt = now
	state.FetchCount++
	state.LatencyMS = d.Latency.Milliseconds()
	state.Bytes = d.Bytes
	state.StatusCode = d.StatusCode
	state.Attempts = d.Attempts
	if err == nil {
		state.LastSuccessAt = now
		state.LastError = ""
		state.ConsecutiveFailures = 0
		state.DisabledUntil = ""
		state.SuccessCount++
	} else {
		state.LastError = err.Error()
		state.ConsecutiveFailures++
		cfg := s.Config()
		if cfg != nil && state.ConsecutiveFailures >= cfg.Governance.DisableAfterFailures {
			state.DisabledUntil = timex.FormatRFC3339(time.Now().Add(
				time.Duration(cfg.Governance.CooldownHours) * time.Hour,
			))
		}
	}
	state.HealthScore = sourceHealth(state)
	s.sources[d.Source.Name] = state
	s.sourceMu.Unlock()
	if s.db != nil {
		dbState := state
		dbState.LastError = ""
		if err != nil {
			dbState.LastError = err.Error()
		}
		if dbErr := s.db.RecordSourceFetch(dbState, err == nil); dbErr != nil {
			slog.Warn("persist source state", "source", d.Source.Name, "err", dbErr)
		}
	}
	if s.metrics != nil {
		s.metrics.ObserveSource(d.Source.Name, err == nil, d.Latency.Seconds(), d.Bytes)
	}
}

func sourceHealth(state db.SourceState) float64 {
	if state.FetchCount == 0 {
		return 100
	}
	success := float64(state.SuccessCount) / float64(state.FetchCount)
	contribution := 0.0
	if state.ContributionTotal > 0 {
		contribution = float64(state.ContributionHQ) / float64(state.ContributionTotal)
	}
	score := success*75 + contribution*25 - float64(state.ConsecutiveFailures)*5
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func (s *Service) updateSourceContributions(nodes []*model.Node) {
	total := make(map[string]int)
	hq := make(map[string]int)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		sources := node.Sources
		if len(sources) == 0 && node.Source != "" {
			sources = []string{node.Source}
		}
		for _, source := range sources {
			total[source]++
			if node.Alive && node.Score >= s.Config().Filter.MinScore {
				hq[source]++
			}
		}
	}
	s.sourceMu.Lock()
	for name, state := range s.sources {
		state.ContributionTotal = total[name]
		state.ContributionHQ = hq[name]
		state.HealthScore = sourceHealth(state)
		s.sources[name] = state
		if s.db != nil {
			if err := s.db.SetSourceContribution(name, state.ContributionTotal, state.ContributionHQ, state.HealthScore); err != nil {
				slog.Warn("persist source contribution", "source", name, "err", err)
			}
		}
	}
	s.sourceMu.Unlock()
}

func (s *Service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active) > 0
}

// Flush 持久化快照（优雅退出）
func (s *Service) Flush() error {
	err := s.store.Flush()
	s.RefreshPublishCache()
	return err
}

// RefreshPublishCache 预渲染高质量订阅
func (s *Service) RefreshPublishCache() *publish.Blob {
	s.pubMu.Lock()
	defer s.pubMu.Unlock()
	cfg := s.Config()
	if s.pub == nil || cfg == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	policy := s.publishPolicy()
	var lock *hotcache.Lock
	if s.hot != nil {
		if acquired, ok, err := s.hot.Acquire(ctx, "publish"); err != nil {
			slog.Warn("redis publish lock", "err", err)
		} else if !ok {
			if blob, getErr := s.hot.GetBlob(ctx); getErr == nil && blob.MatchesPolicy(policy) {
				_ = s.pub.Store(blob, false)
				return blob
			}
		} else {
			lock = acquired
			defer func() {
				if err := lock.Release(context.Background()); err != nil {
					slog.Warn("release redis publish lock", "err", err)
				}
			}()
		}
	}
	nodes := s.SelectPublishNodes()
	maxC := 30
	if cfg.Publish.MaxCountries > 0 {
		maxC = cfg.Publish.MaxCountries
	}
	blob := s.pub.Update(nodes, maxC, policy)
	if s.hot != nil {
		if err := s.hot.SetSnapshot(ctx, blob, nodes); err != nil {
			slog.Warn("persist redis publish cache", "err", err)
		}
	}
	if s.artifacts != nil {
		if err := s.artifacts.UploadSnapshot(ctx, blob); err != nil {
			slog.Warn("upload publish artifacts", "err", err)
		}
	}
	if s.metrics != nil {
		st := s.store.Stats(len(s.EnabledSources()))
		s.metrics.SetNodes(st.TotalNodes, st.AliveNodes, st.HighQuality)
		s.metrics.SetNodeDimensions(s.store.AllNodes())
	}
	return blob
}

func (s *Service) publishPolicy() string {
	cfg := s.Config()
	if cfg == nil {
		return ""
	}
	policy := struct {
		Version        int
		PublishEnabled bool
		MinScore       float64
		MaxNodes       int
		MaxNodeAge     int
		AliveOnly      bool
		MaxCountries   int
		Filter         config.FilterConfig
		DialEnabled    bool
		DialEngine     string
		VerifiedTTL    int
		RenameWithFlag bool
	}{
		Version: 2, PublishEnabled: cfg.Publish.Enabled,
		MinScore: cfg.Publish.MinScore, MaxNodes: cfg.Publish.MaxNodes,
		MaxNodeAge: cfg.Publish.MaxNodeAgeHours, AliveOnly: cfg.Publish.AliveOnly,
		MaxCountries: cfg.Publish.MaxCountries, Filter: cfg.Filter,
		DialEnabled: cfg.Dial.Enabled, DialEngine: cfg.Dial.Engine,
		VerifiedTTL: cfg.Dial.VerifiedTTLHours, RenameWithFlag: cfg.Geo.RenameWithFlag,
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("v2:%x", sum[:8])
}

func (s *Service) newJob(id, typ string, opts map[string]any) *model.Job {
	if id == "" {
		id = newID()
	}
	now := time.Now()
	publicOpts := make(map[string]any, len(opts))
	for key, value := range opts {
		if !strings.HasPrefix(key, "_") {
			publicOpts[key] = value
		}
	}
	actor, _ := opts["_actor"].(string)
	tenant, _ := opts["_tenant"].(string)
	if actor == "" {
		actor = "system"
	}
	if tenant == "" {
		tenant = "default"
	}
	j := &model.Job{
		ID:        id,
		Type:      typ,
		Status:    model.JobPending,
		Message:   "queued",
		CreatedAt: now,
		UpdatedAt: now,
		Options:   publicOpts,
		Stats:     map[string]any{},
		Actor:     actor,
		TenantID:  tenant,
	}
	s.store.SaveJob(j)
	return j
}

func (s *Service) updateJob(j *model.Job, status model.JobStatus, progress float64, msg string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	j.Status = status
	j.Progress = progress
	j.Message = msg
	j.UpdatedAt = time.Now()
	s.store.SaveJob(j)
	if s.db != nil {
		if err := s.db.SaveJob(j); err != nil {
			slog.Warn("persist job", "id", j.ID, "err", err)
		}
		if msg != "" && (int(progress)%25 == 0 || status == model.JobCompleted ||
			status == model.JobFailed || status == model.JobCanceled) {
			if err := s.db.AddJobEvent(j.ID, "info", msg); err != nil {
				slog.Warn("persist job event", "id", j.ID, "err", err)
			}
		}
	}
}

// StartFull 采集 + 质量 + AI 启发
func (s *Service) StartFull(opts map[string]any) (*model.Job, error) {
	return s.Submit("full", opts)
}

func (s *Service) StartFetch(opts map[string]any) (*model.Job, error) {
	return s.Submit("fetch", opts)
}

func (s *Service) StartQuality(opts map[string]any) (*model.Job, error) {
	return s.Submit("quality", opts)
}

func (s *Service) StartAI(opts map[string]any) (*model.Job, error) {
	return s.Submit("ai", opts)
}

// StartGeo 仅国家标注
func (s *Service) StartGeo(opts map[string]any) (*model.Job, error) {
	return s.Submit("geo", opts)
}

// StartDial 真实协议拨测（sing-box）
func (s *Service) StartDial(opts map[string]any) (*model.Job, error) {
	return s.Submit("dial", opts)
}

// StartPurity 对 verified 节点做 IP 纯净度 + Cloudflare 挑战启发式探测
func (s *Service) StartPurity(opts map[string]any) (*model.Job, error) {
	return s.Submit("purity", opts)
}

type runner func(ctx context.Context, j *model.Job)

func (s *Service) runnerFor(typ string) (runner, error) {
	switch typ {
	case "full":
		return s.runFull, nil
	case "fetch":
		return s.runFetch, nil
	case "quality":
		return s.runQuality, nil
	case "ai":
		return s.runAI, nil
	case "geo":
		return s.runGeo, nil
	case "dial":
		return s.runDial, nil
	case "purity":
		return s.runPurity, nil
	default:
		return nil, fmt.Errorf("unsupported job type %q", typ)
	}
}

// Submit queues a job when the durable queue is enabled, otherwise it starts immediately.
func (s *Service) Submit(typ string, opts map[string]any) (*model.Job, error) {
	fn, err := s.runnerFor(typ)
	if err != nil {
		return nil, err
	}
	if s.cfg != nil && s.cfg.Queue.Enabled {
		if s.db == nil {
			return nil, fmt.Errorf("durable queue requires database.enabled")
		}
		priority := 100
		if scheduled, _ := opts["scheduled"].(bool); scheduled {
			priority = 10
		}
		id := newID()
		job := s.newJob(id, typ, opts)
		if err := s.db.SaveJob(job); err != nil {
			return nil, err
		}
		task := &db.QueuedTask{
			ID:          id,
			Type:        typ,
			Options:     opts,
			Priority:    priority,
			MaxAttempts: s.cfg.Queue.MaxAttempts,
		}
		if err := s.db.EnqueueTask(task, s.cfg.Queue.MaxPending); err != nil {
			s.fail(job, err)
			return nil, err
		}
		s.updateJob(job, model.JobPending, 0, "queued")
		return job, nil
	}
	return s.startNow(context.Background(), "", typ, opts, fn, true)
}

// RunQueuedTask executes a leased task synchronously in a worker.
func (s *Service) RunQueuedTask(ctx context.Context, task *db.QueuedTask) (*model.Job, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	fn, err := s.runnerFor(task.Type)
	if err != nil {
		return nil, err
	}
	job, err := s.startNow(ctx, task.ID, task.Type, task.Options, fn, false)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case model.JobCompleted:
		return job, nil
	case model.JobCanceled:
		return job, context.Canceled
	default:
		if job.Error != "" {
			return job, errors.New(job.Error)
		}
		return job, fmt.Errorf("job finished with status %s", job.Status)
	}
}

func (s *Service) startNow(parent context.Context, id, typ string, opts map[string]any, fn runner, async bool) (*model.Job, error) {
	if parent == nil {
		parent = context.Background()
	}
	parent = extractJobTrace(parent, opts)
	s.mu.Lock()
	if len(s.active) >= s.maxJobs {
		s.mu.Unlock()
		return nil, fmt.Errorf("job concurrency limit reached")
	}
	if id == "" {
		id = newID()
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	s.active[id] = activeJob{cancel: cancel, done: done}
	s.mu.Unlock()

	j := s.newJob(id, typ, opts)
	if s.db != nil {
		if persisted, err := s.db.GetJob(id); err == nil {
			j.CreatedAt = persisted.CreatedAt
		}
	}
	if s.db != nil {
		if err := s.db.SaveJob(j); err != nil {
			slog.Warn("persist new job", "id", j.ID, "err", err)
		}
		if err := s.db.Audit(j.Actor, "job.start", typ+" "+j.ID); err != nil {
			slog.Warn("persist job audit", "id", j.ID, "err", err)
		}
	}
	run := func() {
		requestID, _ := opts["_request_id"].(string)
		runCtx, span := otel.Tracer("nodeharvest/job").Start(ctx, "job."+typ, trace.WithAttributes(
			attribute.String("job.id", j.ID), attribute.String("job.type", typ),
			attribute.String("job.tenant", j.TenantID), attribute.String("request.id", requestID),
		))
		slog.Info("job started", "job_id", j.ID, "job_type", typ, "request_id", requestID)
		start := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				s.fail(j, fmt.Errorf("job panic: %v", recovered))
			}
			cancel()
			if s.metrics != nil {
				s.metrics.ObserveJob(typ, string(j.Status), time.Since(start).Seconds())
				s.metrics.IncJob(typ, string(j.Status))
			}
			span.SetAttributes(attribute.String("job.status", string(j.Status)))
			if j.Error != "" {
				span.RecordError(errors.New(j.Error))
				span.SetStatus(codes.Error, j.Error)
			}
			slog.Info("job finished", "job_id", j.ID, "job_type", typ,
				"request_id", requestID, "status", j.Status, "dur_ms", time.Since(start).Milliseconds())
			span.End()
			s.mu.Lock()
			delete(s.active, id)
			close(done)
			s.mu.Unlock()
		}()
		now := time.Now()
		j.StartedAt = &now
		s.updateJob(j, model.JobRunning, 1, "starting")
		fn(runCtx, j)
	}
	if async {
		go run()
		if snapshot, ok := s.store.GetJob(j.ID); ok {
			return snapshot, nil
		}
		return nil, fmt.Errorf("job %s snapshot unavailable", j.ID)
	}
	run()
	return j, nil
}

func extractJobTrace(parent context.Context, opts map[string]any) context.Context {
	carrier := propagation.MapCarrier{}
	if value, ok := opts["_traceparent"].(string); ok {
		carrier.Set("traceparent", value)
	}
	if value, ok := opts["_tracestate"].(string); ok {
		carrier.Set("tracestate", value)
	}
	return otel.GetTextMapPropagator().Extract(parent, carrier)
}

// CancelJob 取消当前运行任务
func (s *Service) CancelJob() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	canceled := false
	for _, active := range s.active {
		active.cancel()
		canceled = true
	}
	return canceled
}

func (s *Service) CancelJobID(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.active[id]
	if ok {
		active.cancel()
	}
	return ok
}

func (s *Service) WaitForJob(ctx context.Context) error {
	s.mu.Lock()
	pending := make([]chan struct{}, 0, len(s.active))
	for _, active := range s.active {
		pending = append(pending, active.done)
	}
	s.mu.Unlock()
	for _, done := range pending {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *Service) runFull(ctx context.Context, j *model.Job) {
	if err := s.doFetch(ctx, j, 0, 35); err != nil {
		s.fail(j, err)
		return
	}
	if err := s.doQuality(ctx, j, 35, 75); err != nil {
		s.fail(j, err)
		return
	}
	skipAI := false
	if v, ok := j.Options["skip_ai"].(bool); ok {
		skipAI = v
	}
	if !skipAI {
		if err := s.doAI(ctx, j, 75, 95); err != nil {
			// AI 失败不整体失败
			slog.Warn("ai probe partial", "err", err)
			j.Stats["ai_error"] = err.Error()
		}
	} else {
		j.Stats["ai_skipped"] = true
		s.updateJob(j, model.JobRunning, 90, "ai skipped")
	}
	s.doExport(j)
	s.complete(j, "full pipeline done")
}

func (s *Service) runFetch(ctx context.Context, j *model.Job) {
	if err := s.doFetch(ctx, j, 0, 90); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "fetch done")
}

func (s *Service) runQuality(ctx context.Context, j *model.Job) {
	if err := s.doQuality(ctx, j, 0, 95); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "quality done")
}

func (s *Service) runAI(ctx context.Context, j *model.Job) {
	if err := s.doAI(ctx, j, 0, 95); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "ai probe done")
}

func (s *Service) runGeo(ctx context.Context, j *model.Job) {
	if err := s.doGeo(ctx, j, 0, 95); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "geo annotate done")
}

func (s *Service) runPurity(ctx context.Context, j *model.Job) {
	if err := s.doPurity(ctx, j, 0, 95); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "purity done")
}

func (s *Service) runDial(ctx context.Context, j *model.Job) {
	if err := s.doDial(ctx, j, 0, 95); err != nil {
		s.fail(j, err)
		return
	}
	s.doExport(j)
	s.complete(j, "dial done")
}

func (s *Service) doFetch(ctx context.Context, j *model.Job, p0, p1 float64) error {
	s.updateJob(j, model.JobRunning, p0, "fetching sources")
	sources := s.EnabledSources()
	if len(sources) == 0 {
		return fmt.Errorf("no enabled sources")
	}
	f := fetcher.New(s.cfg.FetchTimeout(), s.cfg.App.UserAgent)
	docs := f.FetchAll(ctx, sources, min(16, len(sources)))
	if err := ctx.Err(); err != nil {
		return err
	}

	okSources := 0
	var all []*model.Node
	failed := make(map[string]bool)
	now := time.Now()
	for _, d := range docs {
		if d.Err != nil {
			slog.Warn("fetch failed", "source", d.Source.Name, "err", d.Err)
			s.recordSource(d, d.Err)
			failed[d.Source.Name] = true
			continue
		}
		nodes := parser.ParseContent(d.Body, d.Source.Name)
		if len(nodes) == 0 {
			err := fmt.Errorf("no supported nodes parsed")
			slog.Warn("source parse empty", "source", d.Source.Name)
			s.recordSource(d, err)
			failed[d.Source.Name] = true
			continue
		}
		okSources++
		s.recordSource(d, nil)
		for _, n := range nodes {
			n.LastSeenAt = now
		}
		all = append(all, nodes...)
	}
	if okSources == 0 {
		return fmt.Errorf("all %d sources failed; keeping last-good snapshot", len(sources))
	}

	// 失败源沿用上次节点；LastSeenAt 不刷新，发布层会按年龄自动淘汰。
	carried := 0
	if len(failed) > 0 {
		for _, n := range s.store.AllNodes() {
			if failed[n.Source] {
				all = append(all, n)
				carried++
			}
		}
	}
	unique := cleaner.Clean(all, s.cfg)
	if len(unique) == 0 {
		return fmt.Errorf("all fetched documents parsed empty; keeping last-good snapshot")
	}
	if err := s.replaceNodes(unique); err != nil {
		return fmt.Errorf("persist fetched nodes: %w", err)
	}
	s.updateSourceContributions(unique)

	j.Stats["sources_ok"] = okSources
	j.Stats["sources_failed"] = len(failed)
	j.Stats["last_good_carried"] = carried
	j.Stats["parsed"] = len(all)
	j.Stats["unique"] = len(unique)
	s.updateJob(j, model.JobRunning, p1, fmt.Sprintf("fetched unique=%d", len(unique)))
	return nil
}

func (s *Service) doQuality(ctx context.Context, j *model.Job, p0, p1 float64) error {
	cfg := s.Config()
	nodes := s.store.AllNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes to test")
	}
	// 未测优先；失败节点指数退避，稳定节点周期复测。
	now := time.Now()
	due := nodes[:0]
	for _, n := range nodes {
		if n.NextTestAt.IsZero() || !n.NextTestAt.After(now) {
			due = append(due, n)
		}
	}
	nodes = due
	sort.SliceStable(nodes, func(i, k int) bool {
		a, b := nodes[i], nodes[k]
		if a.TestedAt.IsZero() != b.TestedAt.IsZero() {
			return a.TestedAt.IsZero()
		}
		if !a.TestedAt.Equal(b.TestedAt) {
			return a.TestedAt.Before(b.TestedAt)
		}
		return a.Score > b.Score
	})
	maxTest := cfg.Schedule.MaxTest
	if maxTest <= 0 {
		maxTest = 1500
	}
	if v, ok := j.Options["max_test"].(float64); ok && int(v) > 0 {
		maxTest = int(v)
	}
	if len(nodes) > maxTest {
		nodes = nodes[:maxTest]
	}
	if len(nodes) == 0 {
		j.Stats["tested"] = 0
		s.updateJob(j, model.JobRunning, p1, "quality: no nodes due")
		return nil
	}

	s.updateJob(j, model.JobRunning, p0, fmt.Sprintf("quality testing %d nodes", len(nodes)))
	rounds := 3
	if v, ok := j.Options["rounds"].(float64); ok && int(v) > 0 {
		rounds = int(v)
	}

	t := quality.New(quality.Options{
		Concurrency: cfg.App.Concurrency,
		Timeout:     cfg.TestTimeout(),
		Rounds:      rounds,
		TLSProbe:    true,
		EdgeProbe:   false, // 全量时太慢；仅 host 级测一次
		OnProgress: func(done, total int) {
			prog := p0 + (p1-p0)*float64(done)/float64(total)
			if done%50 == 0 || done == total {
				s.updateJob(j, model.JobRunning, prog, fmt.Sprintf("quality %d/%d", done, total))
			}
		},
	})
	t.TestAll(ctx, nodes)
	if err := ctx.Err(); err != nil {
		return err
	}

	// 企业级评分 v2（可解释多因子）
	stability := map[string]float64{}
	if s.db != nil {
		var err error
		stability, err = s.db.NodeStabilities(7)
		if err != nil {
			slog.Warn("load node stability", "err", err)
		}
	}
	weights := qualityWeights(cfg)
	for _, n := range nodes {
		if n != nil && n.Alive {
			n.QualityFailures = 0
			n.SuccessStreak++
			n.NextTestAt = time.Now().Add(6 * time.Hour)
			if n.Quality != nil {
				if historical, ok := stability[n.ID]; ok {
					n.Quality.Stability7D = historical
				} else {
					n.Quality.Stability7D = n.Quality.SuccessRate
				}
			}
			quality.ApplyV2(n, weights)
		} else if n != nil {
			n.QualityFailures++
			n.SuccessStreak = 0
			backoff := time.Hour << min(n.QualityFailures-1, 5)
			n.NextTestAt = time.Now().Add(backoff)
		}
	}

	// 写回测速结果
	if err := s.upsertNodes(nodes, true); err != nil {
		return fmt.Errorf("persist quality results: %w", err)
	}
	s.updateSourceContributions(s.store.AllNodes())

	// 清理：可选剔除死亡节点，只保留高质量候选
	pruned := 0
	if cfg.Filter.PruneAfterQuality {
		var err error
		pruned, err = s.store.Prune(store.PruneOptions{
			DropDead:     true,
			MinScoreKeep: 0, // 死亡已丢；未测保留
			MaxNodes:     cfg.Filter.MaxStoreNodes,
			KeepUntested: true,
		})
		if err != nil {
			return fmt.Errorf("persist pruned nodes: %w", err)
		}
		if s.db != nil {
			if err := s.db.ReplaceNodes(s.store.AllNodes()); err != nil {
				return fmt.Errorf("persist pruned durable nodes: %w", err)
			}
		}
	}

	// 国家标注（优先存活/高质量）
	if cfg.Geo.Enabled && cfg.Geo.AnnotateAfterQuality {
		// 进度落在 p1 前一点
		mid := p0 + (p1-p0)*0.85
		if err := s.doGeo(ctx, j, mid, p0+(p1-p0)*0.92); err != nil {
			slog.Warn("geo annotate", "err", err)
			j.Stats["geo_error"] = err.Error()
		}
	}

	// 可选：quality 后自动真拨（AfterQualityMax=0 表示全部 HQ，按 batch_size 多轮）
	if cfg.Dial.Enabled && cfg.Dial.AfterQuality {
		mid := p0 + (p1-p0)*0.93
		if j.Options == nil {
			j.Options = map[string]any{}
		}
		if _, ok := j.Options["max_dial"]; !ok {
			maxDial := cfg.Dial.AfterQualityMax
			if maxDial <= 0 && cfg.Dial.SamplePercent > 0 {
				hqCount := len(s.store.ListNodes(store.NodeFilter{
					AliveOnly: true, MinScore: cfg.Filter.MinScore,
				}))
				maxDial = max(1, int(math.Ceil(float64(hqCount)*cfg.Dial.SamplePercent/100)))
			}
			j.Options["max_dial"] = float64(maxDial)
			if maxDial <= 0 {
				j.Options["all_hq"] = true
			}
		}
		if err := s.doDial(ctx, j, mid, p1); err != nil {
			slog.Warn("dial after quality", "err", err)
			j.Stats["dial_error"] = err.Error()
		}
	}

	alive, hq := 0, 0
	testedAlive := 0
	for _, node := range nodes {
		if node != nil && node.Alive {
			testedAlive++
		}
	}
	minScore := s.Config().Filter.MinScore
	if minScore <= 0 {
		minScore = 70
	}
	byCountryHQ := map[string]int{}
	for _, n := range s.store.AllNodes() {
		if n.Alive {
			alive++
		}
		if n.Alive && n.Score >= minScore {
			hq++
			cc := n.Country
			if cc == "" {
				cc = "XX"
			}
			byCountryHQ[cc]++
		}
	}
	j.Stats["tested"] = len(nodes)
	j.Stats["alive"] = alive
	j.Stats["high_quality"] = hq
	j.Stats["pruned"] = pruned
	j.Stats["stored"] = s.store.Count()
	j.Stats["by_country_hq"] = byCountryHQ
	s.detectQualityAnomalies(j, len(nodes), testedAlive, hq, byCountryHQ)
	s.updateJob(j, model.JobRunning, p1, quality.Summary(nodes))
	return nil
}

func (s *Service) doDial(ctx context.Context, j *model.Job, p0, p1 float64) error {
	cfg := s.Config()
	if cfg == nil || !cfg.Dial.Enabled {
		return fmt.Errorf("dial disabled in config")
	}
	s.updateJob(j, model.JobRunning, p0, "real protocol dial starting")

	// max_dial: 显式 >0 限制数量；0 / all_hq=true = 全部 HQ
	maxDial := cfg.Dial.MaxNodes
	if v, ok := j.Options["max_dial"].(float64); ok {
		maxDial = int(v)
	} else if v, ok := j.Options["max_dial"].(int); ok {
		maxDial = v
	}
	allHQ := maxDial <= 0
	if v, ok := j.Options["all_hq"].(bool); ok && v {
		allHQ = true
		maxDial = 0
	}

	batchSize := cfg.Dial.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	if v, ok := j.Options["batch_size"].(float64); ok && int(v) > 0 {
		batchSize = int(v)
	}

	onlyUnverified := true
	if v, ok := j.Options["force"].(bool); ok && v {
		onlyUnverified = false
	}

	list := s.pickDialCandidates(maxDial, onlyUnverified, allHQ)
	if len(list) == 0 {
		if onlyUnverified && len(s.pickDialCandidates(maxDial, false, allHQ)) > 0 {
			j.Stats["dial_planned"] = 0
			s.updateJob(j, model.JobRunning, p1, "all dialable nodes already have a recent result")
			return nil
		}
		return fmt.Errorf("no dialable nodes for engine %s", cfg.Dial.Engine)
	}
	if allHQ {
		j.Stats["dial_pool_mode"] = "all_hq_batched"
	} else {
		j.Stats["dial_pool_mode"] = "limited_diversified"
	}
	j.Stats["dial_batch_size"] = batchSize
	j.Stats["dial_planned"] = len(list)

	engines := configuredDialEngines(cfg)
	resolvedBins := make(map[string]string, len(engines))
	for i := range engines {
		bin, engine, err := dialer.AvailableFor(engines[i].bin, engines[i].engine)
		if err != nil {
			return fmt.Errorf("%w; %s", err, dialer.InstallHint())
		}
		engines[i].bin = bin
		engines[i].engine = engine
		resolvedBins[engine] = bin
	}
	j.Stats["dial_engine"] = cfg.Dial.Engine
	j.Stats["dial_bins"] = resolvedBins
	j.Stats["dial_target"] = cfg.Dial.TestURL

	total := len(list)
	rounds := (total + batchSize - 1) / batchSize
	j.Stats["dial_rounds"] = rounds
	s.updateJob(j, model.JobRunning, p0+0.5,
		fmt.Sprintf("dial %d HQ nodes in %d rounds x%d via %s", total, rounds, batchSize, cfg.Dial.Engine))

	okN, failN := 0, 0
	var latSum int64
	doneTotal := 0
	engineStats := make(map[string]map[string]int, len(engines))
	for _, engine := range engines {
		engineStats[engine.engine] = map[string]int{"ok": 0, "fail": 0}
	}

	for r := 0; r < rounds; r++ {
		if err := ctx.Err(); err != nil {
			j.Stats["dial_canceled_at"] = doneTotal
			return err
		}
		start := r * batchSize
		end := start + batchSize
		if end > total {
			end = total
		}
		batch := list[start:end]
		round := r + 1
		s.updateJob(j, model.JobRunning, p0+(p1-p0)*float64(start)/float64(total),
			fmt.Sprintf("dial round %d/%d (%d-%d/%d)", round, rounds, start+1, end, total))

		checks := make([][]*model.DialResult, len(batch))
		for engineIndex, engine := range engines {
			engine := engine
			d, err := dialer.New(dialer.Options{
				Bin:           engine.bin,
				Engine:        engine.engine,
				Concurrency:   cfg.Dial.Concurrency,
				Timeout:       time.Duration(cfg.Dial.TimeoutSec) * time.Second,
				TestURL:       cfg.Dial.TestURL,
				DownloadBytes: cfg.Dial.DownloadBytes,
				WorkDir:       filepath.Join("data/dial-tmp", engine.engine),
				OnProgress: func(done, batchTotal int) {
					checked := start*len(engines) + engineIndex*len(batch) + done
					planned := total * len(engines)
					prog := p0 + (p1-p0)*float64(checked)/float64(planned)
					if done%10 == 0 || done == batchTotal {
						s.updateJob(j, model.JobRunning, prog,
							fmt.Sprintf("%s round %d/%d checks %d/%d", engine.engine, round, rounds, checked, planned))
					}
				},
			})
			if err != nil {
				return err
			}
			d.TestAll(ctx, batch)
			if err := ctx.Err(); err != nil {
				return err
			}
			for i, node := range batch {
				if node.Dial == nil {
					continue
				}
				check := *node.Dial
				check.Checks = nil
				checks[i] = append(checks[i], &check)
				if check.OK {
					engineStats[engine.engine]["ok"]++
				} else {
					engineStats[engine.engine]["fail"]++
				}
			}
		}
		for i, node := range batch {
			node.Dial = combineDialChecks(checks[i])
			node.Verified = node.Dial != nil && node.Dial.OK
			node.Tags = setDialTag(node.Tags, node.Verified)
		}
		for _, node := range batch {
			if node.Dial == nil {
				continue
			}
			if node.Quality == nil {
				node.Quality = &model.Quality{SuccessRate: boolScore(node.Dial.OK)}
			}
			node.Quality.HTTPMS = node.Dial.HTTPMS
			node.Quality.ThroughputBPS = node.Dial.ThroughputBPS
			quality.ApplyV2(node, qualityWeights(cfg))
		}
		if err := s.upsertNodes(batch, true); err != nil {
			return fmt.Errorf("persist dial results: %w", err)
		}

		for _, n := range batch {
			if n.Dial != nil && n.Dial.OK {
				okN++
				latSum += n.Dial.LatencyMS
			} else {
				failN++
			}
		}
		doneTotal = end
		j.Stats["dial_ok"] = okN
		j.Stats["dial_fail"] = failN
		j.Stats["dial_done"] = doneTotal
		j.Stats["verified_total"] = len(s.store.ListNodes(store.NodeFilter{
			VerifiedOnly: true, DialTestedAfter: s.verifiedAfter(), DialEngine: s.verifiedEngine(), Limit: 20000,
		}))
		if cfg.Publish.PreRender {
			s.RefreshPublishCache()
		}
		s.updateJob(j, model.JobRunning, p0+(p1-p0)*float64(doneTotal)/float64(total),
			fmt.Sprintf("dial round %d/%d done ok=%d fail=%d", round, rounds, okN, failN))
	}

	avg := int64(0)
	if okN > 0 {
		avg = latSum / int64(okN)
	}
	j.Stats["dial_ok"] = okN
	j.Stats["dial_fail"] = failN
	j.Stats["dial_avg_ms"] = avg
	j.Stats["dial_by_engine"] = engineStats
	j.Stats["verified_total"] = len(s.store.ListNodes(store.NodeFilter{
		VerifiedOnly: true, DialTestedAfter: s.verifiedAfter(), DialEngine: s.verifiedEngine(), Limit: 20000,
	}))
	s.updateJob(j, model.JobRunning, p1, fmt.Sprintf("dial ok=%d fail=%d rounds=%d planned=%d", okN, failN, rounds, total))
	return nil
}

// pickDialCandidates 选取拨测目标。
// allHQ=true 或 maxDial<=0：取全部存活 HQ（可拨协议），按 score 排序后整表返回。
// 否则：限制数量 + 国家/协议分散，避免只测 CDN 假活。
func (s *Service) pickDialCandidates(maxDial int, onlyUnverified bool, allHQ bool) []*model.Node {
	minScore := 0.0
	if cfg := s.Config(); cfg != nil && cfg.Filter.MinScore > 0 {
		minScore = cfg.Filter.MinScore
	}
	// 全量 HQ：不 Limit；限量模式：放大采样池
	poolLimit := 0
	if !allHQ && maxDial > 0 {
		poolLimit = maxDial * 25
		if poolLimit < 200 {
			poolLimit = 200
		}
		if poolLimit > 8000 {
			poolLimit = 8000
		}
	}
	candidates := s.store.ListNodes(store.NodeFilter{
		AliveOnly:   true,
		HighQuality: minScore >= 70 || minScore <= 0,
		MinScore:    minScore,
		Limit:       poolLimit,
	})
	if len(candidates) == 0 {
		candidates = s.store.ListNodes(store.NodeFilter{AliveOnly: true, Limit: poolLimit})
	}

	type scored struct {
		n     *model.Node
		score float64
	}
	var ranked []scored
	engine := "auto"
	retestAfter := time.Time{}
	if cfg := s.Config(); cfg != nil {
		engine = cfg.Dial.Engine
		retestAfter = s.verifiedAfter()
	}
	for _, n := range candidates {
		if !dialer.SupportsEngine(n, engine) {
			continue
		}
		if onlyUnverified && n.Dial != nil && n.Dial.HasEngine(engine) &&
			!n.Dial.TestedAt.IsZero() && n.Dial.TestedAt.After(retestAfter) {
			continue
		}
		latMs := n.Latency.Milliseconds()
		if latMs <= 0 && n.Quality != nil {
			latMs = n.Quality.AvgLatencyMS
		}
		sc := n.Score
		if latMs > 0 && latMs < 8 {
			sc -= 25
		} else if latMs >= 15 && latMs <= 800 {
			sc += 8
		} else if latMs > 1500 {
			sc -= 10
		}
		if n.Verified {
			sc += 5
		}
		switch n.Protocol {
		case model.ProtoSS, model.ProtoTrojan, model.ProtoHysteria2:
			sc += 3
		}
		// 未测过的优先于已 dial-fail（全量时先测新的）
		if hasTag(n.Tags, "dial-fail") {
			sc -= 5
		}
		ranked = append(ranked, scored{n: n, score: sc})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].n.Latency < ranked[j].n.Latency
	})

	// 全部 HQ：直接返回全表
	if allHQ || maxDial <= 0 {
		out := make([]*model.Node, 0, len(ranked))
		for _, it := range ranked {
			out = append(out, it.n)
		}
		return out
	}

	// 限量：国家/协议分散
	perKey := maxDial/8 + 2
	if perKey < 3 {
		perKey = 3
	}
	if perKey > 12 {
		perKey = 12
	}
	usedKey := map[string]int{}
	usedServer := map[string]int{}
	out := make([]*model.Node, 0, maxDial)
	for _, it := range ranked {
		if len(out) >= maxDial {
			break
		}
		n := it.n
		cc := strings.ToUpper(n.Country)
		if cc == "" {
			cc = "XX"
		}
		key := cc + "|" + string(n.Protocol)
		if usedKey[key] >= perKey {
			continue
		}
		if usedServer[n.Server] >= 2 {
			continue
		}
		usedKey[key]++
		usedServer[n.Server]++
		out = append(out, n)
	}
	if len(out) < maxDial {
		seen := map[string]bool{}
		for _, n := range out {
			seen[n.ID] = true
		}
		for _, it := range ranked {
			if len(out) >= maxDial {
				break
			}
			if seen[it.n.ID] {
				continue
			}
			if usedServer[it.n.Server] >= 3 {
				continue
			}
			usedServer[it.n.Server]++
			out = append(out, it.n)
		}
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func qualityWeights(cfg *config.Config) quality.Weights {
	if cfg == nil {
		return quality.DefaultWeights()
	}
	return quality.Weights{
		Latency: cfg.QualityV2.Latency, Success: cfg.QualityV2.Success,
		Stability: cfg.QualityV2.Stability, TLS: cfg.QualityV2.TLS,
		HTTP: cfg.QualityV2.HTTP, Throughput: cfg.QualityV2.Throughput,
	}
}

func boolScore(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

type dialEngineSpec struct {
	engine string
	bin    string
}

func configuredDialEngines(cfg *config.Config) []dialEngineSpec {
	if cfg.Dial.Engine != "both" {
		bin := cfg.Dial.Bin
		if cfg.Dial.Engine == "mihomo" && cfg.Dial.MihomoBin != "" {
			bin = cfg.Dial.MihomoBin
		}
		return []dialEngineSpec{{engine: cfg.Dial.Engine, bin: bin}}
	}
	return []dialEngineSpec{
		{engine: "sing-box", bin: cfg.Dial.Bin},
		{engine: "mihomo", bin: cfg.Dial.MihomoBin},
	}
}

func combineDialChecks(checks []*model.DialResult) *model.DialResult {
	if len(checks) == 0 {
		return nil
	}
	authoritative := checks[len(checks)-1]
	for _, check := range checks {
		if check != nil && check.Engine == "mihomo" {
			authoritative = check
			break
		}
	}
	if authoritative == nil {
		return nil
	}
	result := *authoritative
	if len(checks) == 1 {
		result.Checks = nil
		return &result
	}
	result.Engine = "both"
	result.Checks = checks
	for _, check := range checks {
		if check != nil && check.TestedAt.After(result.TestedAt) {
			result.TestedAt = check.TestedAt
		}
	}
	return &result
}

func setDialTag(tags []string, verified bool) []string {
	add, remove := "verified", "dial-fail"
	if !verified {
		add, remove = remove, add
	}
	out := tags[:0]
	for _, tag := range tags {
		if tag != add && tag != remove {
			out = append(out, tag)
		}
	}
	return append(out, add)
}

// doPurity 对 verified 节点做纯净度 / CF 挑战探测
func (s *Service) doPurity(ctx context.Context, j *model.Job, p0, p1 float64) error {
	s.updateJob(j, model.JobRunning, p0, "purity probe starting")
	list := s.store.ListNodes(store.NodeFilter{
		VerifiedOnly: true, AliveOnly: true, DialTestedAfter: s.verifiedAfter(),
		DialEngine: s.verifiedEngine(), Limit: 5000,
	})
	if len(list) == 0 {
		return fmt.Errorf("no verified nodes to probe; run dial first")
	}

	maxN := 0
	if v, ok := j.Options["max_nodes"].(float64); ok && int(v) > 0 {
		maxN = int(v)
	}
	conc := 3
	if v, ok := j.Options["concurrency"].(float64); ok && int(v) > 0 {
		conc = int(v)
	}
	bin := ""
	if s.cfg != nil {
		bin = s.cfg.Dial.Bin
	}

	pr, err := purity.New(purity.Options{
		Bin:         bin,
		Concurrency: conc,
		Timeout:     28 * time.Second,
		WorkDir:     "data/purity-tmp",
		MaxNodes:    maxN,
		OnProgress: func(done, total int) {
			prog := p0 + (p1-p0)*float64(done)/float64(total)
			if done%5 == 0 || done == total {
				s.updateJob(j, model.JobRunning, prog, fmt.Sprintf("purity %d/%d", done, total))
			}
		},
	})
	if err != nil {
		return err
	}
	j.Stats["purity_planned"] = len(list)
	if maxN > 0 && len(list) > maxN {
		j.Stats["purity_planned"] = maxN
	}
	s.updateJob(j, model.JobRunning, p0+1, fmt.Sprintf("purity probing %v nodes", j.Stats["purity_planned"]))
	pr.TestAll(ctx, list)
	if err := s.upsertNodes(list, true); err != nil {
		return fmt.Errorf("persist purity results: %w", err)
	}

	okN, cfOK, cleanN := 0, 0, 0
	var sumClean int
	byGrade := map[string]int{}
	for _, n := range list {
		if n.Purity == nil {
			continue
		}
		if n.Purity.OK {
			okN++
			sumClean += n.Purity.CleanScore
		}
		if n.Purity.CFHumanLikely {
			cfOK++
		}
		if n.Purity.CleanScore >= 70 {
			cleanN++
		}
		if n.Purity.Grade != "" {
			byGrade[n.Purity.Grade]++
		}
	}
	avg := 0
	if okN > 0 {
		avg = sumClean / okN
	}
	j.Stats["purity_ok"] = okN
	j.Stats["purity_cf_ok"] = cfOK
	j.Stats["purity_clean70"] = cleanN
	j.Stats["purity_avg_clean"] = avg
	j.Stats["purity_by_grade"] = byGrade
	s.updateJob(j, model.JobRunning, p1, purity.Summary(list))
	return nil
}

func (s *Service) doGeo(ctx context.Context, j *model.Job, p0, p1 float64) error {
	_ = ctx
	if s.geo == nil {
		return fmt.Errorf("geo locator not enabled")
	}
	if !s.geo.Ready() && s.cfg.Geo.AutoDownload {
		if err := s.geo.EnsureDB(s.cfg.Geo.DownloadURL); err != nil {
			slog.Warn("geo download failed, continue with name heuristic", "err", err)
		}
	}
	s.updateJob(j, model.JobRunning, p0, "geo annotating countries")

	// 优先标注存活；若太少则全库
	nodes := s.store.ListNodes(store.NodeFilter{AliveOnly: true, Limit: 3000})
	if len(nodes) < 20 {
		nodes = s.store.AllNodes()
	}
	// 选项：force 重标
	onlyEmpty := true
	if v, ok := j.Options["force"].(bool); ok && v {
		onlyEmpty = false
	}
	minScore := s.cfg.Filter.MinScore
	if minScore <= 0 {
		minScore = 70
	}
	nOK := s.geo.AnnotateNodes(nodes, geo.AnnotateOptions{
		Concurrency: 48,
		OnlyAlive:   false,
		OnlyEmpty:   onlyEmpty,
		OnProgress: func(done, total int) {
			prog := p0 + (p1-p0)*float64(done)/float64(total)
			if done%50 == 0 || done == total {
				s.updateJob(j, model.JobRunning, prog, fmt.Sprintf("geo %d/%d", done, total))
			}
		},
	})
	if err := s.upsertNodes(nodes, false); err != nil {
		s.metrics.ObserveGeo("error", 1)
		return fmt.Errorf("persist geo results: %w", err)
	}
	s.metrics.ObserveGeo("ok", nOK)
	if missing := len(nodes) - nOK; missing > 0 {
		s.metrics.ObserveGeo("unknown", missing)
	}

	// 额外：对高质量未标的再扫一遍名称启发（已在 Lookup 内）
	hqMap := geo.CountByCountry(s.store.AllNodes(), true, true, minScore)
	j.Stats["geo_annotated"] = nOK
	j.Stats["geo_mmdb"] = s.geo.Ready()
	j.Stats["by_country_hq"] = hqMap
	s.updateJob(j, model.JobRunning, p1, fmt.Sprintf("geo ok=%d countries=%d", nOK, len(hqMap)))
	return nil
}

func (s *Service) doAI(ctx context.Context, j *model.Job, p0, p1 float64) error {
	s.updateJob(j, model.JobRunning, p0, "probing AI targets")
	socks := ""
	if v, ok := j.Options["socks5"].(string); ok {
		socks = aiaccess.ParseSocksURL(v)
	}
	if socks == "" {
		socks = os.Getenv("NODE_HARVEST_SOCKS5")
	}

	p := aiaccess.New(aiaccess.Options{
		Concurrency:  24,
		Timeout:      8 * time.Second,
		Socks5Addr:   socks,
		HeuristicTCP: true,
		OnProgress: func(done, total int) {
			prog := p0 + (p1-p0)*0.8*float64(done)/float64(total)
			if done%20 == 0 || done == total {
				s.updateJob(j, model.JobRunning, prog, fmt.Sprintf("ai nodes %d/%d", done, total))
			}
		},
	})

	// 本机直连基线
	hostAI := p.ProbeHost(ctx)
	if err := s.store.SetHostAI(hostAI); err != nil {
		return fmt.Errorf("persist host AI results: %w", err)
	}

	// 节点探测：优先高质量/存活
	nodes := s.store.ListNodes(store.NodeFilter{AliveOnly: true, Limit: 400})
	if len(nodes) < 50 {
		nodes = s.store.ListNodes(store.NodeFilter{Limit: 200})
	}
	p.ProbeNodes(ctx, nodes)
	if err := s.upsertNodes(nodes, false); err != nil {
		return fmt.Errorf("persist AI results: %w", err)
	}

	pass := map[string]int{}
	for _, n := range nodes {
		for k, r := range n.AIAccess {
			if r != nil && r.OK {
				pass[k]++
			}
		}
	}
	j.Stats["ai_nodes"] = len(nodes)
	j.Stats["ai_pass"] = pass
	j.Stats["socks5"] = socks != ""
	s.updateJob(j, model.JobRunning, p1, "ai probe finished")
	return nil
}

func (s *Service) doExport(j *model.Job) {
	cfg := s.Config()
	hq := s.SelectPublishNodes()
	paths, err := exporter.Export(hq, cfg)
	if err != nil {
		j.Stats["export_error"] = err.Error()
		return
	}
	j.Stats["exported"] = len(hq)
	j.Stats["export_files"] = paths

	// 预渲染订阅缓存（热路径）
	if cfg.Publish.PreRender {
		blob := s.RefreshPublishCache()
		if blob != nil {
			j.Stats["publish_etag"] = blob.ETag
			j.Stats["publish_count"] = blob.Count
			j.Stats["publish_updated_at"] = blob.UpdatedAt
		}
	}

	// 额外导出「AI 友好」列表
	aiNodes := s.store.ListNodes(store.NodeFilter{AliveOnly: true, MinScore: 60, Limit: 300})
	var aiURIs []string
	for _, n := range aiNodes {
		if hasAnyAIOK(n) && n.RawURI != "" {
			aiURIs = append(aiURIs, n.RawURI)
		}
	}
	if len(aiURIs) > 0 {
		dir := cfg.Export.Dir
		if err := os.MkdirAll(dir, 0o700); err != nil {
			j.Stats["ai_export_error"] = err.Error()
		} else {
			p := filepath.Join(dir, "nodes-ai-friendly.txt")
			if err := os.WriteFile(p, []byte(strings.Join(aiURIs, "\n")+"\n"), 0o600); err != nil {
				j.Stats["ai_export_error"] = err.Error()
			} else if err := os.Chmod(p, 0o600); err != nil {
				j.Stats["ai_export_error"] = err.Error()
			} else {
				j.Stats["ai_export"] = p
			}
		}
	}
}

// SelectPublishNodes 筛选可对外发布的高质量节点。
func (s *Service) SelectPublishNodes() []*model.Node {
	return s.SelectPublishNodesCountry("")
}

// SelectPublishNodesCountry 可按 ISO 国家码筛选（空=全部）
func (s *Service) SelectPublishNodesCountry(country string) []*model.Node {
	cfg := s.Config()
	minScore := cfg.Publish.MinScore
	if minScore <= 0 {
		minScore = cfg.Filter.MinScore
	}
	if minScore <= 0 {
		minScore = 70
	}
	limit := cfg.Publish.MaxNodes
	if limit <= 0 {
		limit = cfg.Filter.MaxNodes
	}
	aliveOnly := true
	if cfg.Publish.Enabled {
		aliveOnly = cfg.Publish.AliveOnly
	}

	f := store.NodeFilter{
		AliveOnly: aliveOnly,
		MinScore:  minScore,
		Limit:     limit * 3,
		Country:   strings.ToUpper(strings.TrimSpace(country)),
		SeenAfter: s.publishSeenAfter(),
	}
	if cfg.Dial.Enabled {
		f.VerifiedOnly = true
		f.DialTestedAfter = s.verifiedAfter()
		f.DialEngine = s.verifiedEngine()
	}
	if minScore >= 70 {
		f.HighQuality = true
	}
	hq := s.store.ListNodes(f)
	if len(hq) == 0 && country == "" && !cfg.Dial.Enabled {
		hq = s.store.ListNodes(store.NodeFilter{
			AliveOnly: true, MinScore: 50, SeenAfter: s.publishSeenAfter(), Limit: limit * 2,
		})
	}
	if len(hq) == 0 && country == "" && !cfg.Dial.Enabled {
		hq = s.store.ListNodes(store.NodeFilter{
			AliveOnly: true, SeenAfter: s.publishSeenAfter(), Limit: limit,
		})
	}
	hq = filter.Apply(hq, cfg)
	if limit > 0 && len(hq) > limit {
		hq = hq[:limit]
	}
	// 导出前可选国旗前缀
	if cfg.Geo.RenameWithFlag {
		for _, n := range hq {
			applyCountryNamePrefix(n)
		}
	}
	return hq
}

func applyCountryNamePrefix(n *model.Node) {
	if n == nil || n.Country == "" || n.Country == "XX" {
		return
	}
	flag := geo.FlagEmoji(n.Country)
	if flag == "" {
		return
	}
	if strings.HasPrefix(n.Name, flag) {
		return
	}
	// 避免重复 ISO 前缀
	name := strings.TrimSpace(n.Name)
	if name == "" {
		n.Name = flag + " " + n.Country
		return
	}
	n.Name = flag + " " + name
}

// PublishFilter 公开订阅用的过滤参数
func (s *Service) PublishFilter() store.NodeFilter {
	cfg := s.Config()
	minScore := cfg.Publish.MinScore
	if minScore <= 0 {
		minScore = cfg.Filter.MinScore
	}
	limit := cfg.Publish.MaxNodes
	if limit <= 0 {
		limit = cfg.Filter.MaxNodes
	}
	nodeFilter := store.NodeFilter{
		AliveOnly:   cfg.Publish.AliveOnly,
		MinScore:    minScore,
		Limit:       limit,
		HighQuality: minScore >= 70,
		SeenAfter:   s.publishSeenAfter(),
	}
	if cfg.Dial.Enabled {
		nodeFilter.VerifiedOnly = true
		nodeFilter.DialTestedAfter = s.verifiedAfter()
		nodeFilter.DialEngine = s.verifiedEngine()
	}
	return nodeFilter
}

func (s *Service) publishSeenAfter() time.Time {
	hours := s.Config().Publish.MaxNodeAgeHours
	if hours <= 0 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour)
}

func (s *Service) verifiedAfter() time.Time {
	hours := s.Config().Dial.VerifiedTTLHours
	if hours <= 0 {
		hours = 6
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour)
}

func (s *Service) VerifiedAfter() time.Time { return s.verifiedAfter() }

func (s *Service) verifiedEngine() string {
	engine := strings.ToLower(strings.TrimSpace(s.Config().Dial.Engine))
	if engine == "auto" {
		return ""
	}
	return engine
}

func (s *Service) VerifiedEngine() string { return s.verifiedEngine() }

func hasAnyAIOK(n *model.Node) bool {
	for _, r := range n.AIAccess {
		if r != nil && r.OK {
			return true
		}
	}
	return false
}

func (s *Service) complete(j *model.Job, msg string) {
	now := time.Now()
	j.EndedAt = &now
	s.updateJob(j, model.JobCompleted, 100, msg)
	if s.db != nil {
		s.db.ResolveAlertIfActive("consecutive-job-failures")
	}
}

func (s *Service) fail(j *model.Job, err error) {
	now := time.Now()
	j.EndedAt = &now
	j.Error = err.Error()
	status := model.JobFailed
	if errors.Is(err, context.Canceled) {
		status = model.JobCanceled
	}
	s.updateJob(j, status, j.Progress, err.Error())
	if status == model.JobFailed && s.db != nil {
		threshold := s.Config().Governance.JobFailureThreshold
		if failures, countErr := s.db.RecentConsecutiveJobFailures(threshold); countErr == nil && failures >= threshold {
			raiseErr := s.raiseAlert("consecutive-job-failures", "critical",
				fmt.Sprintf("%d consecutive jobs failed", failures),
				map[string]any{"failures": failures, "job_id": j.ID, "job_type": j.Type, "error": j.Error})
			if raiseErr != nil {
				slog.Warn("raise job failure alert", "err", raiseErr)
			}
		}
	}
}

func (s *Service) detectQualityAnomalies(
	j *model.Job, tested, testedAlive, highQuality int, byCountry map[string]int,
) {
	if s.db == nil {
		return
	}
	cfg := s.Config().Governance
	if tested > 0 && testedAlive == 0 {
		_ = s.raiseAlert("all-quality-probes-failed", "critical",
			"all quality probes failed", map[string]any{"job_id": j.ID, "tested": tested})
	} else {
		s.db.ResolveAlertIfActive("all-quality-probes-failed")
	}

	dominantCountry, dominantCount := "", 0
	for country, count := range byCountry {
		if count > dominantCount {
			dominantCountry, dominantCount = country, count
		}
	}
	share := 0.0
	if highQuality > 0 {
		share = float64(dominantCount) / float64(highQuality) * 100
	}
	if highQuality > 0 && share >= cfg.CountrySharePercent {
		_ = s.raiseAlert("country-dominance", "warning",
			fmt.Sprintf("%s represents %.1f%% of high-quality nodes", dominantCountry, share),
			map[string]any{"job_id": j.ID, "country": dominantCountry, "share": share, "nodes": dominantCount})
	} else {
		s.db.ResolveAlertIfActive("country-dominance")
	}

	previous, err := s.db.PreviousCompletedQualityStats(j.ID)
	if err != nil {
		return
	}
	previousHQ, _ := previous["high_quality"].(float64)
	drop := 0.0
	if previousHQ > 0 {
		drop = (previousHQ - float64(highQuality)) / previousHQ * 100
	}
	if drop >= cfg.HQDropPercent {
		_ = s.raiseAlert("high-quality-drop", "critical",
			fmt.Sprintf("high-quality node count dropped %.1f%%", drop),
			map[string]any{"job_id": j.ID, "previous": previousHQ, "current": highQuality, "drop_percent": drop})
	} else {
		s.db.ResolveAlertIfActive("high-quality-drop")
	}
}

func (s *Service) raiseAlert(kind, severity, message string, details map[string]any) error {
	alert, err := s.db.RaiseAlert(kind, severity, message, details)
	if err != nil {
		return err
	}
	cfg := s.Config().Governance
	if cfg.AlertWebhookURL == "" {
		return nil
	}
	payload, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.AlertWebhookURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if cfg.AlertWebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.AlertWebhookSecret))
		_, _ = mac.Write(payload)
		request.Header.Set("X-NodeHarvest-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("alert webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

// ExportBase64 返回 base64 订阅
func (s *Service) ExportBase64(f store.NodeFilter) string {
	uris := s.store.ExportURIs(f)
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(uris, "\n")))
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
