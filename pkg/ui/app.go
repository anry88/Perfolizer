package ui

import (
	"context"
	"fmt"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const maxConsoleChars = 200000
const maxBodyPreviewChars = 20000

type PerfolizerApp struct {
	FyneApp fyne.App
	Window  fyne.Window
	Tree    *widget.Tree
	Content *fyne.Container

	DebugConsole *widget.Entry

	TestPlan      core.TestElement // Root of the plan
	CurrentNodeID string

	agentClient    *AgentClient
	agentInitError error
	pollInterval   time.Duration

	cancelFunc     context.CancelFunc
	isRunning      bool
	isDebugRunning bool
}

func NewPerfolizerApp() *PerfolizerApp {
	a := app.NewWithID("com.github.anry88.perfolizer")
	w := a.NewWindow("Perfolizer")
	w.Resize(fyne.NewSize(1024, 768))

	agentClient, cfg, cfgErr := NewAgentClientFromConfig()
	pollInterval := 15 * time.Second
	if cfg.UIPollIntervalSec > 0 {
		pollInterval = time.Duration(cfg.UIPollIntervalSec) * time.Second
	}

	pa := &PerfolizerApp{
		FyneApp: a,
		Window:  w,
		Content: container.NewMax(widget.NewLabel("Select a node to edit")),

		agentClient:    agentClient,
		agentInitError: cfgErr,
		pollInterval:   pollInterval,
	}

	pa.setupTestPlan()
	pa.setupUI()

	return pa
}

func (pa *PerfolizerApp) Run() {
	pa.Window.ShowAndRun()
}

func (pa *PerfolizerApp) setupTestPlan() {
	// Default starting plan
	root := core.NewBaseElement("Test Plan")
	// Add a default ThreadGroup
	tg := elements.NewSimpleThreadGroup("Thread Group 1", 1, 1)
	root.AddChild(tg)
	pa.TestPlan = &root
}

func (pa *PerfolizerApp) setupUI() {
	// 1. Tree View
	pa.Tree = widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID {
			// Get children IDs
			// This relies on a map or traversal system.
			// For MVP we need a way to map IDs to Elements quickly.
			// Or we recursively traverse.
			// Fyne Tree logic: Root is empty string ""?
			if id == "" {
				return []string{pa.TestPlan.ID()}
			}
			el := pa.findElementByID(pa.TestPlan, id)
			if el != nil {
				var ids []string
				for _, c := range el.GetChildren() {
					ids = append(ids, c.ID())
				}
				return ids
			}
			return nil
		},
		func(id widget.TreeNodeID) bool {
			// IsBranch
			if id == "" {
				return true
			}
			el := pa.findElementByID(pa.TestPlan, id)
			return el != nil && len(el.GetChildren()) > 0
		},
		func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("Node")
		},
		func(id widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
			el := pa.findElementByID(pa.TestPlan, id)
			if el != nil {
				o.(*widget.Label).SetText(el.Name())
			}
		},
	)

	pa.Tree.OnSelected = func(id widget.TreeNodeID) {
		pa.CurrentNodeID = id
		el := pa.findElementByID(pa.TestPlan, id)
		if el != nil {
			pa.showProperties(el)
		}
	}

	debugConsole := widget.NewMultiLineEntry()
	debugConsole.Disable()
	debugConsole.SetMinRowsVisible(12)
	debugConsole.SetPlaceHolder("Debug request logs will appear here.")
	pa.DebugConsole = debugConsole

	clearDebugButton := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		pa.clearDebugConsole()
	})
	debugPanel := container.NewBorder(
		container.NewBorder(nil, nil, widget.NewLabel("Debug Console"), clearDebugButton, nil),
		nil, nil, nil,
		container.NewPadded(debugConsole),
	)

	// 2. Toolbar (Top)
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ContentAddIcon(), func() { pa.addElement() }),       // Add
		widget.NewToolbarAction(theme.ContentRemoveIcon(), func() { pa.removeElement() }), // Remove
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.FolderOpenIcon(), func() { pa.loadTestPlan() }),
		widget.NewToolbarAction(theme.DocumentSaveIcon(), func() { pa.saveTestPlan() }),
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.MediaPlayIcon(), func() { pa.runTest() }),          // Start
		widget.NewToolbarAction(theme.SearchReplaceIcon(), func() { pa.runDebugTest() }), // Debug
		widget.NewToolbarAction(theme.MediaStopIcon(), func() { pa.stopTest() }),         // Stop
	)

	// 3. Layout
	rightSplit := container.NewVSplit(pa.Content, debugPanel)
	rightSplit.SetOffset(0.62)

	split := container.NewHSplit(
		container.NewBorder(nil, nil, nil, nil, pa.Tree),
		rightSplit,
	)
	split.SetOffset(0.3)

	mainLayout := container.NewBorder(toolbar, nil, nil, nil, split)
	pa.Window.SetContent(mainLayout)
}

