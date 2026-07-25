package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/local/node-hunter/internal/auth"
	"github.com/local/node-hunter/internal/dialer"
	"github.com/local/node-hunter/internal/exporter"
	"github.com/local/node-hunter/internal/geo"
	"github.com/local/node-hunter/internal/metrics"
	"github.com/local/node-hunter/internal/middleware"
	"github.com/local/node-hunter/internal/model"
	"github.com/local/node-hunter/internal/pools"
	"github.com/local/node-hunter/internal/publish"
	"github.com/local/node-hunter/internal/scheduler"
	"github.com/local/node-hunter/internal/service"
	"github.com/local/node-hunter/internal/store"
	"github.com/local/node-hunter/internal/timex"
	"github.com/local/node-hunter/internal/version"
)

// Server HTTP API
type Server struct {
	svc    *service.Service
	sch    *scheduler.Scheduler
	auth   *auth.Manager
	mux    *http.ServeMux
	subRL  *middleware.TokenBucket
	started time.Time
}

func New(svc *service.Service, sch *scheduler.Scheduler, am *auth.Manager) *Server {
	cfg := svc.Config()
	s := &Server{
		svc:     svc,
		sch:     sch,
		auth:    am,
		mux:     http.NewServeMux(),
		subRL:   middleware.NewTokenBucket(cfg.Security.SubRPS, cfg.Security.SubBurst),
		started: time.Now(),
	}
	if s.auth == nil {
		s.auth = &auth.Manager{
			MasterToken:       cfg.Publish.Token,
			AdminToken:        cfg.Security.AdminToken,
			QueryTokenAllowed: cfg.Security.AllowQueryToken,
			DB:                svc.DB(),
		}
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = cors(h)
	// 全局限流（API 稍宽）
	apiRL := middleware.NewTokenBucket(s.svc.Config().Security.APIRPS, s.svc.Config().Security.APIBurst)
	h = apiRL.Middleware(h)
	h = middleware.AccessLog(h)
	return h
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/ready", s.handleReady)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/nodes", s.handleNodes)
	s.mux.HandleFunc("GET /api/nodes/{id}", s.handleNode)
	s.mux.HandleFunc("GET /api/jobs", s.handleJobs)
	s.mux.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	s.mux.HandleFunc("POST /api/jobs/fetch", s.handleStartFetch)
	s.mux.HandleFunc("POST /api/jobs/quality", s.handleStartQuality)
	s.mux.HandleFunc("POST /api/jobs/ai", s.handleStartAI)
	s.mux.HandleFunc("POST /api/jobs/full", s.handleStartFull)
	s.mux.HandleFunc("POST /api/jobs/geo", s.handleStartGeo)
	s.mux.HandleFunc("POST /api/jobs/dial", s.handleStartDial)
	s.mux.HandleFunc("POST /api/jobs/purity", s.handleStartPurity)
	s.mux.HandleFunc("POST /api/jobs/cancel", s.handleCancelJob)
	s.mux.HandleFunc("GET /api/dial/status", s.handleDialStatus)
	s.mux.HandleFunc("GET /api/purity/summary", s.handlePuritySummary)
	s.mux.HandleFunc("GET /api/countries", s.handleCountries)
	s.mux.HandleFunc("GET /api/sources", s.handleSources)
	s.mux.HandleFunc("GET /api/ai/targets", s.handleAITargets)
	s.mux.HandleFunc("GET /api/ai/host", s.handleHostAI)
	s.mux.HandleFunc("GET /api/export/raw", s.handleExportRaw)
	s.mux.HandleFunc("GET /api/export/base64", s.handleExportBase64)
	s.mux.HandleFunc("GET /api/export/clash", s.handleExportClash)
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/schedule", s.handleSchedule)
	s.mux.HandleFunc("GET /api/pools", s.handlePools)
	s.mux.HandleFunc("GET /api/pools/{key}/export/raw", s.handlePoolExportRaw)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)

	// admin (token required)
	s.mux.HandleFunc("GET /api/admin/tokens", s.handleAdminListTokens)
	s.mux.HandleFunc("POST /api/admin/tokens", s.handleAdminCreateToken)
	s.mux.HandleFunc("DELETE /api/admin/tokens/{id}", s.handleAdminDeleteToken)
	s.mux.HandleFunc("POST /api/admin/tokens/{id}/enable", s.handleAdminEnableToken)
	s.mux.HandleFunc("POST /api/admin/tokens/{id}/disable", s.handleAdminDisableToken)
	s.mux.HandleFunc("GET /api/admin/audit", s.handleAdminAudit)
	s.mux.HandleFunc("POST /api/admin/publish/refresh", s.handleAdminRefreshPublish)

	s.mountPublish("/sub")
	s.mountPublish("/api/sub")
	if p := s.svc.Config().Publish.PathPrefix; p != "" && p != "/sub" && p != "/api/sub" {
		s.mountPublish(p)
	}
}

