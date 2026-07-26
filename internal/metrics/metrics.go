package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/GALIAIS/NodeHarvest/internal/model"
)

var latencyBounds = [...]float64{.05, .1, .2, .5, 1, 2, 5}

type Registry struct {
	mu sync.Mutex

	subReqs        map[string]*atomic.Int64 // format|code|token_id
	subLatency     latencyAgg
	jobRuns        map[string]*atomic.Int64 // type|status
	jobSeconds     map[string]*latencyAgg   // type|status
	sourceReqs     map[string]*atomic.Int64 // source|status
	sourceTime     map[string]*latencyAgg
	sourceBytes    map[string]*atomic.Int64
	geoResults     map[string]*atomic.Int64
	nodesGauge     atomic.Int64
	aliveGauge     atomic.Int64
	hqGauge        atomic.Int64
	nodeDimensions map[string]int64         // grade|protocol|country|alive
	alerts         map[string]int64         // severity
	queue          map[string]int64         // status
	httpReqs       map[string]*atomic.Int64 // method|path_group|code
}

type latencyAgg struct {
	mu      sync.Mutex
	sum     float64
	n       int64
	max     float64
	buckets [len(latencyBounds) + 1]int64
}

func (a *latencyAgg) observe(seconds float64) {
	a.mu.Lock()
	a.sum += seconds
	a.n++
	if seconds > a.max {
		a.max = seconds
	}
	placed := false
	for i, bound := range latencyBounds {
		if seconds <= bound {
			a.buckets[i]++
			placed = true
		}
	}
	a.buckets[len(a.buckets)-1]++
	if !placed {
		// +Inf was already incremented.
	}
	a.mu.Unlock()
}

func (a *latencyAgg) snapshot() (count int64, sum, max float64, buckets [len(latencyBounds) + 1]int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n, a.sum, a.max, a.buckets
}

func New() *Registry {
	return &Registry{
		subReqs: map[string]*atomic.Int64{}, jobRuns: map[string]*atomic.Int64{},
		jobSeconds: map[string]*latencyAgg{}, sourceReqs: map[string]*atomic.Int64{},
		sourceTime: map[string]*latencyAgg{}, sourceBytes: map[string]*atomic.Int64{},
		geoResults: map[string]*atomic.Int64{}, nodeDimensions: map[string]int64{},
		alerts: map[string]int64{}, queue: map[string]int64{}, httpReqs: map[string]*atomic.Int64{},
	}
}

func (r *Registry) keyCounter(values map[string]*atomic.Int64, key string) *atomic.Int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	counter := values[key]
	if counter == nil {
		counter = &atomic.Int64{}
		values[key] = counter
	}
	return counter
}

func (r *Registry) IncSub(format string, code int) {
	r.IncSubToken(format, code, "anonymous")
}

func (r *Registry) IncSubToken(format string, code int, tokenID string) {
	r.keyCounter(r.subReqs, fmt.Sprintf("%s|%d|%s", format, code, safeTokenLabel(tokenID))).Add(1)
}

func (r *Registry) ObserveSubLatency(seconds float64) { r.subLatency.observe(seconds) }

func (r *Registry) IncJob(jobType, status string) {
	r.keyCounter(r.jobRuns, jobType+"|"+status).Add(1)
}

func (r *Registry) ObserveJob(jobType, status string, seconds float64) {
	key := jobType + "|" + status
	r.mu.Lock()
	aggregate := r.jobSeconds[key]
	if aggregate == nil {
		aggregate = &latencyAgg{}
		r.jobSeconds[key] = aggregate
	}
	r.mu.Unlock()
	aggregate.observe(seconds)
}

func (r *Registry) ObserveSource(name string, ok bool, seconds float64, bytes int) {
	status := "error"
	if ok {
		status = "ok"
	}
	r.keyCounter(r.sourceReqs, name+"|"+status).Add(1)
	r.keyCounter(r.sourceBytes, name).Add(int64(bytes))
	r.mu.Lock()
	aggregate := r.sourceTime[name]
	if aggregate == nil {
		aggregate = &latencyAgg{}
		r.sourceTime[name] = aggregate
	}
	r.mu.Unlock()
	aggregate.observe(seconds)
}

func (r *Registry) ObserveGeo(result string, count int) {
	if count <= 0 {
		count = 1
	}
	r.keyCounter(r.geoResults, result).Add(int64(count))
}

func (r *Registry) IncHTTP(method, group string, code int) {
	r.keyCounter(r.httpReqs, fmt.Sprintf("%s|%s|%d", method, group, code)).Add(1)
}

func (r *Registry) SetNodes(total, alive, highQuality int) {
	r.nodesGauge.Store(int64(total))
	r.aliveGauge.Store(int64(alive))
	r.hqGauge.Store(int64(highQuality))
}

func (r *Registry) SetNodeDimensions(nodes []*model.Node) {
	values := map[string]int64{}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		country := strings.ToUpper(node.Country)
		if country == "" {
			country = "XX"
		}
		alive := "false"
		if node.Alive {
			alive = "true"
		}
		values[node.Grade+"|"+string(node.Protocol)+"|"+country+"|"+alive]++
	}
	r.mu.Lock()
	r.nodeDimensions = values
	r.mu.Unlock()
}

func (r *Registry) SetAlerts(alerts map[string]int64) {
	r.mu.Lock()
	r.alerts = cloneMap(alerts)
	r.mu.Unlock()
}

