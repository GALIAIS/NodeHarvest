package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 轻量 Prometheus 文本指标，无第三方依赖。

type Registry struct {
	mu sync.Mutex

	subReqs     map[string]*atomic.Int64 // format|code
	subLatency  latencyAgg
	jobRuns     map[string]*atomic.Int64 // type|status
	jobSeconds  map[string]*latencyAgg  // type
	sourceErr   map[string]*atomic.Int64
	nodesGauge  atomic.Int64
	aliveGauge  atomic.Int64
	hqGauge     atomic.Int64
	httpReqs    map[string]*atomic.Int64 // method|path_group|code
}

type latencyAgg struct {
	mu   sync.Mutex
	sum  float64
	n    int64
	max  float64
}

func (a *latencyAgg) observe(sec float64) {
	a.mu.Lock()
	a.sum += sec
	a.n++
	if sec > a.max {
		a.max = sec
	}
	a.mu.Unlock()
}

func (a *latencyAgg) snapshot() (count int64, sum, max float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n, a.sum, a.max
}

func New() *Registry {
	return &Registry{
		subReqs:    map[string]*atomic.Int64{},
		jobRuns:    map[string]*atomic.Int64{},
		jobSeconds: map[string]*latencyAgg{},
		sourceErr:  map[string]*atomic.Int64{},
		httpReqs:   map[string]*atomic.Int64{},
	}
}

func (r *Registry) keyCounter(m map[string]*atomic.Int64, key string) *atomic.Int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := m[key]
	if !ok {
		c = &atomic.Int64{}
		m[key] = c
	}
	return c
}

func (r *Registry) IncSub(format string, code int) {
	r.keyCounter(r.subReqs, fmt.Sprintf("%s|%d", format, code)).Add(1)
}

func (r *Registry) ObserveSubLatency(sec float64) { r.subLatency.observe(sec) }

func (r *Registry) IncJob(typ, status string) {
	r.keyCounter(r.jobRuns, typ+"|"+status).Add(1)
}

func (r *Registry) ObserveJob(typ string, sec float64) {
	r.mu.Lock()
	a, ok := r.jobSeconds[typ]
	if !ok {
		a = &latencyAgg{}
		r.jobSeconds[typ] = a
	}
	r.mu.Unlock()
	a.observe(sec)
}

func (r *Registry) IncSourceErr(name string) {
	r.keyCounter(r.sourceErr, name).Add(1)
}

func (r *Registry) IncHTTP(method, group string, code int) {
	r.keyCounter(r.httpReqs, fmt.Sprintf("%s|%s|%d", method, group, code)).Add(1)
}

func (r *Registry) SetNodes(total, alive, hq int) {
	r.nodesGauge.Store(int64(total))
	r.aliveGauge.Store(int64(alive))
	r.hqGauge.Store(int64(hq))
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	})
}

func (r *Registry) Render() string {
	var b strings.Builder
	now := time.Now().UnixMilli()
	_ = now
	write := func(line string) { b.WriteString(line); b.WriteByte('\n') }

	write("# HELP nh_nodes_total Stored nodes")
	write("# TYPE nh_nodes_total gauge")
	write(fmt.Sprintf("nh_nodes_total %d", r.nodesGauge.Load()))
	write("# HELP nh_nodes_alive Alive nodes")
	write("# TYPE nh_nodes_alive gauge")
	write(fmt.Sprintf("nh_nodes_alive %d", r.aliveGauge.Load()))
	write("# HELP nh_nodes_hq High quality nodes")
	write("# TYPE nh_nodes_hq gauge")
	write(fmt.Sprintf("nh_nodes_hq %d", r.hqGauge.Load()))

	write("# HELP nh_sub_requests_total Subscription requests")
	write("# TYPE nh_sub_requests_total counter")
	r.mu.Lock()
	keys := make([]string, 0, len(r.subReqs))
	for k := range r.subReqs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		write(fmt.Sprintf(`nh_sub_requests_total{format=%q,code=%q} %d`, parts[0], parts[1], r.subReqs[k].Load()))
	}
	r.mu.Unlock()

	n, sum, max := r.subLatency.snapshot()
	write("# HELP nh_sub_latency_seconds Subscription latency")
	write("# TYPE nh_sub_latency_seconds summary")
	write(fmt.Sprintf("nh_sub_latency_seconds_sum %g", sum))
	write(fmt.Sprintf("nh_sub_latency_seconds_count %d", n))
	write(fmt.Sprintf("nh_sub_latency_seconds_max %g", max))

	write("# HELP nh_job_runs_total Jobs finished")
	write("# TYPE nh_job_runs_total counter")
	r.mu.Lock()
	keys = keys[:0]
	for k := range r.jobRuns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		write(fmt.Sprintf(`nh_job_runs_total{type=%q,status=%q} %d`, parts[0], parts[1], r.jobRuns[k].Load()))
	}
	for typ, a := range r.jobSeconds {
		cn, sm, mx := a.snapshot()
		write(fmt.Sprintf(`nh_job_duration_seconds_sum{type=%q} %g`, typ, sm))
		write(fmt.Sprintf(`nh_job_duration_seconds_count{type=%q} %d`, typ, cn))
		write(fmt.Sprintf(`nh_job_duration_seconds_max{type=%q} %g`, typ, mx))
	}
	r.mu.Unlock()

	return b.String()
}
