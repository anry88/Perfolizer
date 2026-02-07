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
}

func NewServer() *Server {
	return &Server{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
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
	metrics := renderPrometheusMetrics(running, snapshot)

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

func renderPrometheusMetrics(running bool, snapshot map[string]core.Metric) string {
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

	return b.String()
}