func (s *Server) mountPublish(prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		prefix = "/sub"
	}
	s.mux.HandleFunc("GET "+prefix, s.handleSubIndex)
	s.mux.HandleFunc("GET "+prefix+"/", s.handleSubIndex)
	s.mux.HandleFunc("GET "+prefix+"/raw", s.handleSubRaw)
	s.mux.HandleFunc("GET "+prefix+"/base64", s.handleSubBase64)
	s.mux.HandleFunc("GET "+prefix+"/clash", s.handleSubClash)
	s.mux.HandleFunc("GET "+prefix+"/clash.yaml", s.handleSubClash)
	s.mux.HandleFunc("GET "+prefix+"/meta", s.handleSubMeta)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	geoReady := s.svc.Geo() != nil && s.svc.Geo().Ready()
	dbOK := true
	if s.svc.DB() != nil {
		dbOK = s.svc.DB().Ping() == nil
	}
	blob := s.svc.PublishCache().Get()
	pubCount := 0
	pubAt := ""
	if blob != nil {
		pubCount = blob.Count
		pubAt = blob.UpdatedAt
	}
	writeJSON(w, map[string]any{
		"ok":      true,
		"time":    timex.NowRFC3339(),
		"tz":      "Asia/Shanghai",
		"version": version.Version,
		"uptime_sec": int(version.Uptime().Seconds()),
		"nodes":   s.svc.Store().Count(),
		"running_job": s.svc.IsRunning(),
		"geo_mmdb": geoReady,
		"sqlite":  dbOK && cfg.SQLite.Enabled,
		"publish_count": pubCount,
		"publish_updated_at": pubAt,
		"schedule": cfg.Schedule.Enabled,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// 磁盘可写 + 可选 sqlite
	ready := true
	reasons := []string{}
	if s.svc.Config().SQLite.Enabled && s.svc.DB() != nil {
		if err := s.svc.DB().Ping(); err != nil {
			ready = false
			reasons = append(reasons, "sqlite: "+err.Error())
		}
	}
	// export dir
	// soft: geo not required for ready
	code := http.StatusOK
	if !ready {
		code = http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	writeJSON(w, map[string]any{"ready": ready, "reasons": reasons, "time": timex.NowRFC3339()})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, version.Info())
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.svc.Config().Server.EnableMetrics {
		http.NotFound(w, r)
		return
	}
	// refresh gauges
	st := s.svc.Store().Stats(len(s.svc.Config().EnabledSources()))
	s.svc.Metrics().SetNodes(st.TotalNodes, st.AliveNodes, st.HighQuality)
	s.svc.Metrics().Handler().ServeHTTP(w, r)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st := s.svc.Store().Stats(len(s.svc.Config().EnabledSources()))
	writeJSON(w, st)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	minScore, _ := strconv.ParseFloat(q.Get("min_score"), 64)
	f := store.NodeFilter{
		Protocol:    q.Get("protocol"),
		Source:      q.Get("source"),
		Grade:       q.Get("grade"),
		Country:     q.Get("country"),
		AliveOnly:   q.Get("alive") == "1" || q.Get("alive") == "true",
		MinScore:    minScore,
		AIKey:       q.Get("ai"),
		Search:      q.Get("q"),
		Limit:       limit,
		HighQuality:  q.Get("hq") == "1" || q.Get("hq") == "true",
		VerifiedOnly: q.Get("verified") == "1" || q.Get("verified") == "true",
	}
	nodes := s.svc.Store().ListNodes(f)
	type nodeDTO struct {
		*model.Node
		LatencyMS int64 `json:"latency_ms"`
	}
	list := make([]nodeDTO, 0, len(nodes))
	for _, n := range nodes {
		list = append(list, nodeDTO{Node: n, LatencyMS: n.LatencyMS()})
	}
	writeJSON(w, map[string]any{"total": len(list), "nodes": list})
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, ok := s.svc.Store().GetNode(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, n)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	// prefer sqlite history if available
	if s.svc.DB() != nil {
		if jobs, err := s.svc.DB().ListJobs(30); err == nil && len(jobs) > 0 {
			writeJSON(w, jobs)
			return
		}
	}
	writeJSON(w, s.svc.Store().ListJobs(30))
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.svc.Store().GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, j)
}

