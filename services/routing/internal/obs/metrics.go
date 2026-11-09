package obs

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultLatencyBuckets (seconds) are tuned around the NFR-1 800ms budget.
var DefaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.8, 1, 2.5}

// Registry holds metrics and renders them in Prometheus text format.
type Registry struct {
	mu         sync.Mutex
	collectors []collector
}

type collector interface {
	write(w io.Writer)
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) register(c collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, c)
}

// Handler serves the registry at /metrics.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.mu.Lock()
		cs := append([]collector(nil), r.collectors...)
		r.mu.Unlock()
		for _, c := range cs {
			c.write(w)
		}
	})
}

// labelKey renders sorted-order label pairs as `a="x",b="y"`.
func labelKey(names, values []string) string {
	var b strings.Builder
	for i, n := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(n)
		b.WriteString(`="`)
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(values[i]))
		b.WriteByte('"')
	}
	return b.String()
}

// ---- Counter ----

// CounterVec is a monotonically increasing counter partitioned by labels.
type CounterVec struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	values     map[string]float64
}

// NewCounterVec registers a counter.
func (r *Registry) NewCounterVec(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{name: name, help: help, labels: labels, values: map[string]float64{}}
	r.register(c)
	return c
}

// Add increments the counter for the given label values.
func (c *CounterVec) Add(delta float64, labelValues ...string) {
	k := labelKey(c.labels, labelValues)
	c.mu.Lock()
	c.values[k] += delta
	c.mu.Unlock()
}

// Inc increments by one.
func (c *CounterVec) Inc(labelValues ...string) { c.Add(1, labelValues...) }

func (c *CounterVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range sortedKeys(c.values) {
		fmt.Fprintf(w, "%s%s %s\n", c.name, braces(k), fmtFloat(c.values[k]))
	}
}

// ---- Gauge ----

// GaugeVec is a settable value partitioned by labels.
type GaugeVec struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	values     map[string]float64
}

// NewGaugeVec registers a gauge.
func (r *Registry) NewGaugeVec(name, help string, labels ...string) *GaugeVec {
	g := &GaugeVec{name: name, help: help, labels: labels, values: map[string]float64{}}
	r.register(g)
	return g
}

// Set sets the gauge value for the given label values.
func (g *GaugeVec) Set(v float64, labelValues ...string) {
	k := labelKey(g.labels, labelValues)
	g.mu.Lock()
	g.values[k] = v
	g.mu.Unlock()
}

// Reset clears all series (useful for *_info gauges on version change).
func (g *GaugeVec) Reset() {
	g.mu.Lock()
	g.values = map[string]float64{}
	g.mu.Unlock()
}

func (g *GaugeVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", g.name, g.help, g.name)
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, k := range sortedKeys(g.values) {
		fmt.Fprintf(w, "%s%s %s\n", g.name, braces(k), fmtFloat(g.values[k]))
	}
}

// ---- Histogram ----

// HistogramVec is a cumulative histogram partitioned by labels.
type HistogramVec struct {
	name, help string
	labels     []string
	buckets    []float64
	mu         sync.Mutex
	series     map[string]*histSeries
}

type histSeries struct {
	counts []uint64 // per bucket, non-cumulative
	count  uint64
	sum    float64
}

// NewHistogramVec registers a histogram with the given upper bounds.
func (r *Registry) NewHistogramVec(name, help string, buckets []float64, labels ...string) *HistogramVec {
	b := append([]float64(nil), buckets...)
	sort.Float64s(b)
	h := &HistogramVec{name: name, help: help, labels: labels, buckets: b, series: map[string]*histSeries{}}
	r.register(h)
	return h
}

// Observe records v for the given label values.
func (h *HistogramVec) Observe(v float64, labelValues ...string) {
	k := labelKey(h.labels, labelValues)
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.series[k]
	if s == nil {
		s = &histSeries{counts: make([]uint64, len(h.buckets))}
		h.series[k] = s
	}
	if i := sort.SearchFloat64s(h.buckets, v); i < len(h.buckets) {
		s.counts[i]++
	}
	s.count++
	s.sum += v
}

func (h *HistogramVec) write(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]string, 0, len(h.series))
	for k := range h.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := h.series[k]
		sep := ""
		if k != "" {
			sep = ","
		}
		var cum uint64
		for i, ub := range h.buckets {
			cum += s.counts[i]
			fmt.Fprintf(w, "%s_bucket{%s%sle=\"%s\"} %d\n", h.name, k, sep, fmtFloat(ub), cum)
		}
		fmt.Fprintf(w, "%s_bucket{%s%sle=\"+Inf\"} %d\n", h.name, k, sep, s.count)
		fmt.Fprintf(w, "%s_sum%s %s\n", h.name, braces(k), fmtFloat(s.sum))
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, braces(k), s.count)
	}
}

// ---- HTTP metrics ----

// HTTPMetrics bundles the standard per-route request metrics.
type HTTPMetrics struct {
	requests *CounterVec
	latency  *HistogramVec
}

// NewHTTPMetrics registers http_requests_total and http_request_duration_seconds.
func NewHTTPMetrics(r *Registry) *HTTPMetrics {
	return &HTTPMetrics{
		requests: r.NewCounterVec("http_requests_total", "HTTP requests by route and status code.", "route", "code"),
		latency:  r.NewHistogramVec("http_request_duration_seconds", "HTTP request latency by route.", DefaultLatencyBuckets, "route"),
	}
}

// Observe records one request.
func (m *HTTPMetrics) Observe(route string, status int, d time.Duration) {
	m.requests.Inc(route, strconv.Itoa(status))
	m.latency.Observe(d.Seconds(), route)
}

// ---- helpers ----

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func braces(k string) string {
	if k == "" {
		return ""
	}
	return "{" + k + "}"
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

