package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"perfolizer/pkg/core"
	_ "perfolizer/pkg/elements"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxPlanBodyBytes = 10 << 20 // 10 MiB
const maxDebugPayloadBytes = 2 << 20
const maxDebugBodyBytes = 1 << 20 // 1 MiB

var ErrAlreadyRunning = errors.New("test is already running")

type Server struct {
	mu sync.RWMutex

	running bool
	cancel  context.CancelFunc
	stats   *core.StatsRunner

	httpClient *http.Client
	hostStats  *hostMetricsCollector
}

func NewServer() *Server {
	return &Server{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		hostStats: newHostMetricsCollector(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", s.handleRun)
	mux.HandleFunc("/stop", s.handleStop)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/debug/http", s.handleDebugHTTP)
	mux.HandleFunc("/healthz", s.handleHealthz)
	return mux
}

func (s *Server) Start(plan core.TestElement) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.stats = core.NewStatsRunner(ctx, nil)
	s.running = true
	s.cancel = cancel
	stats := s.stats
	s.mu.Unlock()

	go func() {
		runPlan(ctx, plan, stats)
		cancel()
		s.setStopped(stats)
	}()

	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.running = false
	s.cancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (s *Server) Snapshot() (bool, map[string]core.Metric) {
	s.mu.RLock()
	running := s.running
	stats := s.stats
	s.mu.RUnlock()

	if stats == nil {
		return running, map[string]core.Metric{"Total": {}}
	}
	return running, stats.Snapshot()
}

func (s *Server) setStopped(stats *core.StatsRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stats == stats {
		s.running = false
		s.cancel = nil
	}
}

func runPlan(ctx context.Context, plan core.TestElement, runner core.Runner) {
	var wg sync.WaitGroup

	for _, child := range plan.GetChildren() {
		if !child.Enabled() {
			continue
		}
		tg, ok := child.(core.ThreadGroup)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(group core.ThreadGroup) {
			defer wg.Done()
			group.Start(ctx, runner)
		}(tg)
	}

	wg.Wait()
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPlanBodyBytes)
	defer r.Body.Close()

	plan, err := core.ReadTestPlan(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid test plan: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.Start(plan); err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("started"))
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.Stop()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("stopped"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	running, snapshot := s.Snapshot()
	hostMetrics := hostMetricsSnapshot{}
	if s.hostStats != nil {
		hostMetrics = s.hostStats.collect()
	}
	metrics := renderPrometheusMetrics(running, snapshot, hostMetrics)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = io.WriteString(w, metrics)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleDebugHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDebugPayloadBytes)
	defer r.Body.Close()

	var debugReq core.DebugHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&debugReq); err != nil {
		http.Error(w, fmt.Sprintf("invalid debug request payload: %v", err), http.StatusBadRequest)
		return
	}

	method := strings.ToUpper(strings.TrimSpace(debugReq.Method))
	if method == "" {
		method = http.MethodGet
	}

	exchange := core.DebugHTTPExchange{
		Request: core.DebugHTTPRequest{
			Method: method,
			URL:    debugReq.URL,
		},
	}

	requestBody := trimBody(debugReq.Body, maxDebugBodyBytes)
	exchange.Request.Body = requestBody.body
	exchange.RequestBodyTruncated = requestBody.truncated

	req, err := http.NewRequest(method, debugReq.URL, bytes.NewBufferString(requestBody.body))
	if err != nil {
		exchange.Error = err.Error()
		writeDebugJSON(w, http.StatusOK, exchange)
		return
	}

	for key, values := range debugReq.Headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	exchange.Request.Headers = cloneHeaders(req.Header)

	started := time.Now()
	resp, err := s.httpClient.Do(req)
	exchange.DurationMilliseconds = time.Since(started).Milliseconds()
	if err != nil {
		exchange.Error = err.Error()
		writeDebugJSON(w, http.StatusOK, exchange)
		return
	}
	defer resp.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxDebugBodyBytes+1))
	if readErr != nil {
		exchange.Error = readErr.Error()
		writeDebugJSON(w, http.StatusOK, exchange)
		return
	}

	exchange.Response = &core.DebugHTTPResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    cloneHeaders(resp.Header),
		Body:       string(responseBody),
	}

	if len(responseBody) > maxDebugBodyBytes {
		exchange.ResponseBodyTruncated = true
		exchange.Response.Body = string(responseBody[:maxDebugBodyBytes])
	}

	writeDebugJSON(w, http.StatusOK, exchange)
}

