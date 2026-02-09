package ui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	prefAgentsKey        = "agents"
	prefActiveAgentIDKey = "activeAgentID"
	defaultAgentName     = "Local agent"

	agentStatusFree        = "free"
	agentStatusUnavailable = "unavailable"
	agentStatusRunning     = "running"
)

type agentSettingsEntry struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BaseURL        string `json:"base_url"`
	RestartCommand string `json:"restart_command,omitempty"`
	RestartToken   string `json:"restart_token,omitempty"`
}

type agentRuntimeState struct {
	Status      string
	CurrentTest string
	LastError   string
	Host        AgentHostMetrics
	UpdatedAt   time.Time
}

func (pa *PerfolizerApp) initAgents(defaultBaseURL string, defaultClient *AgentClient) {
	agents := pa.loadAgentsFromPreferences()
	if len(agents) == 0 {
		baseURL := normalizeAgentBaseURL(defaultBaseURL)
		if baseURL == "" && defaultClient != nil {
			baseURL = normalizeAgentBaseURL(defaultClient.BaseURL())
		}
		if baseURL != "" {
			agents = append(agents, agentSettingsEntry{
				ID:      "local-agent",
				Name:    defaultAgentName,
				BaseURL: baseURL,
			})
		}
	}

	usedIDs := make(map[string]bool)
	for i := range agents {
		agents[i].Name = strings.TrimSpace(agents[i].Name)
		if agents[i].Name == "" {
			agents[i].Name = defaultAgentName
		}
		agents[i].BaseURL = normalizeAgentBaseURL(agents[i].BaseURL)
		if agents[i].BaseURL == "" {
			continue
		}
		agents[i].ID = pa.ensureUniqueAgentID(strings.TrimSpace(agents[i].ID), usedIDs, agents[i].Name)
	}

	filtered := make([]agentSettingsEntry, 0, len(agents))
	for _, agent := range agents {
		if agent.BaseURL == "" {
			continue
		}
		filtered = append(filtered, agent)
	}
	pa.agents = filtered
	pa.rebuildAgentClients()

	activeID := strings.TrimSpace(pa.FyneApp.Preferences().StringWithFallback(prefActiveAgentIDKey, ""))
	if !pa.hasAgentID(activeID) && len(pa.agents) > 0 {
		activeID = pa.agents[0].ID
	}
	pa.activeAgentID = activeID
	pa.saveAgentsToPreferences()
}

func (pa *PerfolizerApp) loadAgentsFromPreferences() []agentSettingsEntry {
	raw := strings.TrimSpace(pa.FyneApp.Preferences().StringWithFallback(prefAgentsKey, ""))
	if raw == "" {
		return nil
	}
	var agents []agentSettingsEntry
	if err := json.Unmarshal([]byte(raw), &agents); err != nil {
		return nil
	}
	return agents
}

func (pa *PerfolizerApp) saveAgentsToPreferences() {
	bytes, err := json.Marshal(pa.agents)
	if err == nil {
		pa.FyneApp.Preferences().SetString(prefAgentsKey, string(bytes))
	}
	pa.FyneApp.Preferences().SetString(prefActiveAgentIDKey, pa.activeAgentID)
}

func (pa *PerfolizerApp) rebuildAgentClients() {
	clients := make(map[string]*AgentClient, len(pa.agents))
	known := make(map[string]bool, len(pa.agents))
	for _, agent := range pa.agents {
		known[agent.ID] = true
		baseURL := normalizeAgentBaseURL(agent.BaseURL)
		if baseURL == "" {
			continue
		}
		clients[agent.ID] = NewAgentClient(baseURL)
	}
	pa.agentClients = clients

	pa.agentStateMu.Lock()
	for id := range pa.agentRuntime {
		if !known[id] {
			delete(pa.agentRuntime, id)
		}
	}
	pa.agentStateMu.Unlock()
}

func (pa *PerfolizerApp) hasAgentID(id string) bool {
	if id == "" {
		return false
	}
	for _, agent := range pa.agents {
		if agent.ID == id {
			return true
		}
	}
	return false
}

func (pa *PerfolizerApp) ensureUniqueAgentID(candidate string, used map[string]bool, name string) string {
	candidate = sanitizeID(candidate)
	if candidate == "" {
		candidate = sanitizeID(name)
	}
	if candidate == "" {
		candidate = "agent"
	}
	id := candidate
	seq := 2
	for used[id] || pa.hasAgentID(id) {
		id = fmt.Sprintf("%s-%d", candidate, seq)
		seq++
	}
	used[id] = true
	return id
}

