package ui

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

type showcasePlan struct {
	sessionSampler *elements.HttpSampler
}

// GenerateShowcaseScreenshots renders stable README screenshots into the provided directory.
func GenerateShowcaseScreenshots(a fyne.App, outputDir string) error {
	if a == nil {
		return fmt.Errorf("fyne app is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}

	pa := newPerfolizerApp(a)
	pa.Window.Resize(fyne.NewSize(1440, 920))
	pa.Window.Show()

	plan := configureShowcaseProject(pa)
	if err := saveWindowCapture(pa.Window, filepath.Join(outputDir, "editor-overview.png")); err != nil {
		return err
	}

	populateDebugShowcase(pa, plan)
	if err := saveWindowCapture(pa.Window, filepath.Join(outputDir, "debug-console.png")); err != nil {
		return err
	}

	dashboard := newShowcaseDashboardWindow(a)
	defer dashboard.Window.Close()
	if err := saveWindowCapture(dashboard.Window, filepath.Join(outputDir, "runtime-dashboard.png")); err != nil {
		return err
	}

	settingsWindow, cleanup, err := newShowcaseSettingsWindow(pa)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := saveWindowCapture(settingsWindow, filepath.Join(outputDir, "agent-settings.png")); err != nil {
		return err
	}

	return nil
}

func configureShowcaseProject(pa *PerfolizerApp) showcasePlan {
	root := core.NewBaseElement("Checkout API Flow")

	rpsGroup := elements.NewRPSThreadGroup("Checkout RPS Thread Group", 120)
	rpsGroup.Users = 24
	rpsGroup.ProfileBlocks = []elements.RPSProfileBlock{
		{RampUp: 10 * time.Second, StepDuration: 45 * time.Second, ProfilePercent: 50},
		{RampUp: 15 * time.Second, StepDuration: 60 * time.Second, ProfilePercent: 100},
		{RampUp: 10 * time.Second, StepDuration: 30 * time.Second, ProfilePercent: 140},
	}
	rpsGroup.GracefulShutdown = 5 * time.Second

	sessionSampler := elements.NewHttpSampler("POST /api/session", http.MethodPost, "https://demo.perfolizer.local/api/session")
	sessionSampler.TargetRPS = 48
	sessionSampler.Body = "{\n  \"email\": \"${user_email}\",\n  \"password\": \"${user_password}\"\n}"
	sessionSampler.ExtractVars = []string{"session_token", "catalog_version", "product_id"}

	browseLoop := elements.NewLoopController("Browse Catalog", 2)

	catalogSampler := elements.NewHttpSampler("GET /api/catalog", http.MethodGet, "https://demo.perfolizer.local/api/catalog?version=${catalog_version}")
	catalogSampler.TargetRPS = 72

	productSampler := elements.NewHttpSampler("GET /api/products/${product_id}", http.MethodGet, "https://demo.perfolizer.local/api/products/${product_id}")
	productSampler.TargetRPS = 32

	pause := elements.NewPauseController("Think Time", 250*time.Millisecond)

	browseLoop.AddChild(catalogSampler)
	browseLoop.AddChild(pause)
	browseLoop.AddChild(productSampler)

	rpsGroup.AddChild(sessionSampler)
	rpsGroup.AddChild(browseLoop)

	smokeGroup := elements.NewSimpleThreadGroup("Warmup Thread Group", 2, 1)
	smokeSampler := elements.NewHttpSampler("GET /healthz", http.MethodGet, "https://demo.perfolizer.local/healthz")
	smokeGroup.AddChild(smokeSampler)

	root.AddChild(rpsGroup)
	root.AddChild(smokeGroup)

	project := core.NewProject("Perfolizer Showcase")
	project.AddPlan("Checkout API Flow", &root)
	project.Plans[0].Parameters = []core.Parameter{
		{ID: "param-user-email", Name: "user_email", Type: core.ParamTypeStatic, Value: "showcase@perfolizer.dev"},
		{ID: "param-user-password", Name: "user_password", Type: core.ParamTypeStatic, Value: "demo-password"},
		{ID: "param-session-token", Name: "session_token", Type: core.ParamTypeRegexp, Value: "fallback-session", Expression: "\"token\":\"([^\"]+)\""},
		{ID: "param-catalog-version", Name: "catalog_version", Type: core.ParamTypeJSON, Value: "2026.03.19", Expression: "catalog.version"},
		{ID: "param-product-id", Name: "product_id", Type: core.ParamTypeJSON, Value: "SKU-42", Expression: "items.0.id"},
	}

	pa.Project = project
	pa.CurrentNodeID = ""
	pa.Tree.Refresh()
	pa.Tree.OpenAllBranches()
	if pa.ParameterManager != nil {
		pa.ParameterManager.Refresh()
	}

	nodeID := pa.treeIDForElement(0, sessionSampler)
	pa.CurrentNodeID = nodeID
	pa.Tree.Select(nodeID)

	return showcasePlan{
		sessionSampler: sessionSampler,
	}
}

func populateDebugShowcase(pa *PerfolizerApp, plan showcasePlan) {
	exchange := &core.DebugHTTPExchange{
		Request: core.DebugHTTPRequest{
			Method: http.MethodPost,
			URL:    "https://demo.perfolizer.local/api/session",
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: "{\n  \"email\": \"showcase@perfolizer.dev\",\n  \"password\": \"demo-password\"\n}",
		},
		Response: &core.DebugHTTPResponse{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
				"X-Perfolizer-Agent": {"showcase-agent"},
			},
			Body: "{\"token\":\"sess_demo_01\",\"catalog\":{\"version\":\"2026.03.19\"},\"items\":[{\"id\":\"SKU-42\"}]}",
		},
		DurationMilliseconds: 84,
	}

	logText := strings.TrimSpace(`
Debug run started at 2026-03-19T10:30:00+01:00
Requests to execute once: 1

----------------------------------------
[1/1] Sampler: POST /api/session
Request: POST https://demo.perfolizer.local/api/session
Duration: 84 ms
Status: 200 OK

--- Outgoing Headers ---
Content-Type: application/json

--- Request Body ---
{
  "email": "showcase@perfolizer.dev",
  "password": "demo-password"
}

--- Incoming Headers ---
Content-Type: application/json
X-Perfolizer-Agent: showcase-agent

--- Response Body ---
{"token":"sess_demo_01","catalog":{"version":"2026.03.19"},"items":[{"id":"SKU-42"}]}

--- Parameter Extraction ---
Variable: session_token
  Type: Regexp
  Expression: "token":"([^"]+)"
  Result: "sess_demo_01"
Variable: catalog_version
  Type: JSON
  Path: catalog.version
  Result: "2026.03.19"
Variable: product_id
  Type: JSON
  Path: items.0.id
  Result: "SKU-42"

--- Context Variables ---
catalog_version = "2026.03.19"
product_id = "SKU-42"
session_token = "sess_demo_01"
user_email = "showcase@perfolizer.dev"
user_password = "demo-password"
----------------------------------------

Debug run finished at 2026-03-19T10:30:01+01:00
`)

	pa.clearDebugConsole()
	pa.lastDebugExchange = exchange
	pa.DebugConsoleEntry.SetText(logText)
	pa.DebugConsoleEntry.CursorRow = len(strings.Split(logText, "\n")) - 1
	pa.DebugConsoleEntry.Refresh()
	pa.debugConsoleMode = DebugModeNormal
	pa.updateDebugConsoleUI()

	nodeID := pa.treeIDForElement(0, plan.sessionSampler)
	pa.CurrentNodeID = nodeID
	pa.Tree.Select(nodeID)
}

func newShowcaseDashboardWindow(a fyne.App) *DashboardWindow {
	dashboard := NewDashboardWindow(a)
	dashboard.RpsChart.Resize(fyne.NewSize(920, 180))
	dashboard.LatChart.Resize(fyne.NewSize(920, 180))
	dashboard.ErrChart.Resize(fyne.NewSize(920, 180))
	dashboard.Window.Show()

	for i := 0; i < 18; i++ {
		offset := float64(i % 5)
		dashboard.RpsChart.Add("POST /api/session", 32+offset*2)
		dashboard.LatChart.Add("POST /api/session", 78+offset*4)
		dashboard.ErrChart.Add("POST /api/session", float64(i/9))

		dashboard.RpsChart.Add("GET /api/catalog", 46+offset*3)
		dashboard.LatChart.Add("GET /api/catalog", 54+offset*2)
		dashboard.ErrChart.Add("GET /api/catalog", 0)

		dashboard.RpsChart.Add("GET /api/products/{id}", 24+offset*2)
		dashboard.LatChart.Add("GET /api/products/{id}", 88+offset*6)
		dashboard.ErrChart.Add("GET /api/products/{id}", float64(i/10))
	}
	dashboard.RpsLabel.SetText("Total RPS: 130.00")
	dashboard.LatLabel.SetText("Avg Latency: 72.00 ms")
	dashboard.ErrLabel.SetText("Errors (total): 3")
	dashboard.Legend.Add(widget.NewCheck("POST /api/session", nil))
	dashboard.Legend.Add(widget.NewCheck("GET /api/catalog", nil))
	dashboard.Legend.Add(widget.NewCheck("GET /api/products/{id}", nil))
	for _, obj := range dashboard.Legend.Objects[1:] {
		if cb, ok := obj.(*widget.Check); ok {
			cb.SetChecked(true)
		}
	}

	return dashboard
}

func newShowcaseSettingsWindow(pa *PerfolizerApp) (fyne.Window, func(), error) {
	runningMetrics := strings.NewReader(prometheusSnapshot(
		true,
		39.2,
		14*1024*1024*1024,
		6*1024*1024*1024,
		42.8,
		"/var/load",
		256*1024*1024*1024,
		98*1024*1024*1024,
		38.3,
	))
	runningSnapshot, err := parsePrometheusSnapshot(runningMetrics)
	if err != nil {
		return nil, nil, fmt.Errorf("parse running metrics snapshot: %w", err)
	}

	idleMetrics := strings.NewReader(prometheusSnapshot(
		false,
		11.7,
		8*1024*1024*1024,
		3*1024*1024*1024,
		37.5,
		"/srv/agent",
		128*1024*1024*1024,
		46*1024*1024*1024,
		35.9,
	))
	idleSnapshot, err := parsePrometheusSnapshot(idleMetrics)
	if err != nil {
		return nil, nil, fmt.Errorf("parse idle metrics snapshot: %w", err)
	}

	transport := showcaseRoundTripper{
		metricsByHost: map[string]string{
			"showcase-local": prometheusSnapshot(
				true,
				runningSnapshot.Host.CPUUtilizationPercent,
				runningSnapshot.Host.MemoryTotalBytes,
				runningSnapshot.Host.MemoryUsedBytes,
				runningSnapshot.Host.MemoryUsedPercent,
				runningSnapshot.Host.DiskPath,
				runningSnapshot.Host.DiskTotalBytes,
				runningSnapshot.Host.DiskUsedBytes,
				runningSnapshot.Host.DiskUsedPercent,
			),
			"showcase-lab": prometheusSnapshot(
				false,
				idleSnapshot.Host.CPUUtilizationPercent,
				idleSnapshot.Host.MemoryTotalBytes,
				idleSnapshot.Host.MemoryUsedBytes,
				idleSnapshot.Host.MemoryUsedPercent,
				idleSnapshot.Host.DiskPath,
				idleSnapshot.Host.DiskTotalBytes,
				idleSnapshot.Host.DiskUsedBytes,
				idleSnapshot.Host.DiskUsedPercent,
			),
		},
	}

	pa.agents = []agentSettingsEntry{
		{
			ID:             "local-agent",
			Name:           "Local macOS agent",
			BaseURL:        "http://showcase-local",
			RestartCommand: "launchctl kickstart -k gui/$UID/com.github.anry88.perfolizer-agent",
			RestartToken:   "demo-admin-token",
		},
		{
			ID:      "linux-agent",
			Name:    "Linux lab agent",
			BaseURL: "http://showcase-lab",
		},
	}
	pa.activeAgentID = "local-agent"
	pa.agentClients = map[string]*AgentClient{
		"local-agent": {
			baseURL: "http://showcase-local",
			httpClient: &http.Client{
				Transport: transport,
			},
		},
		"linux-agent": {
			baseURL: "http://showcase-lab",
			httpClient: &http.Client{
				Transport: transport,
			},
		},
	}
	pa.updateAgentRuntimeFromSnapshot("local-agent", runningSnapshot)
	pa.markAgentRunStarted("local-agent", "Checkout API Flow", time.Date(2026, time.March, 19, 10, 30, 0, 0, time.FixedZone("CET", 3600)))
	pa.updateAgentRuntimeFromSnapshot("local-agent", runningSnapshot)
	pa.updateAgentRuntimeFromSnapshot("linux-agent", idleSnapshot)

	win := pa.FyneApp.NewWindow("Settings")
	win.Resize(fyne.NewSize(1320, 820))

	sections := []string{"General", "Shortcuts", "Agents"}
	content := container.NewMax()
	agentsPage := pa.buildAgentsPage(win)
	pages := map[string]fyne.CanvasObject{
		"General":   container.NewPadded(widget.NewCard("General", "", widget.NewLabel("General settings will appear here."))),
		"Shortcuts": pa.buildShortcutsPage(),
		"Agents":    agentsPage,
	}

	setSection := func(name string) {
		page, ok := pages[name]
		if !ok {
			return
		}
		content.Objects = []fyne.CanvasObject{page}
		content.Refresh()
	}

	sectionList := widget.NewList(
		func() int { return len(sections) },
		func() fyne.CanvasObject { return widget.NewLabel("Section") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(sections[i])
		},
	)
	sectionList.OnSelected = func(id widget.ListItemID) {
		setSection(sections[id])
	}

	layout := container.NewHSplit(
		container.NewPadded(widget.NewCard("Settings", "", sectionList)),
		container.NewPadded(content),
	)
	layout.SetOffset(0.2)
	win.SetContent(layout)
	win.Show()
	sectionList.Select(2)

	time.Sleep(50 * time.Millisecond)

	cleanup := func() {
		win.Close()
	}
	return win, cleanup, nil
}

type showcaseRoundTripper struct {
	metricsByHost map[string]string
}

func (rt showcaseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	status := http.StatusOK

	switch req.URL.Path {
	case "/metrics":
		body = rt.metricsByHost[req.URL.Host]
		if body == "" {
			status = http.StatusNotFound
			body = "missing showcase metrics"
		}
	case "/healthz":
		body = "ok"
	default:
		status = http.StatusNotFound
		body = "not found"
	}

	resp := &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	if req.URL.Path == "/metrics" {
		resp.Header.Set("Content-Type", "text/plain; version=0.0.4")
	}
	return resp, nil
}

func prometheusSnapshot(running bool, cpuPercent float64, memoryTotal, memoryUsed uint64, memoryUsedPercent float64, diskPath string, diskTotal, diskUsed uint64, diskUsedPercent float64) string {
	runningValue := 0
	if running {
		runningValue = 1
	}
	return fmt.Sprintf(`# HELP perfolizer_test_running Test running state (1=running, 0=idle).
# TYPE perfolizer_test_running gauge
perfolizer_test_running %d
perfolizer_rps{sampler="Total"} 128.5
perfolizer_avg_response_time_ms{sampler="Total"} 86.4
perfolizer_errors{sampler="Total"} 1
perfolizer_requests_total{sampler="Total"} 2048
perfolizer_errors_total{sampler="Total"} 3
perfolizer_rps{sampler="POST /api/session"} 34.1
perfolizer_avg_response_time_ms{sampler="POST /api/session"} 92.7
perfolizer_errors{sampler="POST /api/session"} 1
perfolizer_requests_total{sampler="POST /api/session"} 512
perfolizer_errors_total{sampler="POST /api/session"} 2
perfolizer_rps{sampler="GET /api/catalog"} 61.8
perfolizer_avg_response_time_ms{sampler="GET /api/catalog"} 58.2
perfolizer_errors{sampler="GET /api/catalog"} 0
perfolizer_requests_total{sampler="GET /api/catalog"} 1024
perfolizer_errors_total{sampler="GET /api/catalog"} 0
perfolizer_host_cpu_utilization_percent %.2f
perfolizer_host_memory_total_bytes %d
perfolizer_host_memory_used_bytes %d
perfolizer_host_memory_used_percent %.2f
perfolizer_host_disk_total_bytes{path=%q} %d
perfolizer_host_disk_used_bytes{path=%q} %d
perfolizer_host_disk_used_percent{path=%q} %.2f
`, runningValue, cpuPercent, memoryTotal, memoryUsed, memoryUsedPercent, diskPath, diskTotal, diskPath, diskUsed, diskPath, diskUsedPercent)
}

func saveWindowCapture(win fyne.Window, path string) error {
	if win == nil {
		return fmt.Errorf("window is nil")
	}
	return savePNG(path, win.Canvas().Capture())
}

func savePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create png directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create png file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	return nil
}