// Helper to find node (DFS)
func (pa *PerfolizerApp) findElementByID(root core.TestElement, id string) core.TestElement {
	if root.ID() == id {
		return root
	}
	for _, child := range root.GetChildren() {
		found := pa.findElementByID(child, id)
		if found != nil {
			return found
		}
	}
	return nil
}

func (pa *PerfolizerApp) showProperties(el core.TestElement) {
	// Clear content
	// Dynamically build form based on type type switch
	pa.Content.Objects = nil

	nameEntry := widget.NewEntry()
	nameEntry.SetText(el.Name())
	nameEntry.OnChanged = func(s string) {
		el.SetName(s)
		pa.Tree.RefreshItem(el.ID())
	}

	form := widget.NewForm(
		widget.NewFormItem("Name", nameEntry),
	)

	// Add specific fields
	switch v := el.(type) {
	case *elements.HttpSampler:
		urlEntry := widget.NewEntry()
		urlEntry.SetText(v.Url)
		urlEntry.OnChanged = func(s string) { v.Url = s }

		methodEntry := widget.NewSelect([]string{"GET", "POST", "PUT", "DELETE"}, func(s string) { v.Method = s })
		methodEntry.SetSelected(v.Method)

		rpsEntry := widget.NewEntry()
		rpsEntry.SetText(strconv.FormatFloat(v.TargetRPS, 'f', 2, 64))
		rpsEntry.OnChanged = func(s string) {
			if val, err := strconv.ParseFloat(s, 64); err == nil {
				v.TargetRPS = val
			}
		}

		bodyEntry := widget.NewMultiLineEntry()
		bodyEntry.SetMinRowsVisible(4)
		bodyEntry.SetText(v.Body)
		bodyEntry.OnChanged = func(s string) { v.Body = s }

		form.Append("URL", urlEntry)
		form.Append("Method", methodEntry)
		form.Append("Body", bodyEntry)
		form.Append("Target RPS (0 = default)", rpsEntry)

	case *elements.SimpleThreadGroup:
		usersEntry := widget.NewEntry()
		usersEntry.SetText(strconv.Itoa(v.Users))
		usersEntry.OnChanged = func(s string) {
			if val, err := strconv.Atoi(s); err == nil {
				v.Users = val
			}
		}

		iterEntry := widget.NewEntry()
		iterEntry.SetText(strconv.Itoa(v.Iterations))
		iterEntry.OnChanged = func(s string) {
			if val, err := strconv.Atoi(s); err == nil {
				v.Iterations = val
			}
		}

		form.Append("Users", usersEntry)
		form.Append("Iterations (-1 for infinite)", iterEntry)

	case *elements.RPSThreadGroup:
		rpsEntry := widget.NewEntry()
		rpsEntry.SetText(strconv.FormatFloat(v.RPS, 'f', 2, 64))
		rpsEntry.OnChanged = func(s string) {
			if val, err := strconv.ParseFloat(s, 64); err == nil {
				v.RPS = val
			}
		}

		usersEntry := widget.NewEntry()
		usersEntry.SetText(strconv.Itoa(v.Users))
		usersEntry.OnChanged = func(s string) {
			if val, err := strconv.Atoi(s); err == nil {
				v.Users = val
			}
		}

		durationEntry := widget.NewEntry()
		durationEntry.SetText(v.Duration.String())
		durationEntry.OnChanged = func(s string) {
			if val, err := time.ParseDuration(s); err == nil {
				v.Duration = val
			}
		}

		form.Append("Target RPS", rpsEntry)
		form.Append("Max Users", usersEntry)
		form.Append("Duration", durationEntry)

	case *elements.PauseController:
		durEntry := widget.NewEntry()
		// Display in milliseconds
		durEntry.SetText(strconv.FormatInt(v.Duration.Milliseconds(), 10))
		durEntry.OnChanged = func(s string) {
			if val, err := strconv.Atoi(s); err == nil {
				v.Duration = time.Duration(val) * time.Millisecond
			}
		}

		form.Append("Duration (ms)", durEntry)
	}

	pa.Content.Objects = []fyne.CanvasObject{container.NewVBox(widget.NewLabel("Properties"), form)}
	pa.Content.Refresh()
}

