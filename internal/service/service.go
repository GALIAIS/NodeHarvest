package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/local/node-hunter/internal/aiaccess"
	"github.com/local/node-hunter/internal/cleaner"
	"github.com/local/node-hunter/internal/config"
	"github.com/local/node-hunter/internal/db"
	"github.com/local/node-hunter/internal/dialer"
	"github.com/local/node-hunter/internal/exporter"
	"github.com/local/node-hunter/internal/fetcher"
	"github.com/local/node-hunter/internal/filter"
	"github.com/local/node-hunter/internal/geo"
	"github.com/local/node-hunter/internal/metrics"
	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/parser"
	"github.com/local/node-hunter/internal/publish"
	"github.com/local/node-hunter/internal/purity"
	"github.com/local/node-hunter/internal/quality"
	"github.com/local/node-hunter/internal/store"
)

// Service 业务编排
type Service struct {
	cfg     *config.Config
	store   *store.Store
	geo     *geo.Locator
	pub     *publish.Cache
	db      *db.Store
	metrics *metrics.Registry
	mu      sync.Mutex
	// 同时只跑一个重任务
	running bool
	// 当前 job 取消
	jobCancel context.CancelFunc
}

type Options struct {
	DB      *db.Store
	Metrics *metrics.Registry
}

func New(cfg *config.Config, st *store.Store) *Service {
	return NewWithOptions(cfg, st, Options{})
}

