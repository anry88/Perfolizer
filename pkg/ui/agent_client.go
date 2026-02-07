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

func NewAgentClientFromConfig() (*AgentClient, config.AgentConfig, error) {
	cfgPath := config.ResolveAgentConfigPath()
	cfg, err := config.LoadAgentConfig(cfgPath)
	if err != nil {
		return nil, cfg, err
	}
	client := &AgentClient{
		baseURL: cfg.BaseURL(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	return client, cfg, nil
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
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/metrics", nil)
	if err != nil {
		return nil, false, fmt.Errorf("create metrics request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("send metrics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("agent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	data, running, err := parsePrometheusMetrics(resp.Body)
	if err != nil {
		return nil, false, err
	}
	return data, running, nil
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

func parsePrometheusMetrics(r io.Reader) (map[string]core.Metric, bool, error) {
	metrics := make(map[string]core.Metric)
	running := false

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

		name, sampler, err := parseMetricSpec(spec)
		if err != nil {
			continue
		}

		if name == "perfolizer_test_running" {
			running = value > 0
			continue
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
		return nil, false, fmt.Errorf("read metrics: %w", err)
	}

	if _, ok := metrics["Total"]; !ok {
		metrics["Total"] = core.Metric{}
	}

	return metrics, running, nil
}

func parseMetricSpec(spec string) (string, string, error) {
	open := strings.IndexByte(spec, '{')
	if open == -1 {
		return spec, "", nil
	}

	close := strings.LastIndexByte(spec, '}')
	if close == -1 || close < open {
		return "", "", fmt.Errorf("invalid metric labels: %s", spec)
	}

	name := spec[:open]
	labelSet := spec[open+1 : close]
	if labelSet == "" {
		return name, "", nil
	}

	for _, pair := range strings.Split(labelSet, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.TrimSpace(kv[0]) != "sampler" {
			continue
		}
		val := strings.TrimSpace(kv[1])
		unquoted, err := strconv.Unquote(val)
		if err != nil {
			return name, "", err
		}
		return name, unquoted, nil
	}

	return name, "", nil
}