func sanitizeID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeAgentBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func waitForAgentReady(client *AgentClient, timeout time.Duration) (AgentMetricsSnapshot, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		snapshot, err := client.FetchSnapshot()
		if err == nil {
			return snapshot, nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("agent is not reachable")
	}
	return AgentMetricsSnapshot{}, lastErr
}

func (pa *PerfolizerApp) resolveActiveAgentClient() (string, *AgentClient, error) {
	if len(pa.agents) == 0 {
		return "", nil, fmt.Errorf("no agents configured")
	}
	activeID := pa.activeAgentID
	if activeID == "" || pa.agentClients[activeID] == nil {
		activeID = pa.agents[0].ID
		pa.activeAgentID = activeID
		pa.saveAgentsToPreferences()
	}
	client := pa.agentClients[activeID]
	if client == nil {
		return "", nil, fmt.Errorf("active agent is not configured")
	}
	return activeID, client, nil
}

func (pa *PerfolizerApp) resolveStopTargetAgent() (string, *AgentClient, error) {
	if pa.runningAgentID != "" {
		if client := pa.agentClients[pa.runningAgentID]; client != nil {
			return pa.runningAgentID, client, nil
		}
	}
	return pa.resolveActiveAgentClient()
}

func (pa *PerfolizerApp) currentPlanDisplayName() string {
	if pa.Project == nil || pa.Project.PlanCount() == 0 {
		return "Test Plan"
	}
	idx := pa.getCurrentPlanIndex()
	if idx < 0 || idx >= pa.Project.PlanCount() {
		idx = 0
	}
	name := strings.TrimSpace(pa.Project.Plans[idx].Name)
	if name == "" {
		return "Test Plan"
	}
	return name
}

func (pa *PerfolizerApp) markAgentRunStarted(agentID, planName string, startedAt time.Time) {
	pa.agentStateMu.Lock()
	state := pa.agentRuntime[agentID]
	state.Status = agentStatusRunning
	state.CurrentTest = fmt.Sprintf("%s @ %s", planName, startedAt.Format("2006-01-02 15:04"))
	state.LastError = ""
	state.UpdatedAt = time.Now()
	pa.agentRuntime[agentID] = state
	pa.agentStateMu.Unlock()
}

func (pa *PerfolizerApp) markAgentIdle(agentID string) {
	pa.agentStateMu.Lock()
	state := pa.agentRuntime[agentID]
	state.Status = agentStatusFree
	state.CurrentTest = ""
	state.LastError = ""
	state.UpdatedAt = time.Now()
	pa.agentRuntime[agentID] = state
	pa.agentStateMu.Unlock()
}

func (pa *PerfolizerApp) markAgentUnavailable(agentID string, err error) {
	pa.agentStateMu.Lock()
	state := pa.agentRuntime[agentID]
	state.Status = agentStatusUnavailable
	state.LastError = ""
	if err != nil {
		state.LastError = err.Error()
	}
	state.UpdatedAt = time.Now()
	pa.agentRuntime[agentID] = state
	pa.agentStateMu.Unlock()
}

func (pa *PerfolizerApp) updateAgentRuntimeFromSnapshot(agentID string, snapshot AgentMetricsSnapshot) {
	pa.agentStateMu.Lock()
	state := pa.agentRuntime[agentID]
	state.Host = snapshot.Host
	state.LastError = ""
	state.UpdatedAt = time.Now()

	if snapshot.Running {
		state.Status = agentStatusRunning
		if state.CurrentTest == "" {
			state.CurrentTest = "external run"
		}
	} else {
		state.Status = agentStatusFree
		state.CurrentTest = ""
	}
	pa.agentRuntime[agentID] = state
	pa.agentStateMu.Unlock()
}

func (pa *PerfolizerApp) getAgentRuntimeState(agentID string) agentRuntimeState {
	pa.agentStateMu.RLock()
	defer pa.agentStateMu.RUnlock()
	state := pa.agentRuntime[agentID]
	if state.Status == "" {
		state.Status = agentStatusUnavailable
	}
	return state
}

