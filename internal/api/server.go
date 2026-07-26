package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/GALIAIS/NodeHarvest/internal/auth"
	"github.com/GALIAIS/NodeHarvest/internal/config"
	"github.com/GALIAIS/NodeHarvest/internal/db"
	"github.com/GALIAIS/NodeHarvest/internal/dialer"
	"github.com/GALIAIS/NodeHarvest/internal/exporter"
	"github.com/GALIAIS/NodeHarvest/internal/geo"
	"github.com/GALIAIS/NodeHarvest/internal/middleware"
	"github.com/GALIAIS/NodeHarvest/internal/model"
	"github.com/GALIAIS/NodeHarvest/internal/pools"
	"github.com/GALIAIS/NodeHarvest/internal/publish"
	"github.com/GALIAIS/NodeHarvest/internal/scheduler"
	"github.com/GALIAIS/NodeHarvest/internal/service"
	"github.com/GALIAIS/NodeHarvest/internal/store"
	"github.com/GALIAIS/NodeHarvest/internal/timex"
	"github.com/GALIAIS/NodeHarvest/internal/version"
)

// Server HTTP API
type Server struct {
	svc     *service.Service
	sch     *scheduler.Scheduler
	auth    *auth.Manager
	mux     *http.ServeMux
	handler http.Handler
	subRL   *middleware.TokenBucket
	loginRL *middleware.TokenBucket
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
		loginRL: middleware.NewTokenBucket(cfg.Security.LoginRPS, cfg.Security.LoginBurst),
		started: time.Now(),
	}
	if s.auth == nil {
		s.auth = &auth.Manager{
			MasterToken: cfg.Publish.Token,
			AdminToken:  cfg.Security.AdminToken,
			DB:          svc.DB(),
		}
	}
	s.routes()
	s.handler = s.buildHandler()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) buildHandler() http.Handler {
	cfg := s.svc.Config()
	var h http.Handler = s.mux
	h = managementBoundary(h, cfg)
	h = cors(h, cfg.Server.AllowedOrigins)
	// 全局限流（API 稍宽）
	apiRL := middleware.NewTokenBucket(cfg.Security.APIRPS, cfg.Security.APIBurst)
	h = apiRL.Middleware(h, cfg.Server.TrustedProxies)
	h = middleware.AccessLog(h, cfg.Server.TrustedProxies, s.svc.Metrics().IncHTTP)
	return h
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/ready", s.handleReady)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/terms", s.handleTerms)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/auth/me", s.handleMe)
	s.mux.HandleFunc("GET /api/v1/auth/oidc/start", s.handleOIDCStart)
	s.mux.HandleFunc("GET /api/v1/auth/oidc/callback", s.handleOIDCCallback)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/nodes", s.handleNodes)
	s.mux.HandleFunc("GET /api/nodes/{id}", s.handleNode)
	s.mux.HandleFunc("GET /api/nodes/{id}/metrics", s.handleNodeMetrics)
	s.mux.HandleFunc("GET /api/stats/trends", s.handleTrends)
	s.mux.HandleFunc("GET /api/jobs", s.handleJobs)
	s.mux.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	s.mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
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
	if s.svc.Config().Server.EnablePprof {
		s.mux.HandleFunc("GET /debug/pprof/", s.adminOnly(httppprof.Index))
		s.mux.HandleFunc("GET /debug/pprof/cmdline", s.adminOnly(httppprof.Cmdline))
		s.mux.HandleFunc("GET /debug/pprof/profile", s.adminOnly(httppprof.Profile))
		s.mux.HandleFunc("GET /debug/pprof/symbol", s.adminOnly(httppprof.Symbol))
		s.mux.HandleFunc("GET /debug/pprof/trace", s.adminOnly(httppprof.Trace))
	}

	// admin (token required)
	s.mux.HandleFunc("GET /api/admin/tokens", s.handleAdminListTokens)
	s.mux.HandleFunc("POST /api/admin/tokens", s.handleAdminCreateToken)
	s.mux.HandleFunc("DELETE /api/admin/tokens/{id}", s.handleAdminDeleteToken)
	s.mux.HandleFunc("POST /api/admin/tokens/{id}/enable", s.handleAdminEnableToken)
	s.mux.HandleFunc("POST /api/admin/tokens/{id}/disable", s.handleAdminDisableToken)
	s.mux.HandleFunc("GET /api/admin/audit", s.handleAdminAudit)
	s.mux.HandleFunc("POST /api/admin/publish/refresh", s.handleAdminRefreshPublish)
	s.mux.HandleFunc("POST /api/admin/sources/{name}/enable", s.handleAdminEnableSource)
	s.mux.HandleFunc("POST /api/admin/sources/{name}/disable", s.handleAdminDisableSource)
	s.mux.HandleFunc("POST /api/admin/sources/{name}/probe", s.handleAdminProbeSource)
	s.mux.HandleFunc("GET /api/admin/tasks", s.handleAdminTasks)
	s.mux.HandleFunc("POST /api/admin/tasks/{id}/cancel", s.handleAdminCancelTask)
	s.mux.HandleFunc("GET /api/admin/queue", s.handleAdminQueue)
	s.mux.HandleFunc("GET /api/admin/users", s.handleAdminListUsers)
	s.mux.HandleFunc("POST /api/admin/users", s.handleAdminCreateUser)
	s.mux.HandleFunc("POST /api/admin/users/{id}/enable", s.handleAdminEnableUser)
	s.mux.HandleFunc("POST /api/admin/users/{id}/disable", s.handleAdminDisableUser)
	s.mux.HandleFunc("PATCH /api/admin/config", s.handleAdminUpdateConfig)
	s.mux.HandleFunc("GET /api/admin/config/versions", s.handleAdminConfigVersions)
	s.mux.HandleFunc("GET /api/admin/alerts", s.handleAdminAlerts)
	s.mux.HandleFunc("POST /api/admin/alerts/{id}/acknowledge", s.handleAdminAcknowledgeAlert)
	s.mux.HandleFunc("POST /api/admin/alerts/{id}/resolve", s.handleAdminResolveAlert)

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
	s.mux.HandleFunc("GET "+prefix+"/pool/{key}/{format}", s.handleSubPool)
	s.mux.HandleFunc("GET "+prefix+"/country/{code}/{format}", s.handleSubCountry)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	geoReady := s.svc.Geo() != nil && s.svc.Geo().Ready()
	dbOK := true
	if s.svc.DB() != nil {
		dbOK = s.svc.DB().Ping() == nil
	}
	redisOK := true
	if s.svc.HotCache() != nil {
		redisOK = s.svc.HotCache().Ping(r.Context()) == nil
	}
	blob := s.svc.PublishCache().Get()
	pubCount := 0
	pubAt := ""
	pubFresh := false
	if blob != nil {
		pubCount = blob.Count
		pubAt = blob.UpdatedAt
		pubFresh = blob.Fresh(s.publishCacheMaxAge())
	}
	sourceUnhealthy := 0
	for _, state := range s.svc.SourceStates() {
		if state.ConsecutiveFailures > 0 {
			sourceUnhealthy++
		}
	}
	writeJSON(w, map[string]any{
		"ok":                 dbOK && redisOK,
		"time":               timex.NowRFC3339(),
		"tz":                 "Asia/Shanghai",
		"version":            version.Version,
		"uptime_sec":         int(version.Uptime().Seconds()),
		"nodes":              s.svc.Store().Count(),
		"running_job":        s.svc.IsRunning(),
		"geo_mmdb":           geoReady,
		"database":           map[string]any{"driver": cfg.Database.Driver, "ok": dbOK},
		"redis":              map[string]any{"enabled": cfg.Redis.Enabled, "ok": redisOK},
		"publish_count":      pubCount,
		"publish_updated_at": pubAt,
		"publish_fresh":      pubFresh,
		"schedule":           cfg.Schedule.Enabled,
		"sources_unhealthy":  sourceUnhealthy,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ready := true
	reasons := []string{}
	if s.svc.Config().Database.Enabled {
		if s.svc.DB() == nil {
			ready = false
			reasons = append(reasons, "database: unavailable")
		} else if err := s.svc.DB().Ping(); err != nil {
			ready = false
			reasons = append(reasons, "database: "+err.Error())
		}
	}
	if s.svc.Config().Redis.Enabled {
		if s.svc.HotCache() == nil {
			ready = false
			reasons = append(reasons, "redis: unavailable")
		} else if err := s.svc.HotCache().Ping(r.Context()); err != nil {
			ready = false
			reasons = append(reasons, "redis: "+err.Error())
		}
	}
	if err := checkWritable(s.svc.Config().Export.Dir); err != nil {
		ready = false
		reasons = append(reasons, "export: "+err.Error())
	}
	code := http.StatusOK
	if !ready {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	writeJSON(w, map[string]any{"ready": ready, "reasons": reasons, "time": timex.NowRFC3339()})
}

func checkWritable(dir string) error {
	file, err := os.CreateTemp(dir, ".ready-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, version.Info())
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"title":     "nodeharvest acceptable use",
		"terms_url": s.svc.Config().Governance.TermsURL,
		"notice":    "Collected endpoints are supplied by independent third parties without availability or safety guarantees.",
		"restrictions": []string{
			"Use only where lawful and authorized.",
			"Do not use the service for abuse, intrusion, evasion, or unlawful access.",
			"Operators may disable sources, credentials, or tenants that violate this policy.",
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tenant   string `json:"tenant"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg := s.svc.Config()
	loginKey := middleware.ClientIP(r, cfg.Server.TrustedProxies) + "|" +
		strings.ToLower(strings.TrimSpace(body.Tenant)) + "|" + strings.ToLower(strings.TrimSpace(body.Username))
	if !s.loginRL.Allow(loginKey) {
		w.Header().Set("Retry-After", "5")
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}
	principal, session, err := s.auth.LoginLocal(body.Tenant, body.Username, body.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	s.setSessionCookie(w, r, session)
	writeJSON(w, map[string]any{"authenticated": true, "principal": principal})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// #nosec G124 -- HttpOnly and SameSite are fixed; Secure follows the TLS/proxy scheme for localhost development.
	http.SetCookie(w, &http.Cookie{
		Name: s.auth.CookieName, Value: "", Path: "/", HttpOnly: true, Secure: requestHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	writeJSON(w, map[string]any{"authenticated": false})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, err := s.auth.RequestPrincipal(r)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"authenticated": principal.Authenticated,
		"principal":     principal,
		"oidc_enabled":  s.auth.OIDCEnabled(),
		"local_enabled": s.svc.Config().Auth.LocalEnabled,
	})
}

func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if !s.auth.OIDCEnabled() {
		http.Error(w, "OIDC is disabled", http.StatusNotFound)
		return
	}
	state, _, _, err := auth.NewTokenPlain()
	if err != nil {
		http.Error(w, "entropy unavailable", http.StatusInternalServerError)
		return
	}
	nonce, _, _, err := auth.NewTokenPlain()
	if err != nil {
		http.Error(w, "entropy unavailable", http.StatusInternalServerError)
		return
	}
	for name, value := range map[string]string{"nh_oidc_state": state, "nh_oidc_nonce": nonce} {
		// #nosec G124 -- HttpOnly and SameSite are fixed; Secure follows the TLS/proxy scheme for localhost development.
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: value, Path: "/api/v1/auth/oidc/callback", HttpOnly: true,
			Secure: requestHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: 600,
		})
	}
	target, err := s.auth.OIDCAuthURL(state, nonce)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, stateErr := r.Cookie("nh_oidc_state")
	nonceCookie, nonceErr := r.Cookie("nh_oidc_nonce")
	if stateErr != nil || nonceErr != nil || stateCookie.Value == "" ||
		stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid OIDC state", http.StatusBadRequest)
		return
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		http.Error(w, "OIDC provider error", http.StatusUnauthorized)
		return
	}
	principal, session, err := s.auth.ExchangeOIDC(r.Context(), r.URL.Query().Get("code"), nonceCookie.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	s.setSessionCookie(w, r, session)
	for _, name := range []string{"nh_oidc_state", "nh_oidc_nonce"} {
		// #nosec G124 -- HttpOnly and SameSite are fixed; Secure follows the TLS/proxy scheme for localhost development.
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/api/v1/auth/oidc/callback", HttpOnly: true,
			Secure: requestHTTPS(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, map[string]any{"authenticated": true, "principal": principal})
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session string) {
	// #nosec G124 -- HttpOnly and SameSite are fixed; Secure follows the TLS/proxy scheme for localhost development.
	http.SetCookie(w, &http.Cookie{
		Name: s.auth.CookieName, Value: session, Path: "/", HttpOnly: true, Secure: requestHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(s.auth.SessionTTL.Seconds()),
	})
}

func requestHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.svc.Config().Server.EnableMetrics {
		http.NotFound(w, r)
		return
	}
	// refresh gauges
	st := s.svc.Store().Stats(len(s.svc.EnabledSources()))
	s.svc.Metrics().SetNodes(st.TotalNodes, st.AliveNodes, st.HighQuality)
	s.svc.Metrics().SetNodeDimensions(s.svc.Store().AllNodes())
	if database := s.svc.DB(); database != nil {
		if alerts, err := database.ListAlerts(true, 500); err == nil {
			bySeverity := map[string]int64{}
			for _, alert := range alerts {
				bySeverity[alert.Severity]++
			}
			s.svc.Metrics().SetAlerts(bySeverity)
		}
		if queue, err := database.QueueStats(); err == nil {
			s.svc.Metrics().SetQueue(queue)
		}
	}
	s.svc.Metrics().Handler().ServeHTTP(w, r)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st := s.svc.Store().Stats(len(s.svc.EnabledSources()))
	writeJSON(w, st)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	minScore, _ := strconv.ParseFloat(q.Get("min_score"), 64)
	f := store.NodeFilter{
		Protocol:     q.Get("protocol"),
		Source:       q.Get("source"),
		Grade:        q.Get("grade"),
		Country:      q.Get("country"),
		AliveOnly:    q.Get("alive") == "1" || q.Get("alive") == "true",
		MinScore:     minScore,
		AIKey:        q.Get("ai"),
		Search:       q.Get("q"),
		Limit:        0,
		HighQuality:  q.Get("hq") == "1" || q.Get("hq") == "true",
		VerifiedOnly: q.Get("verified") == "1" || q.Get("verified") == "true",
	}
	nodes := s.svc.Store().ListNodes(f)
	start := 0
	if cursor := q.Get("cursor"); cursor != "" {
		start = -1
		for i, n := range nodes {
			if n.ID == cursor {
				start = i + 1
				break
			}
		}
		if start < 0 {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
	}
	end := start + limit
	if end > len(nodes) {
		end = len(nodes)
	}
	page := nodes[start:end]
	if !s.hasRole(r, auth.RoleAdmin) {
		redactNodes(page)
	}
	next := ""
	if end < len(nodes) && len(page) > 0 {
		next = page[len(page)-1].ID
	}
	writeJSON(w, map[string]any{
		"total": len(nodes), "count": len(page), "nodes": page,
		"next_cursor": next, "has_more": next != "",
	})
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, ok := s.svc.Store().GetNode(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !s.hasRole(r, auth.RoleAdmin) {
		redactNodes([]*model.Node{n})
	}
	writeJSON(w, n)
}

func (s *Server) handleNodeMetrics(w http.ResponseWriter, r *http.Request) {
	if s.svc.DB() == nil {
		writeJSON(w, []db.DailyMetric{})
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	metrics, err := s.svc.DB().DailyNodeMetrics(r.PathValue("id"), days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, metrics)
}

func (s *Server) handleTrends(w http.ResponseWriter, r *http.Request) {
	if s.svc.DB() == nil {
		writeJSON(w, []db.DailyMetric{})
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	metrics, err := s.svc.DB().DailyNodeMetrics("", days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, metrics)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleViewer)
	if principal == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	cursor := r.URL.Query().Get("cursor")
	var jobs []*model.Job
	// prefer sqlite history if available
	if s.svc.DB() != nil {
		var err error
		jobs, err = s.svc.DB().ListJobsPageTenant(limit+1, cursor, principal.TenantID)
		if err != nil {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
	} else {
		if cursor != "" {
			if _, ok := s.svc.Store().GetJob(cursor); !ok {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
		}
		jobs = s.svc.Store().ListJobsPage(limit+1, cursor)
	}
	hasMore := len(jobs) > limit
	if hasMore {
		jobs = jobs[:limit]
	}
	next := ""
	if hasMore && len(jobs) > 0 {
		next = jobs[len(jobs)-1].ID
	}
	redactJobs(jobs)
	writeJSON(w, map[string]any{
		"jobs": jobs, "count": len(jobs), "next_cursor": next, "has_more": hasMore,
	})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleViewer)
	if principal == nil {
		return
	}
	id := r.PathValue("id")
	var j *model.Job
	var ok bool
	if s.svc.DB() != nil {
		var err error
		j, err = s.svc.DB().GetJobTenant(id, principal.TenantID)
		ok = err == nil
	} else {
		j, ok = s.svc.Store().GetJob(id)
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	redactJobs([]*model.Job{j})
	writeJSON(w, j)
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleViewer)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []db.JobEvent{})
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	events, err := s.svc.DB().ListJobEventsTenant(r.PathValue("id"), after, 500, principal.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
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
		ID            string `json:"id"`
		Name          string `json:"name"`
		Country       string `json:"country"`
		Protocol      string `json:"protocol"`
		ExitIP        string `json:"exit_ip,omitempty"`
		CleanScore    int    `json:"clean_score"`
		RiskScore     int    `json:"risk_score"`
		Grade         string `json:"grade,omitempty"`
		IsProxy       bool   `json:"is_proxy"`
		IsHosting     bool   `json:"is_hosting"`
		CFChallenge   string `json:"cf_challenge,omitempty"`
		CFHumanLikely bool   `json:"cf_human_likely"`
		ISP           string `json:"isp,omitempty"`
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
		"verified_total":   len(nodes),
		"purity_tested":    withPurity,
		"avg_clean_score":  avg,
		"cf_human_likely":  cfOK,
		"clean_score_ge70": clean70,
		"by_grade":         byGrade,
		"nodes":            out,
		"disclaimer":       "CF human check is heuristic (challenge page detection), not real Turnstile solve. Scores use ip-api proxy/hosting flags + CF signals.",
	})
}

func (s *Server) handleDialStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	status := map[string]any{
		"enabled":           cfg.Dial.Enabled,
		"engine":            cfg.Dial.Engine,
		"bin":               cfg.Dial.Bin,
		"concurrency":       cfg.Dial.Concurrency,
		"timeout_sec":       cfg.Dial.TimeoutSec,
		"test_url":          cfg.Dial.TestURL,
		"download_bytes":    cfg.Dial.DownloadBytes,
		"sample_percent":    cfg.Dial.SamplePercent,
		"max_nodes":         cfg.Dial.MaxNodes, // 0=全部 HQ
		"batch_size":        cfg.Dial.BatchSize,
		"after_quality":     cfg.Dial.AfterQuality,
		"after_quality_max": cfg.Dial.AfterQualityMax,
		"all_hq":            cfg.Dial.MaxNodes <= 0,
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
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return
	}
	opts, err := readOpts(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	opts["_actor"] = principal.Actor()
	opts["_tenant"] = principal.TenantID
	opts["_request_id"] = middleware.RequestID(r.Context())
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(r.Context(), carrier)
	if value := carrier.Get("traceparent"); value != "" {
		opts["_traceparent"] = value
	}
	if value := carrier.Get("tracestate"); value != "" {
		opts["_tracestate"] = value
	}
	j, err := fn(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, j)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if s.requireRole(w, r, auth.RoleOperator) == nil {
		return
	}
	ok := s.svc.CancelJob()
	writeJSON(w, map[string]any{"canceled": ok})
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	cfg := s.svc.Config()
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	switch sortBy {
	case "", "priority", "health", "contribution", "success":
	default:
		http.Error(w, "sort must be priority, health, contribution, or success", http.StatusBadRequest)
		return
	}
	type src struct {
		Name                string  `json:"name"`
		Type                string  `json:"type"`
		URL                 string  `json:"url"`
		Enabled             bool    `json:"enabled"`
		Priority            int     `json:"priority"`
		MaxBytes            int64   `json:"max_bytes"`
		LastAttemptAt       string  `json:"last_attempt_at,omitempty"`
		LastSuccessAt       string  `json:"last_success_at,omitempty"`
		LastError           string  `json:"last_error,omitempty"`
		ConsecutiveFailures int     `json:"consecutive_failures"`
		FetchCount          int64   `json:"fetch_count"`
		SuccessRate         float64 `json:"success_rate"`
		LatencyMS           int64   `json:"latency_ms"`
		Bytes               int     `json:"bytes"`
		StatusCode          int     `json:"status_code"`
		DisabledUntil       string  `json:"disabled_until,omitempty"`
		ManuallyDisabled    bool    `json:"manually_disabled"`
		EffectiveEnabled    bool    `json:"effective_enabled"`
		ContributionTotal   int     `json:"contribution_total"`
		ContributionHQ      int     `json:"contribution_hq"`
		HealthScore         float64 `json:"health_score"`
	}
	states := s.svc.SourceStates()
	effective := make(map[string]bool)
	for _, source := range s.svc.EnabledSources() {
		effective[source.Name] = true
	}
	list := make([]src, 0, len(cfg.Sources))
	sources := append([]config.Source(nil), cfg.Sources...)
	principal, _ := s.auth.RequestPrincipal(r)
	isAdmin := principal != nil && principal.Authenticated && principal.Role.Allows(auth.RoleAdmin)
	for _, s0 := range sources {
		state := states[s0.Name]
		sourceURL, lastError := s0.URL, state.LastError
		if !isAdmin {
			safeURL := redactSourceURL(sourceURL)
			lastError = strings.ReplaceAll(lastError, sourceURL, safeURL)
			sourceURL = safeURL
		}
		successRate := 0.0
		if state.FetchCount > 0 {
			successRate = float64(state.SuccessCount) / float64(state.FetchCount)
		}
		list = append(list, src{
			Name: s0.Name, Type: s0.Type, URL: sourceURL, Enabled: s0.Enabled,
			Priority: s0.Priority, MaxBytes: s0.MaxBytes,
			LastAttemptAt: state.LastAttemptAt, LastSuccessAt: state.LastSuccessAt,
			LastError: lastError, ConsecutiveFailures: state.ConsecutiveFailures,
			FetchCount: state.FetchCount, SuccessRate: successRate,
			LatencyMS: state.LatencyMS, Bytes: state.Bytes, StatusCode: state.StatusCode,
			DisabledUntil: state.DisabledUntil, ManuallyDisabled: state.ManuallyDisabled,
			EffectiveEnabled: effective[s0.Name], ContributionTotal: state.ContributionTotal,
			ContributionHQ: state.ContributionHQ, HealthScore: state.HealthScore,
		})
	}
	sort.SliceStable(list, func(i, j int) bool {
		switch sortBy {
		case "health":
			return list[i].HealthScore > list[j].HealthScore
		case "contribution":
			return list[i].ContributionHQ > list[j].ContributionHQ
		case "success":
			return list[i].SuccessRate > list[j].SuccessRate
		default:
			return list[i].Priority > list[j].Priority
		}
	})
	writeJSON(w, list)
}

func (s *Server) handleAITargets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, model.DefaultAITargets())
}
func (s *Server) handleHostAI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.svc.Store().HostAI())
}

func (s *Server) handleExportRaw(w http.ResponseWriter, r *http.Request) {
	if !s.ensureExport(w, r) {
		return
	}
	nodes := s.nodesFromExportQuery(r)
	body := exporter.RenderRaw(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.txt")
	// #nosec G705 -- the response is an authenticated download with text/plain and global nosniff/CSP headers.
	_, _ = w.Write([]byte(body))
}
func (s *Server) handleExportBase64(w http.ResponseWriter, r *http.Request) {
	if !s.ensureExport(w, r) {
		return
	}
	nodes := s.nodesFromExportQuery(r)
	enc := exporter.RenderBase64(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.base64.txt")
	// #nosec G705 -- the response is an authenticated download with text/plain and global nosniff/CSP headers.
	_, _ = w.Write([]byte(enc))
}
func (s *Server) handleExportClash(w http.ResponseWriter, r *http.Request) {
	if !s.ensureExport(w, r) {
		return
	}
	nodes := s.nodesFromExportQuery(r)
	body := exporter.RenderClash(nodes)
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=nodes.clash.yaml")
	// #nosec G705 -- the response is an authenticated YAML download with global nosniff/CSP headers.
	_, _ = w.Write([]byte(body))
}

func (s *Server) ensureExport(w http.ResponseWriter, r *http.Request) bool {
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return false
	}
	if s.svc.DB() != nil {
		_ = s.svc.DB().Audit(principal.Actor(), "export.download", r.URL.Path)
	}
	return true
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
		"sources_enabled":     len(s.svc.EnabledSources()),
		"sources_total":       len(cfg.Sources),
		"version":             version.Version,
		"schedule": map[string]any{
			"enabled":      cfg.Schedule.Enabled,
			"interval_min": cfg.Schedule.IntervalMin,
			"job":          cfg.Schedule.Job,
			"skip_ai":      cfg.Schedule.SkipAI,
		},
		"publish": map[string]any{
			"enabled":            cfg.Publish.Enabled,
			"path_prefix":        cfg.Publish.PathPrefix,
			"token_set":          cfg.Publish.Token != "",
			"min_score":          cfg.Publish.MinScore,
			"max_nodes":          cfg.Publish.MaxNodes,
			"alive_only":         cfg.Publish.AliveOnly,
			"max_node_age_hours": cfg.Publish.MaxNodeAgeHours,
			"public_url":         cfg.Publish.PublicURL,
			"formats":            cfg.Publish.Formats,
			"pre_render":         cfg.Publish.PreRender,
			"cache_sec":          cfg.Publish.CacheSec,
		},
		"security": map[string]any{
			"allow_query_token": cfg.Security.AllowQueryToken,
			"sub_rps":           cfg.Security.SubRPS,
			"admin_token_set":   cfg.Security.AdminToken != "" || cfg.Publish.Token != "",
		},
		"sqlite": cfg.SQLite.Enabled,
		"server": map[string]any{
			"read_timeout_sec":  cfg.Server.ReadTimeoutSec,
			"write_timeout_sec": cfg.Server.WriteTimeoutSec,
			"idle_timeout_sec":  cfg.Server.IdleTimeoutSec,
			"max_header_bytes":  cfg.Server.MaxHeaderBytes,
			"allowed_origins":   cfg.Server.AllowedOrigins,
			"trusted_proxies":   cfg.Server.TrustedProxies,
		},
		"geo": map[string]any{
			"enabled":  cfg.Geo.Enabled,
			"mmdb":     s.svc.Geo() != nil && s.svc.Geo().Ready(),
			"asn_mmdb": cfg.Geo.ASNDBPath != "",
		},
		"database": map[string]any{
			"enabled": cfg.Database.Enabled,
			"driver":  cfg.Database.Driver,
		},
		"redis": map[string]any{
			"enabled": cfg.Redis.Enabled,
		},
		"queue": map[string]any{
			"enabled":          cfg.Queue.Enabled,
			"embedded_workers": cfg.Queue.EmbeddedWorkers,
			"max_pending":      cfg.Queue.MaxPending,
		},
		"auth": map[string]any{
			"local_enabled": cfg.Auth.LocalEnabled,
			"oidc_enabled":  cfg.Auth.OIDC.Enabled,
			"admin_host":    cfg.Auth.AdminHost,
			"public_host":   cfg.Auth.PublicHost,
		},
		"governance": map[string]any{
			"disable_after_failures": cfg.Governance.DisableAfterFailures,
			"cooldown_hours":         cfg.Governance.CooldownHours,
			"hq_drop_percent":        cfg.Governance.HQDropPercent,
			"country_share_percent":  cfg.Governance.CountrySharePercent,
			"terms_url":              cfg.Governance.TermsURL,
			"alert_webhook_set":      cfg.Governance.AlertWebhookURL != "",
		},
		"dial": map[string]any{
			"enabled":           cfg.Dial.Enabled,
			"engine":            cfg.Dial.Engine,
			"after_quality":     cfg.Dial.AfterQuality,
			"after_quality_max": cfg.Dial.AfterQualityMax,
			"sample_percent":    cfg.Dial.SamplePercent,
			"download_bytes":    cfg.Dial.DownloadBytes,
		},
		"observability": map[string]any{
			"otel_enabled": cfg.Observability.OTLPEndpoint != "",
			"service_name": cfg.Observability.ServiceName,
			"sample_ratio": cfg.Observability.SampleRatio,
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
	cfg := s.svc.Config()
	list := pools.Configured(cfg)
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		nodes := pools.Select(s.svc.Store(), p)
		out = append(out, map[string]any{
			"key": p.Key, "title": p.Title, "description": p.Description, "count": len(nodes),
			"refresh_sec": p.RefreshSec, "min_score": p.MinScore, "max_nodes": p.MaxNodes,
			"urls": map[string]string{
				"raw":    cfg.Publish.PathPrefix + "/pool/" + p.Key + "/raw",
				"base64": cfg.Publish.PathPrefix + "/pool/" + p.Key + "/base64",
				"clash":  cfg.Publish.PathPrefix + "/pool/" + p.Key + "/clash",
			},
		})
	}
	writeJSON(w, out)
}

func (s *Server) handlePoolExportRaw(w http.ResponseWriter, r *http.Request) {
	if !s.ensureExport(w, r) {
		return
	}
	key := r.PathValue("key")
	p, ok := pools.Find(s.svc.Config(), key)
	if !ok {
		http.Error(w, "unknown pool", 404)
		return
	}
	nodes := pools.Select(s.svc.Store(), p)
	body := exporter.RenderRaw(nodes)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// #nosec G705 -- the response is an authenticated text download with global nosniff/CSP headers.
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleSubPool(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.ensurePublish(w, r)
	if !ok {
		return
	}
	pool, found := pools.Find(s.svc.Config(), r.PathValue("key"))
	if !found {
		http.Error(w, "unknown pool", http.StatusNotFound)
		return
	}
	nodes := filterTokenNodes(pools.Select(s.svc.Store(), pool), principal)
	s.serveSelectedSubscription(w, r, nodes, r.PathValue("format"), "pool-"+pool.Key, pool.RefreshSec, principal)
}

func (s *Server) handleSubCountry(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.ensurePublish(w, r)
	if !ok {
		return
	}
	country := countryCode(r.PathValue("code"))
	if len(country) != 2 {
		http.Error(w, "invalid country code", http.StatusBadRequest)
		return
	}
	if !principalAllowsCountry(principal, country) {
		http.Error(w, "country not allowed for token", http.StatusForbidden)
		return
	}
	nodes := filterTokenNodes(s.svc.SelectPublishNodesCountry(country), principal)
	s.serveSelectedSubscription(w, r, nodes, r.PathValue("format"), "country-"+country,
		s.svc.Config().Publish.CacheSec, principal)
}

func (s *Server) serveSelectedSubscription(
	w http.ResponseWriter, r *http.Request, nodes []*model.Node, format, filename string, refreshSec int,
	principal *auth.Principal,
) {
	var body, contentType, extension string
	switch strings.ToLower(format) {
	case "raw":
		body, contentType, extension = exporter.RenderRaw(nodes), "text/plain; charset=utf-8", ".txt"
	case "base64":
		body, contentType, extension = exporter.RenderBase64(nodes), "text/plain; charset=utf-8", ".base64.txt"
	case "clash", "clash.yaml":
		body, contentType, extension = exporter.RenderClash(nodes), "text/yaml; charset=utf-8", ".clash.yaml"
	default:
		http.Error(w, "unsupported format", http.StatusBadRequest)
		s.svc.Metrics().IncSubToken(format, http.StatusBadRequest, tokenMetricID(principal))
		return
	}
	sum := sha256.Sum256([]byte(body))
	etag := fmt.Sprintf(`"%x"`, sum[:8])
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		s.svc.Metrics().IncSubToken(format, http.StatusNotModified, tokenMetricID(principal))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s%s", filename, extension))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", max(refreshSec, 0)))
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Node-Count", strconv.Itoa(len(nodes)))
	// #nosec G705 -- the selected format fixes a non-HTML content type and global middleware sets nosniff/CSP.
	written, _ := w.Write([]byte(body))
	if principal != nil && principal.Kind == "db" && s.svc.DB() != nil {
		_ = s.svc.DB().AddTokenBytes(principal.TokenID, int64(written))
	}
	s.svc.Metrics().IncSubToken(format, http.StatusOK, tokenMetricID(principal))
}

func principalAllowsCountry(principal *auth.Principal, country string) bool {
	if principal == nil || len(principal.AllowCountries) == 0 {
		return true
	}
	for _, allowed := range principal.AllowCountries {
		if countryCode(allowed) == country {
			return true
		}
	}
	return false
}

func tokenMetricID(principal *auth.Principal) string {
	if principal == nil {
		return "anonymous"
	}
	if principal.TokenID != "" {
		return principal.TokenID
	}
	if principal.Kind != "" {
		return principal.Kind
	}
	return "anonymous"
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
	key := middleware.ClientIP(r, cfg.Server.TrustedProxies)
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
	if p != nil && p.MaxRPS > 0 {
		burst := int(p.MaxRPS*2) + 1
		if !s.subRL.AllowRate("token|"+p.TokenID, p.MaxRPS, burst) {
			http.Error(w, "token rate limit exceeded", http.StatusTooManyRequests)
			return nil, false
		}
	}
	requestedCountry := countryCode(r.URL.Query().Get("country"))
	if p != nil && requestedCountry != "" && len(p.AllowCountries) > 0 {
		allowed := false
		for _, country := range p.AllowCountries {
			if countryCode(country) == requestedCountry {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "country not allowed for token", http.StatusForbidden)
			return nil, false
		}
	}
	if p != nil && p.Kind == "db" && s.svc.DB() != nil {
		remaining, allowed, err := s.svc.DB().ConsumeTokenQuota(p.TokenID, p.DailyQuota)
		if err != nil {
			http.Error(w, "quota check failed", http.StatusServiceUnavailable)
			return nil, false
		}
		if !allowed {
			http.Error(w, "daily quota exceeded", http.StatusTooManyRequests)
			return nil, false
		}
		if remaining >= 0 {
			w.Header().Set("X-RateLimit-Remaining-Daily", strconv.FormatInt(remaining, 10))
		}
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
	country := countryCode(r.URL.Query().Get("country"))
	// token 国家 ACL
	if p != nil && len(p.AllowCountries) > 0 && country != "" {
		allowed := false
		for _, c := range p.AllowCountries {
			if countryCode(c) == country {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "country not allowed for token", http.StatusForbidden)
			s.svc.Metrics().IncSubToken(format, http.StatusForbidden, tokenMetricID(p))
			return
		}
	}

	var body string
	var count int
	etag := ""
	updated := ""
	blob := s.freshPublishBlob()
	if blob != nil && (p == nil || (len(p.AllowCountries) == 0 && len(p.AllowProtocols) == 0) ||
		(country != "" && len(p.AllowProtocols) == 0)) {
		if b, c, ok := blob.Format(format, country); ok {
			body, count, etag, updated = b, c, blob.ETag, blob.UpdatedAt
		}
	}
	// cache miss or country not pre-rendered → live select
	if body == "" {
		nodes := s.svc.SelectPublishNodesCountry(country)
		nodes = filterTokenNodes(nodes, p)
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
		s.svc.Metrics().IncSubToken(format, 304, tokenMetricID(p))
		s.svc.Metrics().ObserveSubLatency(time.Since(start).Seconds())
		return
	}
	s.setSubHeaders(w, contentType, filename, etag)
	if updated != "" {
		w.Header().Set("X-Updated-At", updated)
	}
	w.Header().Set("X-Node-Count", strconv.Itoa(count))
	// #nosec G705 -- the selected format fixes a non-HTML content type and global middleware sets nosniff/CSP.
	written, _ := w.Write([]byte(body))
	if p != nil && p.Kind == "db" && s.svc.DB() != nil {
		_ = s.svc.DB().AddTokenBytes(p.TokenID, int64(written))
	}
	s.svc.Metrics().IncSubToken(format, 200, tokenMetricID(p))
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
	principal, ok := s.ensurePublish(w, r)
	if !ok {
		return
	}
	country := r.URL.Query().Get("country")
	blob := s.freshPublishBlob()
	if blob != nil && country == "" && len(principal.AllowCountries) == 0 && len(principal.AllowProtocols) == 0 {
		writeJSON(w, map[string]any{
			"count": blob.Count, "by_country": blob.ByCountry, "etag": blob.ETag,
			"generated": blob.UpdatedAt, "min_score": s.svc.Config().Publish.MinScore,
			"max_nodes": s.svc.Config().Publish.MaxNodes, "cached": true,
		})
		return
	}
	nodes := filterTokenNodes(s.svc.SelectPublishNodesCountry(country), principal)
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
	principal, ok := s.ensurePublish(w, r)
	if !ok {
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

	blob := s.freshPublishBlob()
	count := 0
	byCountry := map[string]int{}
	updated := timex.NowRFC3339()
	if blob != nil && len(principal.AllowCountries) == 0 && len(principal.AllowProtocols) == 0 {
		count = blob.Count
		byCountry = blob.ByCountry
		updated = blob.UpdatedAt
	} else {
		nodes := filterTokenNodes(s.svc.SelectPublishNodes(), principal)
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

func (s *Server) freshPublishBlob() *publish.Blob {
	blob := s.svc.PublishCache().Get()
	if blob != nil && blob.Fresh(s.publishCacheMaxAge()) {
		return blob
	}
	return nil
}

func (s *Server) publishCacheMaxAge() time.Duration {
	cfg := s.svc.Config()
	maxAge := time.Duration(cfg.Publish.MaxNodeAgeHours) * time.Hour
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}
	if cfg.Schedule.Enabled {
		scheduleAge := 2 * cfg.ScheduleInterval()
		if scheduleAge > 0 && scheduleAge < maxAge {
			maxAge = scheduleAge
		}
	}
	return maxAge
}

func countryCode(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "UK" {
		return "GB"
	}
	return code
}

func filterTokenNodes(nodes []*model.Node, principal *auth.Principal) []*model.Node {
	if principal == nil || (len(principal.AllowCountries) == 0 && len(principal.AllowProtocols) == 0) {
		return nodes
	}
	countries := make(map[string]bool, len(principal.AllowCountries))
	for _, country := range principal.AllowCountries {
		countries[countryCode(country)] = true
	}
	protocols := make(map[string]bool, len(principal.AllowProtocols))
	for _, protocol := range principal.AllowProtocols {
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		if protocol == "hy2" {
			protocol = "hysteria2"
		}
		protocols[protocol] = true
	}
	filtered := make([]*model.Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		country := countryCode(node.Country)
		if country == "" {
			country = "XX"
		}
		protocol := strings.ToLower(string(node.Protocol))
		if protocol == "hy2" {
			protocol = "hysteria2"
		}
		if len(countries) > 0 && !countries[country] {
			continue
		}
		if len(protocols) > 0 && !protocols[protocol] {
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered
}

func (s *Server) setSubHeaders(w http.ResponseWriter, contentType, filename, etag string) {
	cfg := s.svc.Config()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", filename))
	w.Header().Set("X-NodeHarvest", "publish")
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
	return s.requireRole(w, r, auth.RoleAdmin) != nil
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, required auth.Role) *auth.Principal {
	principal, err := s.auth.RequestPrincipal(r)
	if err != nil || principal == nil || !principal.Authenticated {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil
	}
	if !principal.Role.Allows(required) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return principal
}

func (s *Server) requireSystemAdmin(w http.ResponseWriter, r *http.Request) *auth.Principal {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal != nil && principal.TenantID != "default" {
		http.Error(w, "system administration requires the default tenant", http.StatusForbidden)
		return nil
	}
	return principal
}

func (s *Server) hasRole(r *http.Request, required auth.Role) bool {
	principal, err := s.auth.RequestPrincipal(r)
	return err == nil && principal != nil && principal.Authenticated && principal.Role.Allows(required)
}

func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.requireAdmin(w, r) {
			next(w, r)
		}
	}
}

func (s *Server) handleAdminListTokens(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []any{})
		return
	}
	list, err := s.svc.DB().ListTokensTenant(principal.TenantID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleAdminCreateToken(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	var body struct {
		Name       string   `json:"name"`
		Note       string   `json:"note"`
		Countries  []string `json:"countries"`
		Protocols  []string `json:"protocols"`
		Days       int      `json:"days"`
		MaxRPS     float64  `json:"max_rps"`
		DailyQuota int64    `json:"daily_quota"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		body.Name = "token"
	}
	if len(body.Name) > 100 || len(body.Note) > 1000 || body.Days < 0 || body.Days > 3650 ||
		body.MaxRPS < 0 || body.MaxRPS > 1000 || body.DailyQuota < 0 {
		http.Error(w, "invalid token options", http.StatusBadRequest)
		return
	}
	t, err := s.auth.CreateToken(body.Name, body.Note, principal.TenantID, body.Countries,
		body.Protocols, body.Days, body.MaxRPS, body.DailyQuota, principal.Actor())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleAdminDeleteToken(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	id := r.PathValue("id")
	if s.svc.DB() == nil {
		http.Error(w, "db disabled", 500)
		return
	}
	if err := s.svc.DB().DeleteTokenTenant(id, principal.TenantID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.svc.DB().Audit(principal.Actor(), "token.delete", id)
	writeJSON(w, map[string]any{"deleted": id})
}

func (s *Server) handleAdminEnableToken(w http.ResponseWriter, r *http.Request) {
	s.setTokenEnabled(w, r, true)
}
func (s *Server) handleAdminDisableToken(w http.ResponseWriter, r *http.Request) {
	s.setTokenEnabled(w, r, false)
}
func (s *Server) setTokenEnabled(w http.ResponseWriter, r *http.Request, en bool) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	id := r.PathValue("id")
	if s.svc.DB() == nil {
		http.Error(w, "db disabled", 500)
		return
	}
	if err := s.svc.DB().SetTokenEnabledTenant(id, principal.TenantID, en); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.svc.DB().Audit(principal.Actor(), "token.enabled", fmt.Sprintf("%s=%t", id, en))
	writeJSON(w, map[string]any{"id": id, "enabled": en})
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []any{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	from, err := auditBound(r.URL.Query().Get("from"), false)
	if err != nil {
		http.Error(w, "invalid from date", http.StatusBadRequest)
		return
	}
	to, err := auditBound(r.URL.Query().Get("to"), true)
	if err != nil {
		http.Error(w, "invalid to date", http.StatusBadRequest)
		return
	}
	list, err := s.svc.DB().ListAuditTenantRange(limit, principal.TenantID, from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, list)
}

func auditBound(value string, endOfDay bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) == len("2006-01-02") {
		day, err := time.ParseInLocation("2006-01-02", value, timex.Location())
		if err != nil {
			return "", err
		}
		if endOfDay {
			day = day.Add(24*time.Hour - time.Nanosecond)
		}
		return timex.FormatRFC3339(day), nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", err
	}
	return timex.FormatRFC3339(at), nil
}

func (s *Server) handleAdminRefreshPublish(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return
	}
	blob := s.svc.RefreshPublishCache()
	if s.svc.DB() != nil {
		_ = s.svc.DB().Audit(principal.Actor(), "publish.refresh", blob.ETag)
	}
	writeJSON(w, map[string]any{
		"ok": true, "count": blob.Count, "etag": blob.ETag, "updated_at": blob.UpdatedAt,
	})
}

func (s *Server) handleAdminEnableSource(w http.ResponseWriter, r *http.Request) {
	s.setSourceEnabled(w, r, true)
}

func (s *Server) handleAdminDisableSource(w http.ResponseWriter, r *http.Request) {
	s.setSourceEnabled(w, r, false)
}

func (s *Server) setSourceEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	principal := s.requireSystemAdmin(w, r)
	if principal == nil {
		return
	}
	name := r.PathValue("name")
	if err := s.svc.SetSourceEnabled(name, enabled, principal.Actor()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"name": name, "enabled": enabled})
}

func (s *Server) handleAdminProbeSource(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return
	}
	state, err := s.svc.ProbeSource(r.Context(), r.PathValue("name"))
	if s.svc.DB() != nil {
		_ = s.svc.DB().Audit(principal.Actor(), "source.probe", r.PathValue("name"))
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, map[string]any{"state": state, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"state": state})
}

