package ui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"perfolizer/pkg/config"
	"perfolizer/pkg/core"
	"strconv"
	"strings"
	"time"
)

type AgentClient struct {
	baseURL    string
	httpClient *http.Client
}

type AgentHostMetrics struct {
	CPUUtilizationPercent float64
	MemoryTotalBytes      uint64
	MemoryUsedBytes       uint64
	MemoryUsedPercent     float64
	DiskPath              string
	DiskTotalBytes        uint64
	DiskUsedBytes         uint64
	DiskUsedPercent       float64
}

type AgentMetricsSnapshot struct {
	Data    map[string]core.Metric
	Running bool
	Host    AgentHostMetrics
}

type RestartProcessRequest struct {
	Command string `json:"command,omitempty"`
}

func NewAgentClient(baseURL string) *AgentClient {
	return &AgentClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func NewAgentClientFromConfig() (*AgentClient, config.AgentConfig, error) {
	cfgPath := config.ResolveAgentConfigPath()
	cfg, err := config.LoadAgentConfig(cfgPath)
	if err != nil {
		return nil, cfg, err
	}
	client := NewAgentClient(cfg.BaseURL())
	return client, cfg, nil
}

func (c *AgentClient) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

func (c *AgentClient) RunTest(plan core.TestElement) error {
	payload, err := core.MarshalTestPlan(plan)
	if err != nil {
		return fmt.Errorf("marshal test plan: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/run", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create run request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send run request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	return nil
}

func (c *AgentClient) StopTest() error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/stop", nil)
	if err != nil {
		return fmt.Errorf("create stop request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send stop request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	return nil
}

func (c *AgentClient) FetchMetrics() (map[string]core.Metric, bool, error) {
	snapshot, err := c.FetchSnapshot()
	if err != nil {
		return nil, false, err
	}
	return snapshot.Data, snapshot.Running, nil
}

func (c *AgentClient) FetchSnapshot() (AgentMetricsSnapshot, error) {
	var out AgentMetricsSnapshot

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/metrics", nil)
	if err != nil {
		return out, fmt.Errorf("create metrics request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("send metrics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	snapshot, err := parsePrometheusSnapshot(resp.Body)
	if err != nil {
		return out, err
	}
	return snapshot, nil
}

func (c *AgentClient) DebugHTTP(request core.DebugHTTPRequest) (core.DebugHTTPExchange, error) {
	var exchange core.DebugHTTPExchange

	payload, err := json.Marshal(request)
	if err != nil {
		return exchange, fmt.Errorf("marshal debug request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/debug/http", bytes.NewReader(payload))
	if err != nil {
		return exchange, fmt.Errorf("create debug request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return exchange, fmt.Errorf("send debug request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return exchange, fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&exchange); err != nil {
		return exchange, fmt.Errorf("decode debug response: %w", err)
	}

	return exchange, nil
}

func (c *AgentClient) RestartProcess(command, adminToken string) error {
	payload := RestartProcessRequest{
		Command: strings.TrimSpace(command),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal restart process payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/admin/restart", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create restart process request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(adminToken); token != "" {
		req.Header.Set("X-Perfolizer-Admin-Token", token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send restart process request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	return nil
}

func parsePrometheusMetrics(r io.Reader) (map[string]core.Metric, bool, error) {
	snapshot, err := parsePrometheusSnapshot(r)
	if err != nil {
		return nil, false, err
	}
	return snapshot.Data, snapshot.Running, nil
}

func parsePrometheusSnapshot(r io.Reader) (AgentMetricsSnapshot, error) {
	out := AgentMetricsSnapshot{
		Data: make(map[string]core.Metric),
	}

	metrics := make(map[string]core.Metric)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		spec := parts[0]
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}

		name, labels, err := parseMetricWithLabels(spec)
		if err != nil {
			continue
		}
		sampler := labels["sampler"]

		if name == "perfolizer_test_running" {
			out.Running = value > 0
			continue
		}

		switch name {
		case "perfolizer_host_cpu_utilization_percent":
			out.Host.CPUUtilizationPercent = value
		case "perfolizer_host_memory_total_bytes":
			out.Host.MemoryTotalBytes = uint64(value)
		case "perfolizer_host_memory_used_bytes":
			out.Host.MemoryUsedBytes = uint64(value)
		case "perfolizer_host_memory_used_percent":
			out.Host.MemoryUsedPercent = value
		case "perfolizer_host_disk_total_bytes":
			out.Host.DiskTotalBytes = uint64(value)
			if path, ok := labels["path"]; ok {
				out.Host.DiskPath = path
			}
		case "perfolizer_host_disk_used_bytes":
			out.Host.DiskUsedBytes = uint64(value)
			if path, ok := labels["path"]; ok {
				out.Host.DiskPath = path
			}
		case "perfolizer_host_disk_used_percent":
			out.Host.DiskUsedPercent = value
			if path, ok := labels["path"]; ok {
				out.Host.DiskPath = path
			}
		}

		if sampler == "" {
			continue
		}

		m := metrics[sampler]
		switch name {
		case "perfolizer_rps":
			m.RPS = value
		case "perfolizer_avg_response_time_ms":
			m.AvgLatency = value
		case "perfolizer_errors":
			m.Errors = int(value)
		case "perfolizer_requests_total":
			m.TotalRequests = int(value)
		case "perfolizer_errors_total":
			m.TotalErrors = int(value)
		}
		metrics[sampler] = m
	}

	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read metrics: %w", err)
	}

	if _, ok := metrics["Total"]; !ok {
		metrics["Total"] = core.Metric{}
	}

	out.Data = metrics
	return out, nil
}

func parseMetricSpec(spec string) (string, string, error) {
	name, labels, err := parseMetricWithLabels(spec)
	if err != nil {
		return "", "", err
	}
	return name, labels["sampler"], nil
}

func parseMetricWithLabels(spec string) (string, map[string]string, error) {
	open := strings.IndexByte(spec, '{')
	if open == -1 {
		return spec, map[string]string{}, nil
	}

	close := strings.LastIndexByte(spec, '}')
	if close == -1 || close < open {
		return "", nil, fmt.Errorf("invalid metric labels: %s", spec)
	}

	name := spec[:open]
	labelSet := spec[open+1 : close]
	if labelSet == "" {
		return name, map[string]string{}, nil
	}
	labels := make(map[string]string)

	for _, pair := range strings.Split(labelSet, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		unquoted, err := strconv.Unquote(val)
		if err != nil {
			return name, nil, err
		}
		labels[key] = unquoted
	}

	return name, labels, nil
}