func (pa *PerfolizerApp) refreshAllAgentStates() {
	for _, agent := range pa.agents {
		client := pa.agentClients[agent.ID]
		if client == nil {
			pa.markAgentUnavailable(agent.ID, fmt.Errorf("no client"))
			continue
		}
		snapshot, err := client.FetchSnapshot()
		if err != nil {
			pa.markAgentUnavailable(agent.ID, err)
			continue
		}
		pa.updateAgentRuntimeFromSnapshot(agent.ID, snapshot)
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := float64(unit), 0
	for n := float64(bytes) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.2f %s", float64(bytes)/div, suffixes[exp])
}

func formatHostMetrics(host AgentHostMetrics) string {
	cpuText := fmt.Sprintf("%.2f%%", host.CPUUtilizationPercent)

	memText := "n/a"
	if host.MemoryTotalBytes > 0 {
		memText = fmt.Sprintf("%s / %s (%.2f%%)", formatBytes(host.MemoryUsedBytes), formatBytes(host.MemoryTotalBytes), host.MemoryUsedPercent)
	}

	diskText := "n/a"
	if host.DiskTotalBytes > 0 {
		path := host.DiskPath
		if path == "" {
			path = "/"
		}
		diskText = fmt.Sprintf("%s / %s (%.2f%%) on %s", formatBytes(host.DiskUsedBytes), formatBytes(host.DiskTotalBytes), host.DiskUsedPercent, path)
	}

	return fmt.Sprintf("CPU usage: %s\nMemory: %s\nDisk: %s", cpuText, memText, diskText)
}

func (pa *PerfolizerApp) showPreferences() {
	if pa.settingsWindow != nil {
		pa.settingsWindow.RequestFocus()
		return
	}

	w := pa.FyneApp.NewWindow("Settings")
	w.Resize(fyne.NewSize(1320, 820))
	pa.settingsWindow = w
	w.SetOnClosed(func() {
		pa.settingsWindow = nil
	})

	sections := []string{"General", "Shortcuts", "Agents"}
	content := container.NewMax()

	shortcutsPage := pa.buildShortcutsPage()
	agentsPage := pa.buildAgentsPage(w)
	pages := map[string]fyne.CanvasObject{
		"General":   container.NewPadded(widget.NewCard("General", "", widget.NewLabel("General settings will appear here."))),
		"Shortcuts": shortcutsPage,
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
	w.SetContent(layout)

	sectionList.Select(0)
	w.Show()
}

func (pa *PerfolizerApp) buildShortcutsPage() fyne.CanvasObject {
	prefs := pa.FyneApp.Preferences()
	keyEntry := widget.NewEntry()
	keyEntry.SetText(prefs.StringWithFallback(prefToggleEnabledKey, defaultToggleEnabledKey))
	keyEntry.PlaceHolder = "e.g. Ctrl+E, Alt+Shift+T"

	saveButton := widget.NewButton("Save shortcut", func() {
		text := strings.TrimSpace(keyEntry.Text)
		if text == "" {
			text = defaultToggleEnabledKey
		}
		if _, _, ok := parseShortcut(text); !ok {
			dialog.ShowError(fmt.Errorf("use Ctrl/Alt/Shift/Super with a key (e.g. Ctrl+E)"), pa.settingsWindow)
			return
		}
		prefs.SetString(prefToggleEnabledKey, text)
		pa.registerToggleKey()
		dialog.ShowInformation("Saved", "Shortcut updated.", pa.settingsWindow)
	})

	resetButton := widget.NewButton("Reset to default", func() {
		keyEntry.SetText(defaultToggleEnabledKey)
	})

	form := widget.NewForm(widget.NewFormItem("Toggle element shortcut", keyEntry))
	return container.NewPadded(widget.NewCard("Shortcuts", "", container.NewVBox(
		form,
		container.NewHBox(saveButton, resetButton),
	)))
}

func (pa *PerfolizerApp) buildAgentsPage(win fyne.Window) fyne.CanvasObject {
	agentIDs := make([]string, 0, len(pa.agents))
	selectedID := ""

	nameEntry := widget.NewEntry()
	urlEntry := widget.NewEntry()
	restartCommandEntry := widget.NewEntry()
	restartTokenEntry := widget.NewPasswordEntry()
	selectedLabel := widget.NewLabel("No agent selected")
	activeLabel := widget.NewLabel("Active: no")
	statusLabel := widget.NewLabel("Status: unavailable")
	testLabel := widget.NewLabel("Test ID: -")
	errorLabel := widget.NewLabel("Last error: -")
	metricsLabel := widget.NewLabel("CPU usage: n/a\nMemory: n/a\nDisk: n/a")

	refreshAgentIDs := func() {
		agentIDs = agentIDs[:0]
		for _, agent := range pa.agents {
			agentIDs = append(agentIDs, agent.ID)
		}
		if len(agentIDs) == 0 {
			selectedID = ""
			return
		}
		if selectedID == "" || !pa.hasAgentID(selectedID) {
			selectedID = agentIDs[0]
		}
	}

	var updateDetails func()
	updateDetails = func() {
		if selectedID == "" {
			selectedLabel.SetText("No agent selected")
			activeLabel.SetText("Active: no")
			statusLabel.SetText("Status: unavailable")
			testLabel.SetText("Current test on selected agent: -")
			errorLabel.SetText("Last error: -")
			metricsLabel.SetText("CPU usage: n/a\nMemory: n/a\nDisk: n/a")
			nameEntry.SetText("")
			urlEntry.SetText("")
			restartCommandEntry.SetText("")
			restartTokenEntry.SetText("")
			return
		}

		var selectedAgent agentSettingsEntry
		found := false
		for _, agent := range pa.agents {
			if agent.ID == selectedID {
				selectedAgent = agent
				found = true
				break
			}
		}
		if !found {
			selectedID = ""
			updateDetails()
			return
		}

		selectedLabel.SetText(fmt.Sprintf("Agent: %s", selectedAgent.Name))
		activeLabel.SetText(fmt.Sprintf("Active: %t", selectedAgent.ID == pa.activeAgentID))
		nameEntry.SetText(selectedAgent.Name)
		urlEntry.SetText(selectedAgent.BaseURL)
		restartCommandEntry.SetText(selectedAgent.RestartCommand)
		restartTokenEntry.SetText(selectedAgent.RestartToken)

		runtime := pa.getAgentRuntimeState(selectedAgent.ID)
		statusLabel.SetText(fmt.Sprintf("Status: %s", runtime.Status))
		if runtime.CurrentTest == "" {
			testLabel.SetText(fmt.Sprintf("Current test on %s: -", selectedAgent.Name))
		} else {
			testLabel.SetText(fmt.Sprintf("Current test on %s: %s", selectedAgent.Name, runtime.CurrentTest))
		}
		if runtime.LastError == "" {
			errorLabel.SetText("Last error: -")
		} else {
			errorLabel.SetText(fmt.Sprintf("Last error: %s", runtime.LastError))
		}
		metricsLabel.SetText(formatHostMetrics(runtime.Host))
	}

	refreshAll := func() {
		pa.refreshAllAgentStates()
		refreshAgentIDs()
		updateDetails()
	}

	agentList := widget.NewList(
		func() int { return len(agentIDs) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("Agent")
			name.TextStyle = fyne.TextStyle{Bold: true}
			details := widget.NewLabel("status: - | current test: -")
			return container.NewVBox(name, details)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			agentID := agentIDs[i]
			var agent agentSettingsEntry
			for _, item := range pa.agents {
				if item.ID == agentID {
					agent = item
					break
				}
			}
			runtime := pa.getAgentRuntimeState(agent.ID)
			active := ""
			if agent.ID == pa.activeAgentID {
				active = " [active]"
			}
			testID := "-"
			if runtime.CurrentTest != "" {
				testID = runtime.CurrentTest
			}
			row := obj.(*fyne.Container)
			row.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%s%s", agent.Name, active))
			row.Objects[1].(*widget.Label).SetText(fmt.Sprintf("status: %s | current test: %s", runtime.Status, testID))
		},
	)

	refreshAgentList := func() {
		agentList.Refresh()
	}

	agentList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(agentIDs) {
			return
		}
		selectedID = agentIDs[id]
		updateDetails()
	}

	addAgent := func() {
		nameInput := widget.NewEntry()
		nameInput.SetText("Agent")
		urlInput := widget.NewEntry()
		urlInput.SetPlaceHolder("http://127.0.0.1:9090")
		restartCommandInput := widget.NewEntry()
		restartCommandInput.SetPlaceHolder("optional shell command")
		restartTokenInput := widget.NewPasswordEntry()
		restartTokenInput.SetPlaceHolder("optional admin token")

		addFormItems := []*widget.FormItem{
			widget.NewFormItem("Name", nameInput),
			widget.NewFormItem("Base URL", urlInput),
			widget.NewFormItem("Restart command", restartCommandInput),
			widget.NewFormItem("Restart token", restartTokenInput),
		}
		form := widget.NewForm(addFormItems...)
		formSection := container.NewVBox(
			widget.NewLabel("Configure new agent connection"),
			form,
		)

		addDialog := dialog.NewCustomConfirm("Add agent", "Add", "Cancel", container.NewPadded(formSection), func(ok bool) {
			if !ok {
				return
			}
			baseURL := normalizeAgentBaseURL(urlInput.Text)
			if baseURL == "" {
				dialog.ShowError(fmt.Errorf("invalid base URL"), win)
				return
			}
			name := strings.TrimSpace(nameInput.Text)
			if name == "" {
				name = "Agent"
			}
			usedIDs := make(map[string]bool, len(pa.agents))
			for _, agent := range pa.agents {
				usedIDs[agent.ID] = true
			}
			id := pa.ensureUniqueAgentID("", usedIDs, name)
			pa.agents = append(pa.agents, agentSettingsEntry{
				ID:             id,
				Name:           name,
				BaseURL:        baseURL,
				RestartCommand: strings.TrimSpace(restartCommandInput.Text),
				RestartToken:   strings.TrimSpace(restartTokenInput.Text),
			})
			if pa.activeAgentID == "" {
				pa.activeAgentID = id
			}
			pa.rebuildAgentClients()
			pa.saveAgentsToPreferences()
			refreshAgentIDs()
			refreshAgentList()
			selectedID = id
			updateDetails()
		}, win)
		addDialog.Resize(fyne.NewSize(780, 420))
		addDialog.Show()
	}

	removeSelected := func() {
		if selectedID == "" {
			return
		}
		dialog.ShowConfirm("Remove agent", "Remove selected agent?", func(ok bool) {
			if !ok {
				return
			}
			filtered := make([]agentSettingsEntry, 0, len(pa.agents))
			for _, agent := range pa.agents {
				if agent.ID == selectedID {
					continue
				}
				filtered = append(filtered, agent)
			}
			pa.agents = filtered
			if pa.activeAgentID == selectedID {
				pa.activeAgentID = ""
				if len(pa.agents) > 0 {
					pa.activeAgentID = pa.agents[0].ID
				}
			}
			selectedID = ""
			pa.rebuildAgentClients()
			pa.saveAgentsToPreferences()
			refreshAgentIDs()
			refreshAgentList()
			updateDetails()
		}, win)
	}

	saveSelected := func() {
		if selectedID == "" {
			return
		}
		updatedName := strings.TrimSpace(nameEntry.Text)
		if updatedName == "" {
			updatedName = "Agent"
		}
		updatedURL := normalizeAgentBaseURL(urlEntry.Text)
		if updatedURL == "" {
			dialog.ShowError(fmt.Errorf("invalid base URL"), win)
			return
		}
		updatedRestartCommand := strings.TrimSpace(restartCommandEntry.Text)
		updatedRestartToken := strings.TrimSpace(restartTokenEntry.Text)
		for i := range pa.agents {
			if pa.agents[i].ID == selectedID {
				pa.agents[i].Name = updatedName
				pa.agents[i].BaseURL = updatedURL
				pa.agents[i].RestartCommand = updatedRestartCommand
				pa.agents[i].RestartToken = updatedRestartToken
				break
			}
		}
		pa.rebuildAgentClients()
		pa.saveAgentsToPreferences()
		refreshAll()
		refreshAgentList()
		updateDetails()
	}

	setActive := func() {
		if selectedID == "" {
			return
		}
		pa.activeAgentID = selectedID
		pa.saveAgentsToPreferences()
		refreshAgentList()
		updateDetails()
	}

	restartSelected := func() {
		if selectedID == "" {
			return
		}
		client := pa.agentClients[selectedID]
		if client == nil {
			dialog.ShowError(fmt.Errorf("agent client is not configured"), win)
			return
		}
		if err := client.StopTest(); err != nil {
			pa.markAgentUnavailable(selectedID, err)
			updateDetails()
			refreshAgentList()
			dialog.ShowError(err, win)
			return
		}
		snapshot, err := client.FetchSnapshot()
		if err != nil {
			pa.markAgentUnavailable(selectedID, err)
			updateDetails()
			refreshAgentList()
			dialog.ShowError(err, win)
			return
		}
		pa.updateAgentRuntimeFromSnapshot(selectedID, snapshot)
		dialog.ShowInformation("Agent restart", "Agent runtime session restarted.", win)
		updateDetails()
		refreshAgentList()
	}

	restartProcess := func() {
		if selectedID == "" {
			return
		}
		client := pa.agentClients[selectedID]
		if client == nil {
			dialog.ShowError(fmt.Errorf("agent client is not configured"), win)
			return
		}
		var selectedAgent agentSettingsEntry
		found := false
		for _, agent := range pa.agents {
			if agent.ID == selectedID {
				selectedAgent = agent
				found = true
				break
			}
		}
		if !found {
			dialog.ShowError(fmt.Errorf("agent is not selected"), win)
			return
		}
		waitDialog := dialog.NewCustomWithoutButtons(
			"Restart process",
			container.NewPadded(widget.NewLabel("Restarting agent process, please wait...")),
			win,
		)
		waitDialog.Show()

		go func() {
			err := client.RestartProcess(selectedAgent.RestartCommand, selectedAgent.RestartToken)
			if err != nil {
				pa.markAgentUnavailable(selectedID, err)
				fyne.Do(func() {
					waitDialog.Hide()
					updateDetails()
					refreshAgentList()
					dialog.ShowError(fmt.Errorf("process restart failed: %w", err), win)
				})
				return
			}

			snapshot, readyErr := waitForAgentReady(client, 45*time.Second)
			if readyErr != nil {
				pa.markAgentUnavailable(selectedID, readyErr)
				fyne.Do(func() {
					waitDialog.Hide()
					updateDetails()
					refreshAgentList()
					dialog.ShowError(fmt.Errorf("agent did not recover after restart: %w", readyErr), win)
				})
				return
			}

			pa.updateAgentRuntimeFromSnapshot(selectedID, snapshot)
			fyne.Do(func() {
				waitDialog.Hide()
				updateDetails()
				refreshAgentList()
				dialog.ShowInformation("Restart process", "Agent process restarted successfully.", win)
			})
		}()
	}

	refreshSelected := func() {
		if selectedID == "" {
			refreshAll()
			agentList.Refresh()
			updateDetails()
			return
		}
		client := pa.agentClients[selectedID]
		if client == nil {
			pa.markAgentUnavailable(selectedID, fmt.Errorf("agent client is not configured"))
		} else {
			snapshot, err := client.FetchSnapshot()
			if err != nil {
				pa.markAgentUnavailable(selectedID, err)
			} else {
				pa.updateAgentRuntimeFromSnapshot(selectedID, snapshot)
			}
		}
		refreshAgentList()
		updateDetails()
	}

	refreshAgentIDs()
	refreshAll()
	refreshAgentList()
	updateDetails()
	if len(agentIDs) > 0 {
		for i, id := range agentIDs {
			if id == selectedID {
				agentList.Select(i)
				break
			}
		}
	}

	middlePanel := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Connected agents"),
			container.NewHBox(
				widget.NewButton("Add", addAgent),
				widget.NewButton("Remove", removeSelected),
				widget.NewButton("Refresh", func() {
					refreshAll()
					refreshAgentList()
				}),
			),
		),
		nil,
		nil,
		nil,
		container.NewPadded(agentList),
	)

	rightPanel := container.NewVScroll(container.NewVBox(
		widget.NewCard("Agent", "", container.NewVBox(
			selectedLabel,
			activeLabel,
			widget.NewForm(
				widget.NewFormItem("Name", nameEntry),
				widget.NewFormItem("Base URL", urlEntry),
				widget.NewFormItem("Restart command", restartCommandEntry),
				widget.NewFormItem("Restart token", restartTokenEntry),
			),
			container.NewHBox(
				widget.NewButton("Save", saveSelected),
				widget.NewButton("Set active", setActive),
			),
		)),
		widget.NewCard("Runtime", "", container.NewVBox(
			statusLabel,
			testLabel,
			errorLabel,
			container.NewHBox(
				widget.NewButton("Restart agent", restartSelected),
				widget.NewButton("Restart process", restartProcess),
				widget.NewButton("Refresh metrics", refreshSelected),
			),
		)),
		widget.NewCard("Machine metrics", "", metricsLabel),
	))

	split := container.NewHSplit(
		container.NewPadded(middlePanel),
		container.NewPadded(rightPanel),
	)
	split.SetOffset(0.47)
	return container.NewPadded(split)
}