func (s *Server) handleAdminTasks(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleViewer)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []db.QueuedTask{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := s.svc.DB().ListTasksTenant(limit, r.URL.Query().Get("status"), principal.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, task := range tasks {
		task.Options = nil
	}
	writeJSON(w, tasks)
}

func (s *Server) handleAdminCancelTask(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return
	}
	id := r.PathValue("id")
	if s.svc.DB() != nil {
		if _, err := s.svc.DB().GetJobTenant(id, principal.TenantID); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	if s.svc.DB() != nil {
		_ = s.svc.DB().Audit(principal.Actor(), "task.cancel", id)
	}
	running := s.svc.CancelJobID(id)
	if s.svc.DB() != nil {
		if err := s.svc.DB().CancelTask(id); err != nil && !running {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
	writeJSON(w, map[string]any{"id": id, "canceled": true})
}

func (s *Server) handleAdminQueue(w http.ResponseWriter, r *http.Request) {
	if s.requireRole(w, r, auth.RoleViewer) == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, map[string]any{"enabled": false})
		return
	}
	stats, err := s.svc.DB().QueueStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"enabled": s.svc.Config().Queue.Enabled,
		"workers": s.svc.Config().Queue.EmbeddedWorkers,
		"tasks":   stats,
	})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []db.User{})
		return
	}
	users, err := s.svc.DB().ListUsers(principal.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, users)
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		http.Error(w, "database disabled", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := auth.Role(strings.ToLower(strings.TrimSpace(body.Role)))
	if body.Username == "" || (role != auth.RoleViewer && role != auth.RoleOperator && role != auth.RoleAdmin) {
		http.Error(w, "username and valid role are required", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, id, _, err := auth.NewTokenPlain()
	if err != nil {
		http.Error(w, "entropy unavailable", http.StatusInternalServerError)
		return
	}
	user := &db.User{
		ID: id, TenantID: principal.TenantID, Username: strings.TrimSpace(body.Username),
		Email: strings.TrimSpace(body.Email), PasswordHash: hash, Role: string(role), Enabled: true,
	}
	if err := s.svc.DB().InsertUser(user); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	_ = s.svc.DB().Audit(principal.Actor(), "user.create", user.ID+" "+user.Username)
	writeJSON(w, user)
}

func (s *Server) handleAdminEnableUser(w http.ResponseWriter, r *http.Request) {
	s.setUserEnabled(w, r, true)
}

func (s *Server) handleAdminDisableUser(w http.ResponseWriter, r *http.Request) {
	s.setUserEnabled(w, r, false)
}

func (s *Server) setUserEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	principal := s.requireRole(w, r, auth.RoleAdmin)
	if principal == nil {
		return
	}
	if s.svc.DB() == nil {
		http.Error(w, "database disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if id == principal.Subject && !enabled {
		http.Error(w, "cannot disable the active account", http.StatusBadRequest)
		return
	}
	if err := s.svc.DB().SetUserEnabled(id, principal.TenantID, enabled); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = s.svc.DB().Audit(principal.Actor(), "user.enabled", fmt.Sprintf("%s=%t", id, enabled))
	writeJSON(w, map[string]any{"id": id, "enabled": enabled})
}

func (s *Server) handleAdminUpdateConfig(w http.ResponseWriter, r *http.Request) {
	principal := s.requireSystemAdmin(w, r)
	if principal == nil {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
		config.RuntimePatch
	}
	if err := decodeJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !body.Confirm {
		http.Error(w, "confirm=true is required", http.StatusBadRequest)
		return
	}
	version, err := s.svc.UpdateRuntimeConfig(principal.Actor(), body.RuntimePatch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg := s.svc.Config()
	writeJSON(w, map[string]any{
		"version": version,
		"config": map[string]any{
			"publish_min_score":                 cfg.Publish.MinScore,
			"publish_max_nodes":                 cfg.Publish.MaxNodes,
			"publish_alive_only":                cfg.Publish.AliveOnly,
			"publish_cache_sec":                 cfg.Publish.CacheSec,
			"publish_max_node_age_hours":        cfg.Publish.MaxNodeAgeHours,
			"governance_disable_after_failures": cfg.Governance.DisableAfterFailures,
			"governance_cooldown_hours":         cfg.Governance.CooldownHours,
			"governance_hq_drop_percent":        cfg.Governance.HQDropPercent,
			"governance_country_share_percent":  cfg.Governance.CountrySharePercent,
			"dial_after_quality":                cfg.Dial.AfterQuality,
			"dial_after_quality_max":            cfg.Dial.AfterQualityMax,
		},
	})
}

func (s *Server) handleAdminConfigVersions(w http.ResponseWriter, r *http.Request) {
	if s.requireSystemAdmin(w, r) == nil {
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []db.ConfigVersion{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	versions, err := s.svc.DB().ListConfigVersions(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, versions)
}

func (s *Server) handleAdminAlerts(w http.ResponseWriter, r *http.Request) {
	principal := s.requireRole(w, r, auth.RoleViewer)
	if principal == nil {
		return
	}
	if principal.TenantID != "default" {
		http.Error(w, "system alerts require the default tenant", http.StatusForbidden)
		return
	}
	if s.svc.DB() == nil {
		writeJSON(w, []db.Alert{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	activeOnly := r.URL.Query().Get("active") != "false"
	alerts, err := s.svc.DB().ListAlerts(activeOnly, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, alerts)
}

func (s *Server) handleAdminAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	s.changeAlert(w, r, true)
}

func (s *Server) handleAdminResolveAlert(w http.ResponseWriter, r *http.Request) {
	s.changeAlert(w, r, false)
}

func (s *Server) changeAlert(w http.ResponseWriter, r *http.Request, acknowledge bool) {
	principal := s.requireRole(w, r, auth.RoleOperator)
	if principal == nil {
		return
	}
	if principal.TenantID != "default" {
		http.Error(w, "system alerts require the default tenant", http.StatusForbidden)
		return
	}
	if s.svc.DB() == nil {
		http.Error(w, "database disabled", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var err error
	action := "alert.resolve"
	if acknowledge {
		action = "alert.acknowledge"
		err = s.svc.DB().AcknowledgeAlert(id, principal.Actor())
	} else {
		err = s.svc.DB().ResolveAlert(id)
	}
	if err != nil {
		http.Error(w, "alert not found", http.StatusNotFound)
		return
	}
	_ = s.svc.DB().Audit(principal.Actor(), action, id)
	writeJSON(w, map[string]any{"id": id, "acknowledged": acknowledge, "resolved": !acknowledge})
}

func (s *Server) nodesFromExportQuery(r *http.Request) []*model.Node {
	f := exportFilter(r)
	nodes := s.svc.Store().ListNodes(f)
	if len(nodes) == 0 {
		return s.svc.SelectPublishNodes()
	}
	return nodes
}

func redactNodes(nodes []*model.Node) {
	for _, n := range nodes {
		n.UUID = ""
		n.Password = ""
		n.RawURI = ""
		n.Extra = nil
		n.Fingerprint = ""
	}
}

func redactJobs(jobs []*model.Job) {
	for _, job := range jobs {
		job.Options = nil
	}
}

func redactSourceURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String()
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

func readOpts(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	opts := map[string]any{}
	if r.Body == nil {
		return opts, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer r.Body.Close()
	return opts, decodeOne(json.NewDecoder(r.Body), &opts)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return decodeOne(dec, dst)
}

func decodeOne(dec *json.Decoder, dst any) error {
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain exactly one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func managementBoundary(next http.Handler, cfg *config.Config) http.Handler {
	adminPrefixes := []string{"/api/admin", "/api/jobs", "/api/export", "/api/v1/auth", "/debug", "/metrics"}
	publicAPI := map[string]bool{
		"/api/health": true, "/api/ready": true, "/api/version": true, "/api/stats": true,
		"/api/nodes": true, "/api/countries": true, "/api/sources": true, "/api/pools": true,
		"/api/config": true, "/api/schedule": true, "/api/terms": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := requestHost(r.Host)
		path := r.URL.Path
		subscription := strings.HasPrefix(path, "/sub") || strings.HasPrefix(path, "/api/sub")
		if prefix := strings.TrimRight(cfg.Publish.PathPrefix, "/"); prefix != "" {
			subscription = subscription || path == prefix || strings.HasPrefix(path, prefix+"/")
		}
		management := false
		for _, prefix := range adminPrefixes {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				management = true
				break
			}
		}
		if strings.HasPrefix(path, "/api/") && !publicAPI[path] && !subscription {
			management = true
		}
		if strings.HasPrefix(path, "/api/") && r.Method != http.MethodGet && !strings.HasPrefix(path, "/api/sub") {
			management = true
		}
		if subscription && cfg.Auth.PublicHost != "" && !strings.EqualFold(host, requestHost(cfg.Auth.PublicHost)) {
			http.NotFound(w, r)
			return
		}
		if management {
			if cfg.Auth.AdminHost != "" && !strings.EqualFold(host, requestHost(cfg.Auth.AdminHost)) {
				http.NotFound(w, r)
				return
			}
			if len(cfg.Auth.AdminCIDRs) > 0 && !allowedClientIP(
				middleware.ClientIP(r, cfg.Server.TrustedProxies), cfg.Auth.AdminCIDRs,
			) {
				http.Error(w, "management network denied", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requestHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(raw, "[]")
}

func allowedClientIP(raw string, cidrs []string) bool {
	ip, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	for _, rawCIDR := range cidrs {
		if prefix, err := netip.ParsePrefix(strings.TrimSpace(rawCIDR)); err == nil && prefix.Contains(ip) {
			return true
		}
		if allowed, err := netip.ParseAddr(strings.TrimSpace(rawCIDR)); err == nil && allowed == ip {
			return true
		}
	}
	return false
}

func cors(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		origin := r.Header.Get("Origin")
		allowed := origin == "" || sameOrigin(origin, r.Host)
		for _, candidate := range allowedOrigins {
			if candidate == "*" || strings.EqualFold(strings.TrimSpace(candidate), origin) {
				allowed = true
				if origin != "" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				break
			}
		}
		if origin != "" && !allowed {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Sub-Token, X-Admin-Token")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, host)
}