func (s *Server) handleStartFetch(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartFetch)
}
func (s *Server) handleStartQuality(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartQuality)
}
func (s *Server) handleStartAI(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartAI)
}
func (s *Server) handleStartFull(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartFull)
}
func (s *Server) handleStartGeo(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartGeo)
}
func (s *Server) handleStartDial(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartDial)
}
func (s *Server) handleStartPurity(w http.ResponseWriter, r *http.Request) {
	s.startJob(w, r, s.svc.StartPurity)
}

func (s *Server) handlePuritySummary(w http.ResponseWriter, r *http.Request) {
	nodes := s.svc.Store().ListNodes(store.NodeFilter{VerifiedOnly: true, Limit: 5000})
	type row struct {
		ID            string  `json:"id"`
		Name          string  `json:"name"`
		Country       string  `json:"country"`
		Protocol      string  `json:"protocol"`
		ExitIP        string  `json:"exit_ip,omitempty"`
		CleanScore    int     `json:"clean_score"`
		RiskScore     int     `json:"risk_score"`
		Grade         string  `json:"grade,omitempty"`
		IsProxy       bool    `json:"is_proxy"`
		IsHosting     bool    `json:"is_hosting"`
		CFChallenge   string  `json:"cf_challenge,omitempty"`
		CFHumanLikely bool    `json:"cf_human_likely"`
		ISP           string  `json:"isp,omitempty"`
	}
	out := make([]row, 0, len(nodes))
	byGrade := map[string]int{}
	cfOK, clean70, withPurity := 0, 0, 0
	var sum int
	for _, n := range nodes {
		if n.Purity == nil {
			continue
		}
		withPurity++
		sum += n.Purity.CleanScore
		if n.Purity.Grade != "" {
			byGrade[n.Purity.Grade]++
		}
		if n.Purity.CFHumanLikely {
			cfOK++
		}
		if n.Purity.CleanScore >= 70 {
			clean70++
		}
		out = append(out, row{
			ID: n.ID, Name: n.Name, Country: n.Country, Protocol: string(n.Protocol),
			ExitIP: n.Purity.ExitIP, CleanScore: n.Purity.CleanScore, RiskScore: n.Purity.RiskScore,
			Grade: n.Purity.Grade, IsProxy: n.Purity.IsProxy, IsHosting: n.Purity.IsHosting,
			CFChallenge: n.Purity.CFChallenge, CFHumanLikely: n.Purity.CFHumanLikely, ISP: n.Purity.ISP,
		})
	}
	// 按 clean_score 降序
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CleanScore > out[i].CleanScore {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	avg := 0
	if withPurity > 0 {
		avg = sum / withPurity
	}
	writeJSON(w, map[string]any{
		"verified_total":  len(nodes),
		"purity_tested":   withPurity,
		"avg_clean_score": avg,
		"cf_human_likely": cfOK,
		"clean_score_ge70": clean70,
		"by_grade":        byGrade,
		"nodes":           out,
		"disclaimer":      "CF human check is heuristic (challenge page detection), not real Turnstile solve. Scores use ip-api proxy/hosting flags + CF signals.",
	})
}

func (s *Server) handleDialStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	status := map[string]any{
		"enabled":            cfg.Dial.Enabled,
		"engine":             cfg.Dial.Engine,
		"bin":                cfg.Dial.Bin,
		"concurrency":        cfg.Dial.Concurrency,
		"timeout_sec":        cfg.Dial.TimeoutSec,
		"test_url":           cfg.Dial.TestURL,
		"max_nodes":          cfg.Dial.MaxNodes, // 0=全部 HQ
		"batch_size":         cfg.Dial.BatchSize,
		"after_quality":      cfg.Dial.AfterQuality,
		"after_quality_max":  cfg.Dial.AfterQualityMax, // 0=全部 HQ
		"all_hq":             cfg.Dial.MaxNodes <= 0 || cfg.Dial.AfterQualityMax <= 0,
	}
	vn := s.svc.Store().ListNodes(store.NodeFilter{VerifiedOnly: true, Limit: 5000})
	status["verified_count"] = len(vn)
	if bin, eng, err := dialer.Available(); err == nil {
		status["available"] = true
		status["bin_resolved"] = bin
		status["engine_resolved"] = eng
	} else {
		status["available"] = false
		status["error"] = err.Error()
		status["hint"] = dialer.InstallHint()
	}
	writeJSON(w, status)
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request, fn func(map[string]any) (*model.Job, error)) {
	opts := readOpts(r)
	j, err := fn(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, j)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	ok := s.svc.CancelJob()
	writeJSON(w, map[string]any{"canceled": ok})
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	type src struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		URL     string `json:"url"`
		Enabled bool   `json:"enabled"`
	}
	list := make([]src, 0, len(cfg.Sources))
	for _, s0 := range cfg.Sources {
		list = append(list, src{Name: s0.Name, Type: s0.Type, URL: s0.URL, Enabled: s0.Enabled})
	}
	writeJSON(w, list)
}