func (pa *PerfolizerApp) saveTestPlan() {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, pa.Window)
			return
		}
		if writer == nil {
			return // User cancelled
		}
		defer writer.Close()

		// Use core.SaveTestPlan
		if err := core.SaveTestPlan(writer.URI().Path(), pa.TestPlan); err != nil {
			dialog.ShowError(err, pa.Window)
		}
	}, pa.Window)
}

func (pa *PerfolizerApp) loadTestPlan() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, pa.Window)
			return
		}
		if reader == nil {
			return // User cancelled
		}
		defer reader.Close()

		// Use core.LoadTestPlan
		plan, err := core.LoadTestPlan(reader.URI().Path())
		if err != nil {
			dialog.ShowError(err, pa.Window)
			return
		}
		pa.TestPlan = plan
		// Refresh Tree from root
		pa.Tree.RefreshItem("")
		// Reset
		pa.CurrentNodeID = ""
		pa.Content.Objects = nil
		pa.Content.Refresh()
		pa.Tree.Refresh()

	}, pa.Window)
}

func (pa *PerfolizerApp) runTest() {
	if pa.isRunning {
		return
	}

	if pa.agentInitError != nil {
		dialog.ShowError(fmt.Errorf("agent config error: %w", pa.agentInitError), pa.Window)
		return
	}
	if pa.agentClient == nil {
		dialog.ShowError(fmt.Errorf("agent client is not configured"), pa.Window)
		return
	}

	if err := pa.agentClient.RunTest(pa.TestPlan); err != nil {
		dialog.ShowError(err, pa.Window)
		return
	}

	if pa.cancelFunc != nil {
		pa.cancelFunc()
		pa.cancelFunc = nil
	}

	pa.isRunning = true

	dashboard := NewDashboardWindow(pa.FyneApp)
	dashboard.Show()

	ctx, cancel := context.WithCancel(context.Background())
	pa.cancelFunc = cancel

	go pa.pollAgentMetrics(ctx, dashboard)
}

func (pa *PerfolizerApp) runDebugTest() {
	if pa.isDebugRunning {
		return
	}

	if pa.agentInitError != nil {
		dialog.ShowError(fmt.Errorf("agent config error: %w", pa.agentInitError), pa.Window)
		return
	}
	if pa.agentClient == nil {
		dialog.ShowError(fmt.Errorf("agent client is not configured"), pa.Window)
		return
	}

	samplers := make([]*elements.HttpSampler, 0)
	pa.collectHTTPSamplers(pa.TestPlan, &samplers)
	if len(samplers) == 0 {
		dialog.ShowInformation("Debug run", "No HTTP samplers found in the test plan.", pa.Window)
		return
	}

	pa.isDebugRunning = true
	pa.clearDebugConsole()
	pa.appendDebugLog(fmt.Sprintf("Debug run started at %s", time.Now().Format(time.RFC3339)))
	pa.appendDebugLog(fmt.Sprintf("Requests to execute once: %d", len(samplers)))

	go pa.executeDebugRun(samplers)
}