func (r *Registry) SetQueue(queue map[string]int64) {
	r.mu.Lock()
	r.queue = cloneMap(queue)
	r.mu.Unlock()
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	})
}

func (r *Registry) Render() string {
	var b strings.Builder
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}

	write("# HELP nh_nodes_total Stored nodes")
	write("# TYPE nh_nodes_total gauge")
	write("nh_nodes_total %d", r.nodesGauge.Load())
	write("# HELP nh_nodes_alive Alive nodes")
	write("# TYPE nh_nodes_alive gauge")
	write("nh_nodes_alive %d", r.aliveGauge.Load())
	write("# HELP nh_nodes_hq High quality nodes")
	write("# TYPE nh_nodes_hq gauge")
	write("nh_nodes_hq %d", r.hqGauge.Load())

	r.mu.Lock()
	write("# HELP nh_nodes_by_dimension Nodes by grade, protocol, country, and liveness")
	write("# TYPE nh_nodes_by_dimension gauge")
	for _, key := range sortedKeys(r.nodeDimensions) {
		parts := strings.SplitN(key, "|", 4)
		write(`nh_nodes_by_dimension{grade=%q,protocol=%q,country=%q,alive=%q} %d`,
			parts[0], parts[1], parts[2], parts[3], r.nodeDimensions[key])
	}

	write("# HELP nh_sub_requests_total Subscription requests")
	write("# TYPE nh_sub_requests_total counter")
	for _, key := range sortedKeys(r.subReqs) {
		parts := strings.SplitN(key, "|", 3)
		write(`nh_sub_requests_total{format=%q,code=%q,token_id=%q} %d`,
			parts[0], parts[1], parts[2], r.subReqs[key].Load())
	}
	r.mu.Unlock()
	renderHistogram(&b, "nh_sub_latency_seconds", nil, &r.subLatency)

	r.mu.Lock()
	write("# HELP nh_job_runs_total Jobs finished")
	write("# TYPE nh_job_runs_total counter")
	for _, key := range sortedKeys(r.jobRuns) {
		parts := strings.SplitN(key, "|", 2)
		write(`nh_job_runs_total{type=%q,status=%q} %d`, parts[0], parts[1], r.jobRuns[key].Load())
	}
	for _, key := range sortedKeys(r.jobSeconds) {
		parts := strings.SplitN(key, "|", 2)
		renderHistogram(&b, "nh_job_duration_seconds",
			map[string]string{"type": parts[0], "status": parts[1]}, r.jobSeconds[key])
	}

	write("# HELP nh_source_fetch_total Source fetch results")
	write("# TYPE nh_source_fetch_total counter")
	for _, key := range sortedKeys(r.sourceReqs) {
		parts := strings.SplitN(key, "|", 2)
		write(`nh_source_fetch_total{source=%q,status=%q} %d`, parts[0], parts[1], r.sourceReqs[key].Load())
	}
	for _, source := range sortedKeys(r.sourceTime) {
		renderHistogram(&b, "nh_source_fetch_duration_seconds", map[string]string{"source": source}, r.sourceTime[source])
	}
	for _, source := range sortedKeys(r.sourceBytes) {
		write(`nh_source_fetch_bytes_total{source=%q} %d`, source, r.sourceBytes[source].Load())
	}
	for _, result := range sortedKeys(r.geoResults) {
		write(`nh_geo_annotate_total{result=%q} %d`, result, r.geoResults[result].Load())
	}
	for _, severity := range sortedKeys(r.alerts) {
		write(`nh_alerts_active{severity=%q} %d`, severity, r.alerts[severity])
	}
	for _, status := range sortedKeys(r.queue) {
		write(`nh_queue_tasks{status=%q} %d`, status, r.queue[status])
	}

	write("# HELP nh_http_requests_total HTTP requests")
	write("# TYPE nh_http_requests_total counter")
	for _, key := range sortedKeys(r.httpReqs) {
		parts := strings.SplitN(key, "|", 3)
		write(`nh_http_requests_total{method=%q,route=%q,code=%q} %d`,
			parts[0], parts[1], parts[2], r.httpReqs[key].Load())
	}
	r.mu.Unlock()
	return b.String()
}

func renderHistogram(b *strings.Builder, name string, labels map[string]string, aggregate *latencyAgg) {
	count, sum, maximum, buckets := aggregate.snapshot()
	labelText := labelSet(labels)
	for i, bound := range latencyBounds {
		bucketLabels := cloneStrings(labels)
		bucketLabels["le"] = fmt.Sprintf("%g", bound)
		fmt.Fprintf(b, "%s_bucket%s %d\n", name, labelSet(bucketLabels), buckets[i])
	}
	bucketLabels := cloneStrings(labels)
	bucketLabels["le"] = "+Inf"
	fmt.Fprintf(b, "%s_bucket%s %d\n", name, labelSet(bucketLabels), buckets[len(buckets)-1])
	fmt.Fprintf(b, "%s_sum%s %g\n", name, labelText, sum)
	fmt.Fprintf(b, "%s_count%s %d\n", name, labelText, count)
	fmt.Fprintf(b, "%s_max%s %g\n", name, labelText, maximum)
}

func labelSet(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := sortedKeys(labels)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, labels[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func safeTokenLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "anonymous"
	}
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(values map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStrings(values map[string]string) map[string]string {
	out := make(map[string]string, len(values)+1)
	for key, value := range values {
		out[key] = value
	}
	return out
}
