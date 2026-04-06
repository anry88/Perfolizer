package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"perfolizer/pkg/config"
	"perfolizer/pkg/core"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
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

	d := expfmt.NewDecoder(r, expfmt.FmtText)
	for {
		var mf dto.MetricFamily
		if err := d.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			return out, fmt.Errorf("decode prometheus metrics: %w", err)
		}

		name := mf.GetName()
		for _, m := range mf.Metric {
			labels := make(map[string]string)
			for _, lp := range m.Label {
				labels[lp.GetName()] = lp.GetValue()
			}

			var value float64
			if m.Gauge != nil {
				value = m.GetGauge().GetValue()
			} else if m.Counter != nil {
				value = m.GetCounter().GetValue()
			} else if m.Untyped != nil {
				value = m.GetUntyped().GetValue()
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

			metric := out.Data[sampler]
			switch name {
			case "perfolizer_rps":
				metric.RPS = value
			case "perfolizer_avg_response_time_ms":
				metric.AvgLatency = value
			case "perfolizer_errors":
				metric.Errors = int(value)
			case "perfolizer_requests_total":
				metric.TotalRequests = int(value)
			case "perfolizer_errors_total":
				metric.TotalErrors = int(value)
			}
			out.Data[sampler] = metric
		}
	}

	if _, ok := out.Data["Total"]; !ok {
		out.Data["Total"] = core.Metric{}
	}

	return out, nil
}