func NewWithOptions(cfg *config.Config, st *store.Store, opt Options) *Service {
	s := &Service{cfg: cfg, store: st, db: opt.DB, metrics: opt.Metrics}
	if s.metrics == nil {
		s.metrics = metrics.New()
	}
	s.pub = publish.NewCache(cfg.Export.Dir)
	if cfg != nil && cfg.Geo.Enabled {
		s.geo = geo.New(cfg.Geo.DBPath)
		if err := s.geo.Open(); err != nil {
			slog.Info("geoip db not ready yet", "err", err)
		}
		if cfg.Geo.AutoDownload && !s.geo.Ready() {
			go func() {
				if err := s.geo.EnsureDB(cfg.Geo.DownloadURL); err != nil {
					slog.Warn("geoip ensure failed, name heuristic only", "err", err)
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

func (s *Service) Store() *store.Store           { return s.store }
func (s *Service) Config() *config.Config        { return s.cfg }
func (s *Service) Geo() *geo.Locator             { return s.geo }
func (s *Service) PublishCache() *publish.Cache  { return s.pub }
func (s *Service) DB() *db.Store                 { return s.db }
func (s *Service) Metrics() *metrics.Registry    { return s.metrics }
func (s *Service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Flush 持久化快照（优雅退出）
func (s *Service) Flush() {
	// store 在 mutation 时已 persist；这里强制再导一次订阅
	s.RefreshPublishCache()
}

// RefreshPublishCache 预渲染高质量订阅
func (s *Service) RefreshPublishCache() *publish.Blob {
	if s.pub == nil || s.cfg == nil || !s.cfg.Publish.PreRender {
		// 仍尝试更新，便于 /sub 热路径
	}
	nodes := s.SelectPublishNodes()
	maxC := 30
	if s.cfg != nil && s.cfg.Publish.MaxCountries > 0 {
		maxC = s.cfg.Publish.MaxCountries
	}
	blob := s.pub.Update(nodes, maxC)
	if s.metrics != nil {
		st := s.store.Stats(len(s.cfg.EnabledSources()))
		s.metrics.SetNodes(st.TotalNodes, st.AliveNodes, st.HighQuality)
	}
	return blob
}

func (s *Service) newJob(typ string, opts map[string]any) *model.Job {
	id := newID()
	now := time.Now()
	j := &model.Job{
		ID:        id,
		Type:      typ,
		Status:    model.JobPending,
		Message:   "queued",
		CreatedAt: now,
		UpdatedAt: now,
		Options:   opts,
		Stats:     map[string]any{},
	}
	s.store.SaveJob(j)
	return j
}

func (s *Service) updateJob(j *model.Job, status model.JobStatus, progress float64, msg string) {
	j.Status = status
	j.Progress = progress
	j.Message = msg
	j.UpdatedAt = time.Now()
	s.store.SaveJob(j)
	if s.db != nil {
		_ = s.db.SaveJob(j)
		if msg != "" && (int(progress)%25 == 0 || status == model.JobCompleted || status == model.JobFailed) {
			_ = s.db.AddJobEvent(j.ID, "info", msg)
		}
	}
}

// StartFull 采集 + 质量 + AI 启发
func (s *Service) StartFull(opts map[string]any) (*model.Job, error) {
	return s.start("full", opts, s.runFull)
}

func (s *Service) StartFetch(opts map[string]any) (*model.Job, error) {
	return s.start("fetch", opts, s.runFetch)
}

func (s *Service) StartQuality(opts map[string]any) (*model.Job, error) {
	return s.start("quality", opts, s.runQuality)
}

func (s *Service) StartAI(opts map[string]any) (*model.Job, error) {
	return s.start("ai", opts, s.runAI)
}

// StartGeo 仅国家标注
func (s *Service) StartGeo(opts map[string]any) (*model.Job, error) {
	return s.start("geo", opts, s.runGeo)
}

// StartDial 真实协议拨测（sing-box）
func (s *Service) StartDial(opts map[string]any) (*model.Job, error) {
	return s.start("dial", opts, s.runDial)
}

// StartPurity 对 verified 节点做 IP 纯净度 + Cloudflare 挑战启发式探测
func (s *Service) StartPurity(opts map[string]any) (*model.Job, error) {
	return s.start("purity", opts, s.runPurity)
}

type runner func(ctx context.Context, j *model.Job)

func (s *Service) start(typ string, opts map[string]any, fn runner) (*model.Job, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, fmt.Errorf("another job is running")
	}
	s.running = true
	ctx, cancel := context.WithCancel(context.Background())
	s.jobCancel = cancel
	s.mu.Unlock()

	j := s.newJob(typ, opts)
	if s.db != nil {
		_ = s.db.SaveJob(j)
		_ = s.db.Audit("system", "job.start", typ+" "+j.ID)
	}
	go func() {
		start := time.Now()
		defer func() {
			cancel()
			s.mu.Lock()
			s.running = false
			s.jobCancel = nil
			s.mu.Unlock()
			if s.metrics != nil {
				s.metrics.ObserveJob(typ, time.Since(start).Seconds())
				s.metrics.IncJob(typ, string(j.Status))
			}
		}()
		now := time.Now()
		j.StartedAt = &now
		s.updateJob(j, model.JobRunning, 1, "starting")
		fn(ctx, j)
	}()
	return j, nil
}

// CancelJob 取消当前运行任务
func (s *Service) CancelJob() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobCancel != nil {
		s.jobCancel()
		return true
	}
	return false
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
	sources := s.cfg.EnabledSources()
	if len(sources) == 0 {
		return fmt.Errorf("no enabled sources")
	}
	f := fetcher.New(s.cfg.FetchTimeout(), s.cfg.App.UserAgent)
	docs := f.FetchAll(ctx, sources, min(16, len(sources)))

	okSources := 0
	var all []*model.Node
	for _, d := range docs {
		if d.Err != nil {
			slog.Warn("fetch failed", "source", d.Source.Name, "err", d.Err)
			if s.metrics != nil {
				s.metrics.IncSourceErr(d.Source.Name)
			}
			continue
		}
		okSources++
		nodes := parser.ParseContent(d.Body, d.Source.Name)
		all = append(all, nodes...)
	}
	unique := cleaner.Clean(all, s.cfg)
	s.store.ReplaceNodes(unique)

	j.Stats["sources_ok"] = okSources
	j.Stats["parsed"] = len(all)
	j.Stats["unique"] = len(unique)
	s.updateJob(j, model.JobRunning, p1, fmt.Sprintf("fetched unique=%d", len(unique)))
	return nil
}

func (s *Service) doQuality(ctx context.Context, j *model.Job, p0, p1 float64) error {
	nodes := s.store.AllNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes to test")
	}
	// 可选：只测未测或 top N
	maxTest := 1500
	if v, ok := j.Options["max_test"].(float64); ok && int(v) > 0 {
		maxTest = int(v)
	}
	if len(nodes) > maxTest {
		// 优先未测 + 随机截断前 maxTest（已按 score 排过）
		nodes = nodes[:maxTest]
	}

	s.updateJob(j, model.JobRunning, p0, fmt.Sprintf("quality testing %d nodes", len(nodes)))
	rounds := 3
	if v, ok := j.Options["rounds"].(float64); ok && int(v) > 0 {
		rounds = int(v)
	}

	t := quality.New(quality.Options{
		Concurrency: s.cfg.App.Concurrency,
		Timeout:     s.cfg.TestTimeout(),
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

	// 企业级评分 v2（可解释多因子）
	for _, n := range nodes {
		if n != nil && n.Alive {
			quality.ApplyV2(n)
		}
	}

	// 写回测速结果
	s.store.UpsertNodes(nodes)

	// 清理：可选剔除死亡节点，只保留高质量候选
	pruned := 0
	if s.cfg.Filter.PruneAfterQuality {
		pruned = s.store.Prune(store.PruneOptions{
			DropDead:     true,
			MinScoreKeep: 0, // 死亡已丢；未测保留
			MaxNodes:     s.cfg.Filter.MaxStoreNodes,
			KeepUntested: true,
		})
	}

	// 国家标注（优先存活/高质量）
	if s.cfg.Geo.Enabled && s.cfg.Geo.AnnotateAfterQuality {
		// 进度落在 p1 前一点
		mid := p0 + (p1-p0)*0.85
		if err := s.doGeo(ctx, j, mid, p0+(p1-p0)*0.92); err != nil {
			slog.Warn("geo annotate", "err", err)
			j.Stats["geo_error"] = err.Error()
		}
	}

	// 可选：quality 后自动真拨（AfterQualityMax=0 表示全部 HQ，按 batch_size 多轮）
	if s.cfg.Dial.Enabled && s.cfg.Dial.AfterQuality {
		mid := p0 + (p1-p0)*0.93
		if j.Options == nil {
			j.Options = map[string]any{}
		}
		if _, ok := j.Options["max_dial"]; !ok {
			// 0 = 全部 HQ
			j.Options["max_dial"] = float64(s.cfg.Dial.AfterQualityMax)
		}
		if _, ok := j.Options["all_hq"]; !ok && s.cfg.Dial.AfterQualityMax <= 0 {
			j.Options["all_hq"] = true
		}
		if err := s.doDial(ctx, j, mid, p1); err != nil {
			slog.Warn("dial after quality", "err", err)
			j.Stats["dial_error"] = err.Error()
		}
	}

	alive, hq := 0, 0
	minScore := s.cfg.Filter.MinScore
	if minScore <= 0 {
		minScore = 70
	}
	byCountryHQ := map[string]int{}
	for _, n := range s.store.AllNodes() {
		if n.Alive {
			alive++
		}
		if n.Score >= minScore {
			hq++
			if n.Alive {
				cc := n.Country
				if cc == "" {
					cc = "XX"
				}
				byCountryHQ[cc]++
			}
		}
	}
	j.Stats["tested"] = len(nodes)
	j.Stats["alive"] = alive
	j.Stats["high_quality"] = hq
	j.Stats["pruned"] = pruned
	j.Stats["stored"] = s.store.Count()
	j.Stats["by_country_hq"] = byCountryHQ
	s.updateJob(j, model.JobRunning, p1, quality.Summary(nodes))
	return nil
}

func (s *Service) doDial(ctx context.Context, j *model.Job, p0, p1 float64) error {
	if s.cfg == nil || !s.cfg.Dial.Enabled {
		return fmt.Errorf("dial disabled in config")
	}
	s.updateJob(j, model.JobRunning, p0, "real protocol dial starting")

	// max_dial: 显式 >0 限制数量；0 / all_hq=true = 全部 HQ
	maxDial := s.cfg.Dial.MaxNodes
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

	batchSize := s.cfg.Dial.BatchSize
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
		return fmt.Errorf("no dialable nodes (need ss/vmess/vless/trojan/hy2)")
	}
	if allHQ {
		j.Stats["dial_pool_mode"] = "all_hq_batched"
	} else {
		j.Stats["dial_pool_mode"] = "limited_diversified"
	}
	j.Stats["dial_batch_size"] = batchSize
	j.Stats["dial_planned"] = len(list)

	// 解析一次引擎信息（每轮可重建 dialer 以绑定进度）
	probe, err := dialer.New(dialer.Options{
		Bin:         s.cfg.Dial.Bin,
		Engine:      s.cfg.Dial.Engine,
		Concurrency: s.cfg.Dial.Concurrency,
		Timeout:     time.Duration(s.cfg.Dial.TimeoutSec) * time.Second,
		TestURL:     s.cfg.Dial.TestURL,
		WorkDir:     "data/dial-tmp",
	})
	if err != nil {
		return fmt.Errorf("%w; %s", err, dialer.InstallHint())
	}
	j.Stats["dial_engine"] = probe.Engine()
	j.Stats["dial_bin"] = probe.Bin()
	j.Stats["dial_target"] = s.cfg.Dial.TestURL

	total := len(list)
	rounds := (total + batchSize - 1) / batchSize
	j.Stats["dial_rounds"] = rounds
	s.updateJob(j, model.JobRunning, p0+0.5, fmt.Sprintf("dial %d HQ nodes in %d rounds x%d via %s", total, rounds, batchSize, probe.Engine()))

	okN, failN := 0, 0
	var latSum int64
	doneTotal := 0

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

		baseDone := doneTotal
		d, err := dialer.New(dialer.Options{
			Bin:         s.cfg.Dial.Bin,
			Engine:      s.cfg.Dial.Engine,
			Concurrency: s.cfg.Dial.Concurrency,
			Timeout:     time.Duration(s.cfg.Dial.TimeoutSec) * time.Second,
			TestURL:     s.cfg.Dial.TestURL,
			WorkDir:     "data/dial-tmp",
			OnProgress: func(done, batchTotal int) {
				cur := baseDone + done
				prog := p0 + (p1-p0)*float64(cur)/float64(total)
				if done%10 == 0 || done == batchTotal {
					s.updateJob(j, model.JobRunning, prog,
						fmt.Sprintf("dial round %d/%d overall %d/%d", round, rounds, cur, total))
				}
			},
		})
		if err != nil {
			return err
		}
		d.TestAll(ctx, batch)
		s.store.UpsertNodes(batch)

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
		j.Stats["verified_total"] = len(s.store.ListNodes(store.NodeFilter{VerifiedOnly: true, Limit: 20000}))
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
	j.Stats["verified_total"] = len(s.store.ListNodes(store.NodeFilter{VerifiedOnly: true, Limit: 20000}))
	s.updateJob(j, model.JobRunning, p1, fmt.Sprintf("dial ok=%d fail=%d rounds=%d planned=%d", okN, failN, rounds, total))
	return nil
}

// pickDialCandidates 选取拨测目标。
// allHQ=true 或 maxDial<=0：取全部存活 HQ（可拨协议），按 score 排序后整表返回。
// 否则：限制数量 + 国家/协议分散，避免只测 CDN 假活。
func (s *Service) pickDialCandidates(maxDial int, onlyUnverified bool, allHQ bool) []*model.Node {
	minScore := 0.0
	if s.cfg != nil && s.cfg.Filter.MinScore > 0 {
		minScore = s.cfg.Filter.MinScore
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
	for _, n := range candidates {
		if !dialer.Supports(n) {
			continue
		}
		if onlyUnverified && n.Verified {
			continue
		}
		// 限量模式才跳过 dial-fail；全量 HQ 要把所有未 verified 的都测到
		if !allHQ && onlyUnverified && hasTag(n.Tags, "dial-fail") {
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
		if len(out) == 0 && onlyUnverified {
			return s.pickDialCandidates(0, false, true)
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
	if len(out) == 0 && onlyUnverified {
		return s.pickDialCandidates(maxDial, false, false)
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

// doPurity 对 verified 节点做纯净度 / CF 挑战探测
func (s *Service) doPurity(ctx context.Context, j *model.Job, p0, p1 float64) error {
	s.updateJob(j, model.JobRunning, p0, "purity probe starting")
	list := s.store.ListNodes(store.NodeFilter{VerifiedOnly: true, AliveOnly: true, Limit: 5000})
	if len(list) == 0 {
		// 兜底：有 dial ok 标记的
		all := s.store.ListNodes(store.NodeFilter{AliveOnly: true, HighQuality: true, Limit: 500})
		for _, n := range all {
			if n.Verified || (n.Dial != nil && n.Dial.OK) {
				list = append(list, n)
			}
		}
	}
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
	s.store.UpsertNodes(list)

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
	s.store.UpsertNodes(nodes)

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
		socks = os.Getenv("NODE_HUNTER_SOCKS5")
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
	s.store.SetHostAI(hostAI)

	// 节点探测：优先高质量/存活
	nodes := s.store.ListNodes(store.NodeFilter{AliveOnly: true, Limit: 400})
	if len(nodes) < 50 {
		nodes = s.store.ListNodes(store.NodeFilter{Limit: 200})
	}
	p.ProbeNodes(ctx, nodes)
	s.store.UpsertNodes(nodes)

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
	hq := s.SelectPublishNodes()
	paths, err := exporter.Export(hq, s.cfg)
	if err != nil {
		j.Stats["export_error"] = err.Error()
		return
	}
	j.Stats["exported"] = len(hq)
	j.Stats["export_files"] = paths

	// 预渲染订阅缓存（热路径）
	if s.cfg.Publish.PreRender {
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
		dir := s.cfg.Export.Dir
		_ = os.MkdirAll(dir, 0o755)
		p := filepath.Join(dir, "nodes-ai-friendly.txt")
		_ = os.WriteFile(p, []byte(strings.Join(aiURIs, "\n")+"\n"), 0o644)
		j.Stats["ai_export"] = p
	}
}

// SelectPublishNodes 筛选可对外发布的高质量节点
// 若存在真实拨测通过节点，优先发布 verified 池（更可信）
func (s *Service) SelectPublishNodes() []*model.Node {
	verified := s.store.ListNodes(store.NodeFilter{VerifiedOnly: true, AliveOnly: true, Limit: s.cfg.Publish.MaxNodes})
	if len(verified) >= 10 {
		// 国旗前缀
		if s.cfg.Geo.RenameWithFlag {
			for _, n := range verified {
				applyCountryNamePrefix(n)
			}
		}
		return verified
	}
	return s.SelectPublishNodesCountry("")
}

// SelectPublishNodesCountry 可按 ISO 国家码筛选（空=全部）
func (s *Service) SelectPublishNodesCountry(country string) []*model.Node {
	minScore := s.cfg.Publish.MinScore
	if minScore <= 0 {
		minScore = s.cfg.Filter.MinScore
	}
	if minScore <= 0 {
		minScore = 70
	}
	limit := s.cfg.Publish.MaxNodes
	if limit <= 0 {
		limit = s.cfg.Filter.MaxNodes
	}
	aliveOnly := true
	if s.cfg.Publish.Enabled {
		aliveOnly = s.cfg.Publish.AliveOnly
	}

	f := store.NodeFilter{
		AliveOnly: aliveOnly,
		MinScore:  minScore,
		Limit:     limit * 3,
		Country:   strings.ToUpper(strings.TrimSpace(country)),
	}
	if minScore >= 70 {
		f.HighQuality = true
	}
	hq := s.store.ListNodes(f)
	if len(hq) == 0 && country == "" {
		hq = s.store.ListNodes(store.NodeFilter{AliveOnly: true, MinScore: 50, Limit: limit * 2})
	}
	if len(hq) == 0 && country == "" {
		hq = s.store.ListNodes(store.NodeFilter{AliveOnly: true, Limit: limit})
	}
	hq = filter.Apply(hq, s.cfg)
	if limit > 0 && len(hq) > limit {
		hq = hq[:limit]
	}
	// 导出前可选国旗前缀
	if s.cfg.Geo.RenameWithFlag {
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
	minScore := s.cfg.Publish.MinScore
	if minScore <= 0 {
		minScore = s.cfg.Filter.MinScore
	}
	limit := s.cfg.Publish.MaxNodes
	if limit <= 0 {
		limit = s.cfg.Filter.MaxNodes
	}
	return store.NodeFilter{
		AliveOnly:   s.cfg.Publish.AliveOnly,
		MinScore:    minScore,
		Limit:       limit,
		HighQuality: minScore >= 70,
	}
}

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
}

func (s *Service) fail(j *model.Job, err error) {
	now := time.Now()
	j.EndedAt = &now
	j.Error = err.Error()
	s.updateJob(j, model.JobFailed, j.Progress, err.Error())
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