func writeDebugJSON(w http.ResponseWriter, status int, payload core.DebugHTTPExchange) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func cloneHeaders(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		cp := make([]string, len(values))
		copy(cp, values)
		out[key] = cp
	}
	return out
}

type bodySlice struct {
	body      string
	truncated bool
}

func trimBody(body string, maxLen int) bodySlice {
	if len(body) <= maxLen {
		return bodySlice{body: body}
	}
	return bodySlice{
		body:      body[:maxLen],
		truncated: true,
	}
}

func renderPrometheusMetrics(running bool, snapshot map[string]core.Metric, host hostMetricsSnapshot) string {
	var b strings.Builder

	b.WriteString("# HELP perfolizer_test_running Test running state (1=running, 0=idle).\n")
	b.WriteString("# TYPE perfolizer_test_running gauge\n")
	if running {
		b.WriteString("perfolizer_test_running 1\n")
	} else {
		b.WriteString("perfolizer_test_running 0\n")
	}

	b.WriteString("# HELP perfolizer_rps Requests per second per sampler in the latest stats window.\n")
	b.WriteString("# TYPE perfolizer_rps gauge\n")
	b.WriteString("# HELP perfolizer_avg_response_time_ms Average response time in milliseconds in the latest stats window.\n")
	b.WriteString("# TYPE perfolizer_avg_response_time_ms gauge\n")
	b.WriteString("# HELP perfolizer_errors Errors in the latest stats window.\n")
	b.WriteString("# TYPE perfolizer_errors gauge\n")
	b.WriteString("# HELP perfolizer_requests_total Total request count since test start.\n")
	b.WriteString("# TYPE perfolizer_requests_total counter\n")
	b.WriteString("# HELP perfolizer_errors_total Total error count since test start.\n")
	b.WriteString("# TYPE perfolizer_errors_total counter\n")

	samplers := make([]string, 0, len(snapshot))
	for sampler := range snapshot {
		samplers = append(samplers, sampler)
	}
	sort.Strings(samplers)

	for _, sampler := range samplers {
		metric := snapshot[sampler]
		label := strconv.Quote(sampler)

		fmt.Fprintf(&b, "perfolizer_rps{sampler=%s} %.6f\n", label, metric.RPS)
		fmt.Fprintf(&b, "perfolizer_avg_response_time_ms{sampler=%s} %.6f\n", label, metric.AvgLatency)
		fmt.Fprintf(&b, "perfolizer_errors{sampler=%s} %d\n", label, metric.Errors)
		fmt.Fprintf(&b, "perfolizer_requests_total{sampler=%s} %d\n", label, metric.TotalRequests)
		fmt.Fprintf(&b, "perfolizer_errors_total{sampler=%s} %d\n", label, metric.TotalErrors)
	}

	appendHostMetrics(&b, host)

	return b.String()
}