func (s *Server) handleAITargets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, model.DefaultAITargets())
}
func (s *Server) handleHostAI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.svc.Store().HostAI())
}

func (s *Server) handleExportRaw(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodesFromExportQuery(r)
	body := exporter.RenderRaw(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.txt")
	_, _ = w.Write([]byte(body))
}
func (s *Server) handleExportBase64(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodesFromExportQuery(r)
	enc := exporter.RenderBase64(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.base64.txt")
	_, _ = w.Write([]byte(enc))
}
func (s *Server) handleExportClash(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodesFromExportQuery(r)
	body := exporter.RenderClash(nodes)
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.clash.yaml")
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	writeJSON(w, map[string]any{
		"concurrency":         cfg.App.Concurrency,
		"fetch_timeout":       cfg.App.FetchTimeoutSec,
		"test_timeout":        cfg.App.TestTimeoutSec,
		"max_latency_ms":      cfg.Filter.MaxLatencyMS,
		"max_nodes":           cfg.Filter.MaxNodes,
		"min_score":           cfg.Filter.MinScore,
		"prune_after_quality": cfg.Filter.PruneAfterQuality,
		"protocols":           cfg.Protocols,
		"sources_enabled":     len(cfg.EnabledSources()),
		"sources_total":       len(cfg.Sources),
		"version":             version.Version,
		"schedule": map[string]any{
			"enabled":      cfg.Schedule.Enabled,
			"interval_min": cfg.Schedule.IntervalMin,
			"job":          cfg.Schedule.Job,
			"skip_ai":      cfg.Schedule.SkipAI,
		},
		"publish": map[string]any{
			"enabled":      cfg.Publish.Enabled,
			"path_prefix":  cfg.Publish.PathPrefix,
			"token_set":    cfg.Publish.Token != "",
			"min_score":    cfg.Publish.MinScore,
			"max_nodes":    cfg.Publish.MaxNodes,
			"public_url":   cfg.Publish.PublicURL,
			"formats":      cfg.Publish.Formats,
			"pre_render":   cfg.Publish.PreRender,
			"cache_sec":    cfg.Publish.CacheSec,
		},
		"security": map[string]any{
			"allow_query_token": cfg.Security.AllowQueryToken,
			"sub_rps":           cfg.Security.SubRPS,
			"admin_token_set":   cfg.Security.AdminToken != "" || cfg.Publish.Token != "",
		},
		"sqlite": cfg.SQLite.Enabled,
		"geo": map[string]any{
			"enabled": cfg.Geo.Enabled,
			"mmdb":    s.svc.Geo() != nil && s.svc.Geo().Ready(),
		},
	})
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if s.sch == nil {
		writeJSON(w, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, s.sch.Status())
}

func (s *Server) handlePools(w http.ResponseWriter, r *http.Request) {
	minScore := s.svc.Config().Filter.MinScore
	list := pools.Defaults(minScore)
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		nodes := pools.Select(s.svc.Store(), p)
		out = append(out, map[string]any{
			"key": p.Key, "title": p.Title, "description": p.Description, "count": len(nodes),
		})
	}
	writeJSON(w, out)
}

func (s *Server) handlePoolExportRaw(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	p, ok := pools.Find(key, s.svc.Config().Filter.MinScore)
	if !ok {
		http.Error(w, "unknown pool", 404)
		return
	}
	nodes := pools.Select(s.svc.Store(), p)
	body := exporter.RenderRaw(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleCountries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hq := q.Get("hq") == "1" || q.Get("hq") == "true" || q.Get("hq") == ""
	alive := q.Get("alive") != "0"
	minScore := s.svc.Config().Filter.MinScore
	if minScore <= 0 {
		minScore = 70
	}
	if v := q.Get("min_score"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			minScore = f
		}
	}
	nodes := s.svc.Store().ListNodes(store.NodeFilter{
		AliveOnly: alive, HighQuality: hq, MinScore: minScore, Limit: 5000,
	})
	type row struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Flag    string `json:"flag"`
		Count   int    `json:"count"`
		Display string `json:"display"`
	}
	counts := map[string]int{}
	for _, n := range nodes {
		cc := n.Country
		if cc == "" {
			cc = "XX"
		}
		if cc == "UK" {
			cc = "GB"
		}
		counts[cc]++
	}
	list := make([]row, 0, len(counts))
	for code, c := range counts {
		list = append(list, row{
			Code: code, Name: geo.ISOToName(code), Flag: geo.FlagEmoji(code),
			Count: c, Display: countryDisplay(code),
		})
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].Count > list[i].Count {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	writeJSON(w, map[string]any{
		"total_countries": len(list),
		"total_nodes":     len(nodes),
		"min_score":       minScore,
		"hq":              hq,
		"countries":       list,
	})
}

func countryDisplay(code string) string {
	if code == "" || code == "XX" {
		return "🌐 Unknown"
	}
	return geo.DisplayName(code)
}

// ---- publish ----

func (s *Server) extractToken(r *http.Request) string {
	if s.svc.Config().Security.AllowQueryToken {
		if t := r.URL.Query().Get("token"); t != "" {
			return t
		}
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if strings.HasPrefix(strings.ToLower(h), "token ") {
		return strings.TrimSpace(h[6:])
	}
	if t := r.Header.Get("X-Sub-Token"); t != "" {
		return t
	}
	// 若禁用 query 仍兼容：仅当 allow 时已返回；此处再读 query 仅当 master empty? no
	if s.svc.Config().Security.AllowQueryToken {
		return r.URL.Query().Get("token")
	}
	// still accept query for backward compat if header missing and allow false? 按配置严格
	return ""
}

func (s *Server) ensurePublish(w http.ResponseWriter, r *http.Request) (*auth.Principal, bool) {
	cfg := s.svc.Config()
	if !cfg.Publish.Enabled {
		http.Error(w, "publish disabled", http.StatusNotFound)
		return nil, false
	}
	// 订阅限流更严
	key := middleware.ClientIP(r)
	if t := s.extractToken(r); t != "" {
		if len(t) > 8 {
			key += "|" + t[:8]
		}
	}
	if !s.subRL.Allow(key) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return nil, false
	}
	p, err := s.auth.ValidateSubToken(s.extractToken(r))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return p, true
}

func (s *Server) serveSubFormat(w http.ResponseWriter, r *http.Request, format, filename, contentType string) {
	start := time.Now()
	p, ok := s.ensurePublish(w, r)
	if !ok {
		s.svc.Metrics().IncSub(format, http.StatusUnauthorized)
		return
	}
	country := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country")))
	if country == "UK" {
		country = "GB"
	}
	// token 国家 ACL
	if p != nil && len(p.AllowCountries) > 0 && country != "" {
		allowed := false
		for _, c := range p.AllowCountries {
			if strings.EqualFold(c, country) {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "country not allowed for token", http.StatusForbidden)
			s.svc.Metrics().IncSub(format, http.StatusForbidden)
			return
		}
	}

	var body string
	var count int
	etag := ""
	updated := ""
	blob := s.svc.PublishCache().Get()
	if blob != nil {
		if b, c, ok := blob.Format(format, country); ok {
			body, count, etag, updated = b, c, blob.ETag, blob.UpdatedAt
		}
	}
	// cache miss or country not pre-rendered → live select
	if body == "" {
		nodes := s.svc.SelectPublishNodesCountry(country)
		// filter by token countries if no country param
		if p != nil && len(p.AllowCountries) > 0 && country == "" {
			allow := map[string]bool{}
			for _, c := range p.AllowCountries {
				allow[strings.ToUpper(c)] = true
			}
			filtered := nodes[:0]
			for _, n := range nodes {
				cc := n.Country
				if cc == "" {
					cc = "XX"
				}
				if allow[strings.ToUpper(cc)] {
					filtered = append(filtered, n)
				}
			}
			nodes = filtered
		}
		switch format {
		case "raw":
			body = exporter.RenderRaw(nodes)
		case "base64":
			body = exporter.RenderBase64(nodes)
		case "clash":
			body = exporter.RenderClash(nodes)
		}
		count = len(nodes)
		updated = timex.NowRFC3339()
	}

	if inm := r.Header.Get("If-None-Match"); inm != "" && etag != "" && inm == etag {
		w.WriteHeader(http.StatusNotModified)
		s.svc.Metrics().IncSub(format, 304)
		s.svc.Metrics().ObserveSubLatency(time.Since(start).Seconds())
		return
	}
	s.setSubHeaders(w, contentType, filename, etag)
	if updated != "" {
		w.Header().Set("X-Updated-At", updated)
	}
	w.Header().Set("X-Node-Count", strconv.Itoa(count))
	_, _ = w.Write([]byte(body))
	s.svc.Metrics().IncSub(format, 200)
	s.svc.Metrics().ObserveSubLatency(time.Since(start).Seconds())
}

func (s *Server) handleSubRaw(w http.ResponseWriter, r *http.Request) {
	s.serveSubFormat(w, r, "raw", "sub.txt", "text/plain; charset=utf-8")
}
func (s *Server) handleSubBase64(w http.ResponseWriter, r *http.Request) {
	s.serveSubFormat(w, r, "base64", "sub.base64", "text/plain; charset=utf-8")
}
func (s *Server) handleSubClash(w http.ResponseWriter, r *http.Request) {
	s.serveSubFormat(w, r, "clash", "clash.yaml", "text/yaml; charset=utf-8")
}

func (s *Server) handleSubMeta(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ensurePublish(w, r); !ok {
		return
	}
	country := r.URL.Query().Get("country")
	blob := s.svc.PublishCache().Get()
	if blob != nil && country == "" {
		writeJSON(w, map[string]any{
			"count": blob.Count, "by_country": blob.ByCountry, "etag": blob.ETag,
			"generated": blob.UpdatedAt, "min_score": s.svc.Config().Publish.MinScore,
			"max_nodes": s.svc.Config().Publish.MaxNodes, "cached": true,
		})
		return
	}
	nodes := s.svc.SelectPublishNodesCountry(country)
	by := map[string]int{}
	for _, n := range nodes {
		cc := n.Country
		if cc == "" {
			cc = "XX"
		}
		by[cc]++
	}
	writeJSON(w, map[string]any{
		"count": len(nodes), "by_country": by, "country": country,
		"generated": timex.NowRFC3339(), "cached": false,
		"min_score": s.svc.Config().Publish.MinScore, "max_nodes": s.svc.Config().Publish.MaxNodes,
	})
}

func (s *Server) handleSubIndex(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ensurePublish(w, r); !ok {
		return
	}
	cfg := s.svc.Config()
	base := cfg.Publish.PublicURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
		base = scheme + "://" + host
	}
	prefix := cfg.Publish.PathPrefix
	if prefix == "" {
		prefix = "/sub"
	}
	tokenQ := ""
	if t := s.extractToken(r); t != "" && cfg.Security.AllowQueryToken {
		tokenQ = "?token=" + t
	} else if cfg.Publish.Token != "" && cfg.Security.AllowQueryToken {
		// 不回显 master token 明文到 JSON 若请求未带 — 仅当请求已带
	}
	// 若请求带了 token，链接回填（方便客户端）
	if t := s.extractToken(r); t != "" && cfg.Security.AllowQueryToken {
		tokenQ = "?token=" + t
	}
	joinQ := func(extra string) string {
		if extra == "" {
			return tokenQ
		}
		if tokenQ == "" {
			return "?" + extra
		}
		return tokenQ + "&" + extra
	}

	blob := s.svc.PublishCache().Get()
	count := 0
	byCountry := map[string]int{}
	updated := timex.NowRFC3339()
	if blob != nil {
		count = blob.Count
		byCountry = blob.ByCountry
		updated = blob.UpdatedAt
	} else {
		nodes := s.svc.SelectPublishNodes()
		count = len(nodes)
		for _, n := range nodes {
			cc := n.Country
			if cc == "" {
				cc = "XX"
			}
			byCountry[cc]++
		}
	}

	type clink struct {
		Code  string `json:"code"`
		Count int    `json:"count"`
		Raw   string `json:"raw"`
		B64   string `json:"base64"`
		Clash string `json:"clash"`
	}
	var countryLinks []clink
	for code, c := range byCountry {
		if code == "XX" || c < 1 {
			continue
		}
		q := joinQ("country=" + code)
		countryLinks = append(countryLinks, clink{
			Code: code, Count: c,
			Raw: base + prefix + "/raw" + q, B64: base + prefix + "/base64" + q, Clash: base + prefix + "/clash" + q,
		})
	}
	for i := 0; i < len(countryLinks); i++ {
		for j := i + 1; j < len(countryLinks); j++ {
			if countryLinks[j].Count > countryLinks[i].Count {
				countryLinks[i], countryLinks[j] = countryLinks[j], countryLinks[i]
			}
		}
	}
	if len(countryLinks) > 20 {
		countryLinks = countryLinks[:20]
	}
	links := map[string]string{
		"raw": base + prefix + "/raw" + tokenQ, "base64": base + prefix + "/base64" + tokenQ,
		"clash": base + prefix + "/clash" + tokenQ,
	}
	writeJSON(w, map[string]any{
		"ok": true, "nodes": count, "min_score": cfg.Publish.MinScore, "max_nodes": cfg.Publish.MaxNodes,
		"alive_only": cfg.Publish.AliveOnly, "by_country": byCountry, "updated_at": updated,
		"tz": "Asia/Shanghai", "links": links, "by_country_links": countryLinks,
		"cached": blob != nil,
		"usage": map[string]string{
			"v2rayN": links["base64"], "clash": links["clash"], "raw_uri": links["raw"],
			"country_example": base + prefix + "/base64" + joinQ("country=US"),
		},
	})
}

func (s *Server) setSubHeaders(w http.ResponseWriter, contentType, filename, etag string) {
	cfg := s.svc.Config()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))
	w.Header().Set("X-Node-Hunter", "publish")
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if cfg.Publish.CacheSec > 0 {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cfg.Publish.CacheSec))
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.Header().Set("Profile-Update-Interval", "6")
}