func (pa *PerfolizerApp) executeDebugRun(samplers []*elements.HttpSampler) {
	for i, sampler := range samplers {
		pa.appendDebugLog("")
		pa.appendDebugLog(fmt.Sprintf("[%d/%d] Sampler: %s", i+1, len(samplers), sampler.Name()))

		exchange, err := pa.agentClient.DebugHTTP(core.DebugHTTPRequest{
			Method: sampler.Method,
			URL:    sampler.Url,
			Body:   sampler.Body,
		})
		if err != nil {
			pa.appendDebugLog(fmt.Sprintf("Agent call failed: %v", err))
			continue
		}

		pa.appendDebugLog(formatDebugExchange(exchange))
	}

	pa.appendDebugLog("")
	pa.appendDebugLog(fmt.Sprintf("Debug run finished at %s", time.Now().Format(time.RFC3339)))

	fyne.Do(func() {
		pa.isDebugRunning = false
	})
}

func (pa *PerfolizerApp) collectHTTPSamplers(root core.TestElement, out *[]*elements.HttpSampler) {
	if sampler, ok := root.(*elements.HttpSampler); ok {
		*out = append(*out, sampler)
	}
	for _, child := range root.GetChildren() {
		pa.collectHTTPSamplers(child, out)
	}
}

func (pa *PerfolizerApp) clearDebugConsole() {
	if pa.DebugConsole == nil {
		return
	}
	fyne.Do(func() {
		pa.DebugConsole.SetText("")
	})
}

func (pa *PerfolizerApp) appendDebugLog(line string) {
	if pa.DebugConsole == nil {
		return
	}
	fyne.Do(func() {
		text := line
		if pa.DebugConsole.Text != "" {
			text = pa.DebugConsole.Text + "\n" + line
		}
		if len(text) > maxConsoleChars {
			text = "[console output truncated]\n" + text[len(text)-maxConsoleChars:]
		}
		pa.DebugConsole.SetText(text)
	})
}

func (pa *PerfolizerApp) stopTest() {
	if pa.cancelFunc != nil {
		pa.cancelFunc()
		pa.cancelFunc = nil
	}
	pa.isRunning = false

	if pa.agentClient == nil {
		return
	}

	if err := pa.agentClient.StopTest(); err != nil {
		dialog.ShowError(err, pa.Window)
	}
}

func (pa *PerfolizerApp) pollAgentMetrics(ctx context.Context, dashboard *DashboardWindow) {
	pa.pollOnce(dashboard)

	ticker := time.NewTicker(pa.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pa.pollOnce(dashboard)
		}
	}
}

func (pa *PerfolizerApp) pollOnce(dashboard *DashboardWindow) {
	data, running, err := pa.agentClient.FetchMetrics()
	if err != nil {
		return
	}

	dashboard.Update(data)
	if !running {
		pa.isRunning = false
		if pa.cancelFunc != nil {
			pa.cancelFunc()
			pa.cancelFunc = nil
		}
	}
}

// Helper to find parent of a node (DFS)
func (pa *PerfolizerApp) findParent(root core.TestElement, childID string) core.TestElement {
	for _, child := range root.GetChildren() {
		if child.ID() == childID {
			return root
		}
		found := pa.findParent(child, childID)
		if found != nil {
			return found
		}
	}
	return nil
}

func (pa *PerfolizerApp) addElement() {
	var parent core.TestElement
	if pa.CurrentNodeID != "" {
		parent = pa.findElementByID(pa.TestPlan, pa.CurrentNodeID)
	} else {
		parent = pa.TestPlan
	}

	if parent == nil {
		return
	}

	// Simple dialog with buttons for now
	d := dialog.NewCustom("Select Element Type", "Cancel",
		container.NewVBox(
			widget.NewButton("Simple Thread Group", func() { pa.doAddElement(parent, "Simple Thread Group") }),
			widget.NewButton("RPS Thread Group", func() { pa.doAddElement(parent, "RPS Thread Group") }),
			widget.NewButton("HTTP Sampler", func() { pa.doAddElement(parent, "HTTP Sampler") }),
			widget.NewButton("If Controller", func() { pa.doAddElement(parent, "If Controller") }),
			widget.NewButton("Pause Controller", func() { pa.doAddElement(parent, "Pause Controller") }),
		), pa.Window)
	d.Show()
}