func appendHostMetrics(b *strings.Builder, host hostMetricsSnapshot) {
	b.WriteString("# HELP perfolizer_host_cpu_idle_percent Host CPU idle time percent.\n")
	b.WriteString("# TYPE perfolizer_host_cpu_idle_percent gauge\n")
	b.WriteString("# HELP perfolizer_host_cpu_user_percent Host CPU user time percent.\n")
	b.WriteString("# TYPE perfolizer_host_cpu_user_percent gauge\n")
	b.WriteString("# HELP perfolizer_host_cpu_system_percent Host CPU system time percent.\n")
	b.WriteString("# TYPE perfolizer_host_cpu_system_percent gauge\n")
	b.WriteString("# HELP perfolizer_host_cpu_utilization_percent Host CPU utilization percent.\n")
	b.WriteString("# TYPE perfolizer_host_cpu_utilization_percent gauge\n")
	if host.CPUAvailable {
		fmt.Fprintf(b, "perfolizer_host_cpu_idle_percent %.6f\n", host.CPUIdlePercent)
		fmt.Fprintf(b, "perfolizer_host_cpu_user_percent %.6f\n", host.CPUUserPercent)
		fmt.Fprintf(b, "perfolizer_host_cpu_system_percent %.6f\n", host.CPUSystemPercent)
		fmt.Fprintf(b, "perfolizer_host_cpu_utilization_percent %.6f\n", host.CPUUtilizationPct)
	}

	b.WriteString("# HELP perfolizer_host_context_switches_total Host context switches total (if supported).\n")
	b.WriteString("# TYPE perfolizer_host_context_switches_total counter\n")
	if host.HasContextSwitches {
		fmt.Fprintf(b, "perfolizer_host_context_switches_total %d\n", host.ContextSwitchesTotal)
	}

	b.WriteString("# HELP perfolizer_host_cpu_throttled_total CPU throttled periods total from cgroup stats (if available).\n")
	b.WriteString("# TYPE perfolizer_host_cpu_throttled_total counter\n")
	if host.HasThrottledTotal {
		fmt.Fprintf(b, "perfolizer_host_cpu_throttled_total %d\n", host.ThrottledTotal)
	}

	b.WriteString("# HELP perfolizer_host_cpu_throttled_seconds_total CPU throttled time total in seconds (if available).\n")
	b.WriteString("# TYPE perfolizer_host_cpu_throttled_seconds_total counter\n")
	if host.HasThrottledSeconds {
		fmt.Fprintf(b, "perfolizer_host_cpu_throttled_seconds_total %.6f\n", host.ThrottledSeconds)
	}

	b.WriteString("# HELP perfolizer_host_memory_total_bytes Host memory total bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_total_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_used_bytes Host memory used bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_used_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_free_bytes Host memory free bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_free_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_available_bytes Host memory available bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_available_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_cached_bytes Host memory cached bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_cached_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_buffers_bytes Host memory buffers bytes.\n")
	b.WriteString("# TYPE perfolizer_host_memory_buffers_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_memory_used_percent Host memory utilization percent.\n")
	b.WriteString("# TYPE perfolizer_host_memory_used_percent gauge\n")
	if host.MemoryAvailable {
		fmt.Fprintf(b, "perfolizer_host_memory_total_bytes %d\n", host.MemoryTotalBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_used_bytes %d\n", host.MemoryUsedBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_free_bytes %d\n", host.MemoryFreeBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_available_bytes %d\n", host.MemoryAvailableBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_cached_bytes %d\n", host.MemoryCachedBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_buffers_bytes %d\n", host.MemoryBuffersBytes)
		fmt.Fprintf(b, "perfolizer_host_memory_used_percent %.6f\n", host.MemoryUsedPercent)
	}

	b.WriteString("# HELP perfolizer_host_swap_total_bytes Host swap total bytes.\n")
	b.WriteString("# TYPE perfolizer_host_swap_total_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_swap_used_bytes Host swap used bytes.\n")
	b.WriteString("# TYPE perfolizer_host_swap_used_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_swap_free_bytes Host swap free bytes.\n")
	b.WriteString("# TYPE perfolizer_host_swap_free_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_swap_used_percent Host swap used percent.\n")
	b.WriteString("# TYPE perfolizer_host_swap_used_percent gauge\n")
	b.WriteString("# HELP perfolizer_host_swap_in_bytes_total Host swap in bytes total.\n")
	b.WriteString("# TYPE perfolizer_host_swap_in_bytes_total counter\n")
	b.WriteString("# HELP perfolizer_host_swap_out_bytes_total Host swap out bytes total.\n")
	b.WriteString("# TYPE perfolizer_host_swap_out_bytes_total counter\n")
	if host.SwapAvailable {
		fmt.Fprintf(b, "perfolizer_host_swap_total_bytes %d\n", host.SwapTotalBytes)
		fmt.Fprintf(b, "perfolizer_host_swap_used_bytes %d\n", host.SwapUsedBytes)
		fmt.Fprintf(b, "perfolizer_host_swap_free_bytes %d\n", host.SwapFreeBytes)
		fmt.Fprintf(b, "perfolizer_host_swap_used_percent %.6f\n", host.SwapUsedPercent)
		fmt.Fprintf(b, "perfolizer_host_swap_in_bytes_total %d\n", host.SwapInBytesTotal)
		fmt.Fprintf(b, "perfolizer_host_swap_out_bytes_total %d\n", host.SwapOutBytesTotal)
	}

	b.WriteString("# HELP perfolizer_host_memory_page_faults_total Host memory page faults total (if supported).\n")
	b.WriteString("# TYPE perfolizer_host_memory_page_faults_total counter\n")
	if host.HasPageFaults {
		fmt.Fprintf(b, "perfolizer_host_memory_page_faults_total %d\n", host.PageFaultsTotal)
	}

	b.WriteString("# HELP perfolizer_host_memory_major_page_faults_total Host memory major page faults total (if supported).\n")
	b.WriteString("# TYPE perfolizer_host_memory_major_page_faults_total counter\n")
	if host.HasMajorPageFaults {
		fmt.Fprintf(b, "perfolizer_host_memory_major_page_faults_total %d\n", host.MajorPageFaultsTotal)
	}

	b.WriteString("# HELP perfolizer_host_memory_page_in_total Host memory pages paged in total (if supported).\n")
	b.WriteString("# TYPE perfolizer_host_memory_page_in_total counter\n")
	if host.HasPageIn {
		fmt.Fprintf(b, "perfolizer_host_memory_page_in_total %d\n", host.PageInTotal)
	}

	b.WriteString("# HELP perfolizer_host_memory_page_out_total Host memory pages paged out total (if supported).\n")
	b.WriteString("# TYPE perfolizer_host_memory_page_out_total counter\n")
	if host.HasPageOut {
		fmt.Fprintf(b, "perfolizer_host_memory_page_out_total %d\n", host.PageOutTotal)
	}

	pathLabel := strconv.Quote(host.DiskPath)
	b.WriteString("# HELP perfolizer_host_disk_total_bytes Host disk total bytes for selected path.\n")
	b.WriteString("# TYPE perfolizer_host_disk_total_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_disk_used_bytes Host disk used bytes for selected path.\n")
	b.WriteString("# TYPE perfolizer_host_disk_used_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_disk_free_bytes Host disk free bytes for selected path.\n")
	b.WriteString("# TYPE perfolizer_host_disk_free_bytes gauge\n")
	b.WriteString("# HELP perfolizer_host_disk_used_percent Host disk utilization percent for selected path.\n")
	b.WriteString("# TYPE perfolizer_host_disk_used_percent gauge\n")
	if host.DiskAvailable {
		fmt.Fprintf(b, "perfolizer_host_disk_total_bytes{path=%s} %d\n", pathLabel, host.DiskTotalBytes)
		fmt.Fprintf(b, "perfolizer_host_disk_used_bytes{path=%s} %d\n", pathLabel, host.DiskUsedBytes)
		fmt.Fprintf(b, "perfolizer_host_disk_free_bytes{path=%s} %d\n", pathLabel, host.DiskFreeBytes)
		fmt.Fprintf(b, "perfolizer_host_disk_used_percent{path=%s} %.6f\n", pathLabel, host.DiskUsedPercent)
	}

	b.WriteString("# HELP perfolizer_host_disk_read_bytes_total Host disk read bytes total across visible devices.\n")
	b.WriteString("# TYPE perfolizer_host_disk_read_bytes_total counter\n")
	b.WriteString("# HELP perfolizer_host_disk_write_bytes_total Host disk write bytes total across visible devices.\n")
	b.WriteString("# TYPE perfolizer_host_disk_write_bytes_total counter\n")
	b.WriteString("# HELP perfolizer_host_disk_read_ops_total Host disk read operations total across visible devices.\n")
	b.WriteString("# TYPE perfolizer_host_disk_read_ops_total counter\n")
	b.WriteString("# HELP perfolizer_host_disk_write_ops_total Host disk write operations total across visible devices.\n")
	b.WriteString("# TYPE perfolizer_host_disk_write_ops_total counter\n")
	b.WriteString("# HELP perfolizer_host_disk_io_time_seconds_total Host disk io busy time total across visible devices.\n")
	b.WriteString("# TYPE perfolizer_host_disk_io_time_seconds_total counter\n")
	b.WriteString("# HELP perfolizer_host_disk_utilization_percent Host disk utilization percent derived from io_time deltas.\n")
	b.WriteString("# TYPE perfolizer_host_disk_utilization_percent gauge\n")
	fmt.Fprintf(b, "perfolizer_host_disk_read_bytes_total %d\n", host.DiskReadBytesTotal)
	fmt.Fprintf(b, "perfolizer_host_disk_write_bytes_total %d\n", host.DiskWriteBytesTotal)
	fmt.Fprintf(b, "perfolizer_host_disk_read_ops_total %d\n", host.DiskReadOpsTotal)
	fmt.Fprintf(b, "perfolizer_host_disk_write_ops_total %d\n", host.DiskWriteOpsTotal)
	if host.HasDiskIOTime {
		fmt.Fprintf(b, "perfolizer_host_disk_io_time_seconds_total %.6f\n", host.DiskIOTimeSeconds)
	}
	if host.HasDiskUtilization {
		fmt.Fprintf(b, "perfolizer_host_disk_utilization_percent %.6f\n", host.DiskUtilizationPct)
	}
}