// ---- admin ----

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	tok := s.extractToken(r)
	if tok == "" {
		tok = r.Header.Get("X-Admin-Token")
	}
	if !s.auth.ValidateAdmin(tok) {
		http.Error(w, "admin unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) handleAdminListTokens(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := s.svc.DB().ListTokens()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleAdminCreateToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Name      string   `json:"name"`
		Note      string   `json:"note"`
		Countries []string `json:"countries"`
		Days      int      `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = "token"
	}
	t, err := s.auth.CreateToken(body.Name, body.Note, body.Countries, body.Days)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleAdminDeleteToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if s.svc.DB() == nil {
		http.Error(w, "db disabled", 500)
		return
	}
	if err := s.svc.DB().DeleteToken(id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.svc.DB().Audit("admin", "token.delete", id)
	writeJSON(w, map[string]any{"deleted": id})
}

func (s *Server) handleAdminEnableToken(w http.ResponseWriter, r *http.Request) {
	s.setTokenEnabled(w, r, true)
}
func (s *Server) handleAdminDisableToken(w http.ResponseWriter, r *http.Request) {
	s.setTokenEnabled(w, r, false)
}
func (s *Server) setTokenEnabled(w http.ResponseWriter, r *http.Request, en bool) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if s.svc.DB() == nil {
		http.Error(w, "db disabled", 500)
		return
	}
	if err := s.svc.DB().SetTokenEnabled(id, en); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"id": id, "enabled": en})
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := s.svc.DB().ListAudit(100)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleAdminRefreshPublish(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	blob := s.svc.RefreshPublishCache()
	writeJSON(w, map[string]any{
		"ok": true, "count": blob.Count, "etag": blob.ETag, "updated_at": blob.UpdatedAt,
	})
}

func (s *Server) nodesFromExportQuery(r *http.Request) []*model.Node {
	f := exportFilter(r)
	nodes := s.svc.Store().ListNodes(f)
	if len(nodes) == 0 {
		return s.svc.SelectPublishNodes()
	}
	return nodes
}

func exportFilter(r *http.Request) store.NodeFilter {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 500
	}
	minScore, _ := strconv.ParseFloat(q.Get("min_score"), 64)
	if minScore == 0 && (q.Get("hq") == "1" || q.Get("hq") == "true") {
		minScore = 70
	}
	return store.NodeFilter{
		Protocol: q.Get("protocol"), Country: q.Get("country"),
		AliveOnly: q.Get("alive") != "0", MinScore: minScore, AIKey: q.Get("ai"),
		Limit: limit, HighQuality: q.Get("hq") == "1",
	}
}

func readOpts(r *http.Request) map[string]any {
	opts := map[string]any{}
	if r.Body == nil {
		return opts
	}
	defer r.Body.Close()
	_ = json.NewDecoder(r.Body).Decode(&opts)
	return opts
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Sub-Token, X-Admin-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// silence unused import if metrics only via handler
var _ = metrics.New
var _ = publish.NewCache