func (pa *PerfolizerApp) doAddElement(parent core.TestElement, typeName string) {
	pa.Window.Canvas().Overlays().Top().Hide() // Close info/dialog

	var newEl core.TestElement
	switch typeName {
	case "Simple Thread Group":
		newEl = elements.NewSimpleThreadGroup("Thread Group", 1, 1)
	case "RPS Thread Group":
		newEl = elements.NewRPSThreadGroup("RPS Group", 10.0, 60*1000000000)
	case "HTTP Sampler":
		newEl = &elements.HttpSampler{BaseElement: core.NewBaseElement("HTTP Request"), Method: "GET", Url: "http://localhost"}
	case "If Controller":
		newEl = elements.NewIfController("If Controller", func(ctx *core.Context) bool { return true })
	case "Pause Controller":
		newEl = &elements.PauseController{BaseElement: core.NewBaseElement("Pause"), Duration: 1000}
	}

	if newEl != nil {
		parent.AddChild(newEl)
		pa.Tree.RefreshItem(parent.ID())
		if parent == pa.TestPlan {
			pa.Tree.RefreshItem("")
		}
		pa.Tree.OpenBranch(parent.ID())
	}
}

func (pa *PerfolizerApp) removeElement() {
	if pa.CurrentNodeID == "" {
		return
	}
	id := pa.CurrentNodeID
	if id == pa.TestPlan.ID() {
		dialog.ShowInformation("Error", "Cannot remove Root Test Plan", pa.Window)
		return
	}

	parent := pa.findParent(pa.TestPlan, id)
	if parent != nil {
		parent.RemoveChild(id)
		pa.Tree.RefreshItem(parent.ID())
		pa.Content.Objects = nil
		pa.Content.Refresh()
		pa.CurrentNodeID = "" // Clear selection
	}
}

func formatDebugExchange(exchange core.DebugHTTPExchange) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Request: %s %s\n", exchange.Request.Method, exchange.Request.URL)
	fmt.Fprintf(&b, "Duration: %d ms\n", exchange.DurationMilliseconds)
	b.WriteString("Outgoing headers:\n")
	b.WriteString(formatHeaders(exchange.Request.Headers))
	if exchange.Request.Body == "" {
		b.WriteString("Request body:\n<empty>\n")
	} else {
		b.WriteString("Request body:\n")
		b.WriteString(truncatePreview(exchange.Request.Body, maxBodyPreviewChars))
		b.WriteString("\n")
	}
	if exchange.RequestBodyTruncated {
		b.WriteString("Request body is truncated.\n")
	}

	if exchange.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", exchange.Error)
		return strings.TrimRight(b.String(), "\n")
	}

	if exchange.Response == nil {
		b.WriteString("No response from agent.\n")
		return strings.TrimRight(b.String(), "\n")
	}

	fmt.Fprintf(&b, "Status: %d (%s)\n", exchange.Response.StatusCode, exchange.Response.Status)
	b.WriteString("Incoming headers:\n")
	b.WriteString(formatHeaders(exchange.Response.Headers))
	if exchange.Response.Body == "" {
		b.WriteString("Response body:\n<empty>\n")
	} else {
		b.WriteString("Response body:\n")
		b.WriteString(truncatePreview(exchange.Response.Body, maxBodyPreviewChars))
		b.WriteString("\n")
	}
	if exchange.ResponseBodyTruncated {
		b.WriteString("Response body is truncated.\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func formatHeaders(headers map[string][]string) string {
	if len(headers) == 0 {
		return "<empty>\n"
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		values := headers[key]
		if len(values) == 0 {
			fmt.Fprintf(&b, "%s: \n", key)
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&b, "%s: %s\n", key, value)
		}
	}
	return b.String()
}

func truncatePreview(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + fmt.Sprintf("\n...[truncated, %d more chars]", len(value)-maxLen)
}
