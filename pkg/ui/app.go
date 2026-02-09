package ui

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const maxDebugItems = 150
const maxBodyPreviewChars = 20000
const prefToggleEnabledKey = "toggleEnabledKey"
const defaultToggleEnabledKey = "Ctrl+E"

// treeWithContextMenu wraps the tree so right-click shows Enable/Disable menu for the selected node.
type treeWithContextMenu struct {
	widget.BaseWidget
	tree *widget.Tree
	pa   *PerfolizerApp
}

func newTreeWithContextMenu(tree *widget.Tree, pa *PerfolizerApp) *treeWithContextMenu {
	t := &treeWithContextMenu{tree: tree, pa: pa}
	t.ExtendBaseWidget(t)
	return t
}

func (t *treeWithContextMenu) TappedSecondary(*fyne.PointEvent) {
	t.pa.showNodeContextMenu(t.pa.CurrentNodeID)
}

func (t *treeWithContextMenu) CreateRenderer() fyne.WidgetRenderer {
	return &treeWithContextMenuRenderer{tree: t.tree, obj: t}
}

type treeWithContextMenuRenderer struct {
	tree *widget.Tree
	obj  *treeWithContextMenu
}

func (r *treeWithContextMenuRenderer) Destroy() {}

func (r *treeWithContextMenuRenderer) Layout(size fyne.Size) {
	r.tree.Resize(size)
}

func (r *treeWithContextMenuRenderer) MinSize() fyne.Size {
	return r.tree.MinSize()
}

func (r *treeWithContextMenuRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.tree}
}

func (r *treeWithContextMenuRenderer) Refresh() {}

// uriPath returns a filesystem path from a fyne URI (handles file:// on Windows).
func uriPath(uri fyne.URI) string {
	p := uri.Path()
	// file:///C:/... is represented as /C:/...; strip only that Windows URI prefix.
	if len(p) >= 4 &&
		p[0] == '/' &&
		((p[1] >= 'a' && p[1] <= 'z') || (p[1] >= 'A' && p[1] <= 'Z')) &&
		p[2] == ':' &&
		p[3] == '/' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

type PerfolizerApp struct {
	FyneApp fyne.App
	Window  fyne.Window
	Tree    *widget.Tree
	Content *fyne.Container

	DebugConsoleList   *fyne.Container
	DebugConsoleScroll *container.Scroll

	Project       *core.Project // Project with multiple test plans
	CurrentNodeID string        // Tree node ID: "plan:i" or "plan:i:elementId"

	agentClient    *AgentClient
	agentInitError error
	pollInterval   time.Duration

	cancelFunc     context.CancelFunc
	isRunning      bool
	isDebugRunning bool

	toggleShortcut fyne.Shortcut // stored so we can remove when re-registering
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
	pa.registerToggleKey()

	return pa
}

func (pa *PerfolizerApp) Run() {
	pa.Window.ShowAndRun()
}

func (pa *PerfolizerApp) setupTestPlan() {
	pa.Project = core.NewProject("Project")
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("Thread Group 1", 1, 1)
	root.AddChild(tg)
	pa.Project.AddPlan("Test Plan", &root)
}

func (pa *PerfolizerApp) setupUI() {
	// 1. Tree View: root "" -> plan:0, plan:1, ...; plan:i -> plan:i:childIds; plan:i:elId -> children
	pa.Tree = widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID {
			if id == "" {
				var ids []string
				for i := 0; i < pa.Project.PlanCount(); i++ {
					ids = append(ids, fmt.Sprintf("plan:%d", i))
				}
				return ids
			}
			planIdx, el := pa.resolveNode(id)
			if planIdx < 0 {
				return nil
			}
			var root core.TestElement
			if el == nil {
				root = pa.Project.Plans[planIdx].Root
			} else {
				root = el
			}
			var ids []string
			for _, c := range root.GetChildren() {
				ids = append(ids, fmt.Sprintf("plan:%d:%s", planIdx, c.ID()))
			}
			return ids
		},
		func(id widget.TreeNodeID) bool {
			if id == "" {
				return true
			}
			planIdx, el := pa.resolveNode(id)
			if planIdx < 0 {
				return false
			}
			var root core.TestElement
			if el == nil {
				root = pa.Project.Plans[planIdx].Root
			} else {
				root = el
			}
			return len(root.GetChildren()) > 0
		},
		func(branch bool) fyne.CanvasObject {
			seg := &widget.TextSegment{Text: " "}
			seg.Style.ColorName = theme.ColorNameForeground
			return widget.NewRichText(seg)
		},
		func(id widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
			planIdx, el := pa.resolveNode(id)
			if planIdx < 0 {
				return
			}
			rt := o.(*widget.RichText)
			segs := rt.Segments
			if len(segs) == 0 {
				return
			}
			seg := segs[0].(*widget.TextSegment)
			var name string
			if el == nil {
				name = pa.Project.Plans[planIdx].Name
				if planIdx == pa.getCurrentPlanIndex() {
					seg.Style.ColorName = theme.ColorNamePrimary
					seg.Style.TextStyle = fyne.TextStyle{Bold: true}
				} else {
					seg.Style.ColorName = theme.ColorNameForeground
					seg.Style.TextStyle = fyne.TextStyle{}
				}
			} else {
				name = el.Name()
				seg.Style.TextStyle = fyne.TextStyle{}
				if el.Enabled() {
					seg.Style.ColorName = theme.ColorNameForeground
				} else {
					seg.Style.ColorName = theme.ColorNameDisabled
				}
			}
			seg.Text = name
			rt.Refresh()
		},
	)

	pa.Tree.OnSelected = func(id widget.TreeNodeID) {
		pa.CurrentNodeID = id
		planIdx, el := pa.resolveNode(id)
		if planIdx >= 0 {
			// Refresh plan nodes so active plan highlight updates
			for i := 0; i < pa.Project.PlanCount(); i++ {
				pa.Tree.RefreshItem(fmt.Sprintf("plan:%d", i))
			}
			if el == nil {
				pa.showPlanProperties(planIdx)
			} else {
				pa.showProperties(el)
			}
		}
	}

	debugConsoleList := container.NewVBox()
	debugConsoleScroll := container.NewVScroll(debugConsoleList)
	pa.DebugConsoleList = debugConsoleList
	pa.DebugConsoleScroll = debugConsoleScroll

	clearDebugButton := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		pa.clearDebugConsole()
	})
	debugPanel := container.NewBorder(
		container.NewBorder(nil, nil, widget.NewLabel("Debug Console"), clearDebugButton, nil),
		nil, nil, nil,
		container.NewPadded(debugConsoleScroll),
	)

	// 2. Toolbar (Top)
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ContentAddIcon(), func() { pa.addElement() }),       // Add element
		widget.NewToolbarAction(theme.ContentRemoveIcon(), func() { pa.removeElement() }), // Remove element/plan
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.FolderNewIcon(), func() { pa.addPlan() }), // Add plan
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.FolderOpenIcon(), func() { pa.loadTestPlan() }),
		widget.NewToolbarAction(theme.DocumentSaveIcon(), func() { pa.saveTestPlan() }),
		widget.NewToolbarAction(theme.SettingsIcon(), func() { pa.showPreferences() }), // Settings
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.MediaPlayIcon(), func() { pa.runTest() }),          // Start
		widget.NewToolbarAction(theme.SearchReplaceIcon(), func() { pa.runDebugTest() }), // Debug
		widget.NewToolbarAction(theme.MediaStopIcon(), func() { pa.stopTest() }),         // Stop
	)

	// 3. Layout
	rightSplit := container.NewVSplit(pa.Content, debugPanel)
	rightSplit.SetOffset(0.62)

	// Wrap tree so right-click opens context menu (no ⋮ button)
	treeWithCtxMenu := newTreeWithContextMenu(pa.Tree, pa)
	split := container.NewHSplit(
		container.NewBorder(nil, nil, nil, nil, treeWithCtxMenu),
		rightSplit,
	)
	split.SetOffset(0.3)

	// Top bar: toolbar + separator so it doesn't blend with content
	toolbarBar := container.NewVBox(
		container.NewPadded(container.NewStack(
			canvas.NewRectangle(theme.Color(theme.ColorNameButton)),
			toolbar,
		)),
		widget.NewSeparator(),
	)
	mainLayout := container.NewBorder(toolbarBar, nil, nil, nil, split)
	pa.Window.SetContent(mainLayout)
}

// parseShortcut parses "Ctrl+E", "Alt+Shift+E" etc. into KeyName and Modifier.
// Requires at least one modifier (Ctrl, Alt, Shift, Super) so the shortcut works with AddShortcut.
func parseShortcut(s string) (keyName fyne.KeyName, modifier fyne.KeyModifier, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, false
	}
	parts := strings.Split(s, "+")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	if len(parts) < 2 {
		return "", 0, false // need at least Modifier+Key
	}
	keyPart := parts[len(parts)-1]
	modParts := parts[:len(parts)-1]
	var mod fyne.KeyModifier
	for _, m := range modParts {
		switch strings.ToLower(m) {
		case "ctrl", "control":
			mod |= fyne.KeyModifierControl
		case "alt":
			mod |= fyne.KeyModifierAlt
		case "shift":
			mod |= fyne.KeyModifierShift
		case "super", "cmd", "meta":
			mod |= fyne.KeyModifierSuper
		default:
			return "", 0, false
		}
	}
	if mod == 0 {
		return "", 0, false
	}
	// Map key string to fyne.KeyName (letters and common keys)
	keyPart = strings.ToUpper(keyPart)
	if len(keyPart) == 1 && keyPart >= "A" && keyPart <= "Z" {
		return fyne.KeyName(keyPart), mod, true
	}
	// Common keys
	switch keyPart {
	case "SPACE":
		return fyne.KeySpace, mod, true
	case "ENTER", "RETURN":
		return fyne.KeyReturn, mod, true
	case "TAB":
		return fyne.KeyTab, mod, true
	case "ESCAPE", "ESC":
		return fyne.KeyEscape, mod, true
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		return fyne.KeyName(keyPart), mod, true
	default:
		return fyne.KeyName(keyPart), mod, true
	}
}

func (pa *PerfolizerApp) registerToggleKey() {
	canvas := pa.Window.Canvas()
	if pa.toggleShortcut != nil {
		canvas.RemoveShortcut(pa.toggleShortcut)
		pa.toggleShortcut = nil
	}
	spec := pa.FyneApp.Preferences().StringWithFallback(prefToggleEnabledKey, defaultToggleEnabledKey)
	keyName, modifier, ok := parseShortcut(spec)
	if !ok {
		spec = defaultToggleEnabledKey
		keyName, modifier, ok = parseShortcut(spec)
		if !ok {
			return
		}
	}
	shortcut := &desktop.CustomShortcut{KeyName: keyName, Modifier: modifier}
	pa.toggleShortcut = shortcut
	canvas.AddShortcut(shortcut, func(fyne.Shortcut) {
		pa.toggleCurrentElementEnabled()
	})
}

func (pa *PerfolizerApp) showPreferences() {
	prefs := pa.FyneApp.Preferences()
	currentKey := prefs.StringWithFallback(prefToggleEnabledKey, defaultToggleEnabledKey)
	keyEntry := widget.NewEntry()
	keyEntry.SetText(currentKey)
	keyEntry.PlaceHolder = "e.g. Ctrl+E, Alt+Shift+T"
	dialog.ShowForm("Preferences", "Save", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Toggle element shortcut (e.g. Ctrl+E)", keyEntry),
	}, func(ok bool) {
		if !ok {
			return
		}
		txt := strings.TrimSpace(keyEntry.Text)
		if txt == "" {
			txt = defaultToggleEnabledKey
		}
		if _, _, parseOk := parseShortcut(txt); !parseOk {
			dialog.ShowError(fmt.Errorf("use a combination with Ctrl, Alt, Shift or Super (e.g. Ctrl+E)"), pa.Window)
			return
		}
		prefs.SetString(prefToggleEnabledKey, txt)
		pa.registerToggleKey()
	}, pa.Window)
}

// parsePlanNodeID splits "plan:i" or "plan:i:elementId" into plan index and optional element ID.
// Returns planIndex, elementID (empty for plan node), ok.
func (pa *PerfolizerApp) parsePlanNodeID(nodeID string) (planIndex int, elementID string, ok bool) {
	if nodeID == "" || pa.Project == nil {
		return -1, "", false
	}
	parts := strings.SplitN(nodeID, ":", 3)
	if len(parts) < 2 || parts[0] != "plan" {
		return -1, "", false
	}
	var idx int
	if _, err := fmt.Sscanf(parts[1], "%d", &idx); err != nil || idx < 0 || idx >= pa.Project.PlanCount() {
		return -1, "", false
	}
	if len(parts) == 3 {
		return idx, parts[2], true
	}
	return idx, "", true
}

// getCurrentPlanIndex returns the plan index for CurrentNodeID, or 0 if none.
func (pa *PerfolizerApp) getCurrentPlanIndex() int {
	idx, _, ok := pa.parsePlanNodeID(pa.CurrentNodeID)
	if !ok || idx < 0 {
		return 0
	}
	return idx
}

// getCurrentPlan returns the root TestElement of the current plan, or nil.
func (pa *PerfolizerApp) getCurrentPlan() core.TestElement {
	idx := pa.getCurrentPlanIndex()
	if pa.Project == nil || idx < 0 || idx >= pa.Project.PlanCount() {
		return nil
	}
	return pa.Project.Plans[idx].Root
}

// resolveNode returns the plan index and the TestElement for the given tree node ID.
// For "plan:i" element is nil (plan node). For "plan:i:elId" element is the element.
func (pa *PerfolizerApp) resolveNode(nodeID string) (planIndex int, element core.TestElement) {
	idx, elID, ok := pa.parsePlanNodeID(nodeID)
	if !ok || pa.Project == nil || idx < 0 || idx >= pa.Project.PlanCount() {
		return -1, nil
	}
	root := pa.Project.Plans[idx].Root
	if elID == "" {
		return idx, nil
	}
	return idx, pa.findElementByID(root, elID)
}

func (pa *PerfolizerApp) showNodeContextMenu(nodeID string) {
	_, el := pa.resolveNode(nodeID)
	if el == nil {
		return // plan node has no enable/disable
	}
	enabled := el.Enabled()
	enableItem := fyne.NewMenuItem("Enable", func() {
		el.SetEnabled(true)
		pa.Tree.RefreshItem(nodeID)
	})
	disableItem := fyne.NewMenuItem("Disable", func() {
		el.SetEnabled(false)
		pa.Tree.RefreshItem(nodeID)
	})
	enableItem.Disabled = enabled
	disableItem.Disabled = !enabled
	menu := fyne.NewMenu("", enableItem, disableItem)
	pop := widget.NewPopUpMenu(menu, pa.Window.Canvas())
	pop.Show()
}

func (pa *PerfolizerApp) toggleCurrentElementEnabled() {
	planIdx, el := pa.resolveNode(pa.CurrentNodeID)
	if planIdx < 0 || el == nil {
		return
	}
	el.SetEnabled(!el.Enabled())
	pa.Tree.RefreshItem(pa.CurrentNodeID)
	pa.showProperties(el) // refresh properties panel if this node is selected
}

// treeIDForElement returns the tree node ID for an element in the given plan.
func (pa *PerfolizerApp) treeIDForElement(planIndex int, el core.TestElement) string {
	if pa.Project == nil || planIndex < 0 || planIndex >= pa.Project.PlanCount() {
		return ""
	}
	root := pa.Project.Plans[planIndex].Root
	if root == el {
		return fmt.Sprintf("plan:%d", planIndex)
	}
	return fmt.Sprintf("plan:%d:%s", planIndex, el.ID())
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

func (pa *PerfolizerApp) showPlanProperties(planIndex int) {
	if pa.Project == nil || planIndex < 0 || planIndex >= pa.Project.PlanCount() {
		return
	}
	pa.Content.Objects = nil
	pe := &pa.Project.Plans[planIndex]
	nameEntry := widget.NewEntry()
	nameEntry.SetText(pe.Name)
	nameEntry.OnChanged = func(s string) {
		pe.Name = s
		pa.Tree.RefreshItem(fmt.Sprintf("plan:%d", planIndex))
	}
	form := widget.NewForm(widget.NewFormItem("Plan name", nameEntry))
	pa.Content.Objects = []fyne.CanvasObject{container.NewVBox(widget.NewLabel("Test plan"), form)}
	pa.Content.Refresh()
}

func (pa *PerfolizerApp) showProperties(el core.TestElement) {
	pa.Content.Objects = nil

	nameEntry := widget.NewEntry()
	nameEntry.SetText(el.Name())
	nameEntry.OnChanged = func(s string) {
		el.SetName(s)
		pa.Tree.RefreshItem(pa.treeIDForElement(pa.getCurrentPlanIndex(), el))
	}

	enabledCheck := widget.NewCheck("Enabled (included in test run)", func(checked bool) {
		el.SetEnabled(checked)
		pa.Tree.RefreshItem(pa.treeIDForElement(pa.getCurrentPlanIndex(), el))
	})
	enabledCheck.SetChecked(el.Enabled())

	form := widget.NewForm(
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("", enabledCheck),
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
			return
		}
		defer writer.Close()
		path := uriPath(writer.URI())
		if err := core.SaveProject(path, pa.Project); err != nil {
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
			return
		}
		defer reader.Close()
		path := uriPath(reader.URI())
		proj, err := core.LoadProject(path)
		if err != nil {
			// Legacy: single test plan JSON
			plan, loadErr := core.LoadTestPlan(path)
			if loadErr != nil {
				dialog.ShowError(err, pa.Window)
				return
			}
			proj = core.NewProject("Project")
			proj.AddPlan(plan.Name(), plan)
		}
		pa.Project = proj
		pa.Tree.RefreshItem("")
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

	plan := pa.getCurrentPlan()
	if plan == nil {
		dialog.ShowError(fmt.Errorf("no test plan selected"), pa.Window)
		return
	}
	if err := pa.agentClient.RunTest(plan); err != nil {
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
	if plan := pa.getCurrentPlan(); plan != nil {
		pa.collectHTTPSamplers(plan, &samplers)
	}
	if len(samplers) == 0 {
		dialog.ShowInformation("Debug run", "No HTTP samplers found in the test plan.", pa.Window)
		return
	}

	pa.isDebugRunning = true
	pa.clearDebugConsole()
	pa.appendDebugInfo(fmt.Sprintf("Debug run started at %s", time.Now().Format(time.RFC3339)))
	pa.appendDebugInfo(fmt.Sprintf("Requests to execute once: %d", len(samplers)))

	go pa.executeDebugRun(samplers)
}

func (pa *PerfolizerApp) executeDebugRun(samplers []*elements.HttpSampler) {
	for i, sampler := range samplers {
		exchange, err := pa.agentClient.DebugHTTP(core.DebugHTTPRequest{
			Method: sampler.Method,
			URL:    sampler.Url,
			Body:   sampler.Body,
		})
		pa.appendDebugSamplerCard(i+1, len(samplers), sampler, &exchange, err)
	}

	pa.appendDebugInfo(fmt.Sprintf("Debug run finished at %s", time.Now().Format(time.RFC3339)))

	fyne.Do(func() {
		pa.isDebugRunning = false
	})
}

func (pa *PerfolizerApp) collectHTTPSamplers(root core.TestElement, out *[]*elements.HttpSampler) {
	if !root.Enabled() {
		return
	}
	if sampler, ok := root.(*elements.HttpSampler); ok {
		*out = append(*out, sampler)
	}
	for _, child := range root.GetChildren() {
		pa.collectHTTPSamplers(child, out)
	}
}

func (pa *PerfolizerApp) clearDebugConsole() {
	if pa.DebugConsoleList == nil {
		return
	}
	fyne.Do(func() {
		pa.DebugConsoleList.Objects = nil
		pa.DebugConsoleList.Refresh()
	})
}

func (pa *PerfolizerApp) appendDebugInfo(line string) {
	info := widget.NewRichText(
		&widget.TextSegment{
			Text: line,
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForeground,
			},
		},
	)
	pa.appendDebugItem(info)
}

func (pa *PerfolizerApp) appendDebugItem(item fyne.CanvasObject) {
	if pa.DebugConsoleList == nil {
		return
	}
	fyne.Do(func() {
		pa.DebugConsoleList.Add(item)
		if len(pa.DebugConsoleList.Objects) > maxDebugItems {
			pa.DebugConsoleList.Objects = pa.DebugConsoleList.Objects[len(pa.DebugConsoleList.Objects)-maxDebugItems:]
		}
		pa.DebugConsoleList.Refresh()
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

func (pa *PerfolizerApp) addPlan() {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("Thread Group 1", 1, 1)
	root.AddChild(tg)
	pa.Project.AddPlan("Test Plan", &root)
	pa.Tree.RefreshItem("")
	pa.Tree.OpenBranch(fmt.Sprintf("plan:%d", pa.Project.PlanCount()-1))
}

func (pa *PerfolizerApp) addElement() {
	planIdx, el := pa.resolveNode(pa.CurrentNodeID)
	if planIdx < 0 {
		return
	}
	var parent core.TestElement
	if el == nil {
		parent = pa.Project.Plans[planIdx].Root
	} else {
		parent = el
	}
	if parent == nil {
		return
	}

	// Simple dialog with buttons for now
	d := dialog.NewCustom("Select Element Type", "Cancel",
		container.NewVBox(
			widget.NewButton("Simple Thread Group", func() { pa.doAddElement(planIdx, parent, "Simple Thread Group") }),
			widget.NewButton("RPS Thread Group", func() { pa.doAddElement(planIdx, parent, "RPS Thread Group") }),
			widget.NewButton("HTTP Sampler", func() { pa.doAddElement(planIdx, parent, "HTTP Sampler") }),
			widget.NewButton("If Controller", func() { pa.doAddElement(planIdx, parent, "If Controller") }),
			widget.NewButton("Pause Controller", func() { pa.doAddElement(planIdx, parent, "Pause Controller") }),
		), pa.Window)
	d.Show()
}

func (pa *PerfolizerApp) doAddElement(planIdx int, parent core.TestElement, typeName string) {
	if top := pa.Window.Canvas().Overlays().Top(); top != nil {
		top.Hide()
	}

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
		treeID := pa.treeIDForElement(planIdx, parent)
		pa.Tree.RefreshItem(treeID)
		if treeID == fmt.Sprintf("plan:%d", planIdx) {
			pa.Tree.RefreshItem("")
		}
		pa.Tree.OpenBranch(treeID)
	}
}

func (pa *PerfolizerApp) removeElement() {
	if pa.CurrentNodeID == "" {
		return
	}
	planIdx, el := pa.resolveNode(pa.CurrentNodeID)
	if planIdx < 0 {
		return
	}
	if el == nil {
		// Removing a plan
		if pa.Project.PlanCount() <= 1 {
			dialog.ShowInformation("Error", "Cannot remove the last test plan", pa.Window)
			return
		}
		dialog.ShowConfirm("Remove plan", "Remove this test plan?", func(ok bool) {
			if ok {
				pa.Project.RemovePlanAt(planIdx)
				pa.Tree.RefreshItem("")
				pa.CurrentNodeID = ""
				pa.Content.Objects = nil
				pa.Content.Refresh()
			}
		}, pa.Window)
		return
	}
	root := pa.Project.Plans[planIdx].Root
	if el == root {
		dialog.ShowInformation("Error", "Cannot remove the plan root", pa.Window)
		return
	}
	parent := pa.findParent(root, el.ID())
	if parent != nil {
		parent.RemoveChild(el.ID())
		pa.Tree.RefreshItem(pa.treeIDForElement(planIdx, parent))
		pa.Content.Objects = nil
		pa.Content.Refresh()
		pa.CurrentNodeID = ""
	}
}

func (pa *PerfolizerApp) appendDebugSamplerCard(index, total int, sampler *elements.HttpSampler, exchange *core.DebugHTTPExchange, agentErr error) {
	requestMethod := sampler.Method
	requestURL := sampler.Url
	requestBody := sampler.Body
	duration := "-"
	outgoingHeaders := "<empty>"
	incomingHeaders := "<empty>"
	responseBody := "<empty>"
	statusText := "FAILED"
	statusColor := theme.ColorNameError
	errorText := ""
	success := false

	if exchange != nil {
		if exchange.Request.Method != "" {
			requestMethod = exchange.Request.Method
		}
		if exchange.Request.URL != "" {
			requestURL = exchange.Request.URL
		}
		if exchange.Request.Body != "" {
			requestBody = exchange.Request.Body
		}
		if exchange.DurationMilliseconds > 0 {
			duration = fmt.Sprintf("%d ms", exchange.DurationMilliseconds)
		}
		outgoingHeaders = formatHeadersText(exchange.Request.Headers)
		if exchange.RequestBodyTruncated {
			requestBody = truncatePreview(requestBody, maxBodyPreviewChars)
		}
		if exchange.Error != "" {
			errorText = exchange.Error
		}
		if exchange.Response != nil {
			statusText = fmt.Sprintf("%d (%s)", exchange.Response.StatusCode, exchange.Response.Status)
			incomingHeaders = formatHeadersText(exchange.Response.Headers)
			if exchange.Response.Body != "" {
				responseBody = exchange.Response.Body
			}
			if exchange.ResponseBodyTruncated {
				responseBody = truncatePreview(responseBody, maxBodyPreviewChars)
			}
			success = exchange.Response.StatusCode >= 200 && exchange.Response.StatusCode < 400
		}
	}

	if requestBody == "" {
		requestBody = "<empty>"
	} else {
		requestBody = truncatePreview(requestBody, maxBodyPreviewChars)
	}
	if responseBody != "<empty>" {
		responseBody = truncatePreview(responseBody, maxBodyPreviewChars)
	}
	if agentErr != nil {
		errorText = agentErr.Error()
	}
	if success {
		statusColor = theme.ColorNameSuccess
	}

	segments := make([]widget.RichTextSegment, 0, 28)

	appendSegment := func(text string, colorName fyne.ThemeColorName, textStyle fyne.TextStyle) {
		segments = append(segments, &widget.TextSegment{
			Text: text,
			Style: widget.RichTextStyle{
				ColorName: colorName,
				TextStyle: textStyle,
			},
		})
	}
	appendField := func(name, value string) {
		appendSegment(name+": ", theme.ColorNamePrimary, fyne.TextStyle{Bold: true})
		appendSegment(value+"\n", theme.ColorNameForeground, fyne.TextStyle{Monospace: true})
	}
	appendBlockField := func(name, value string) {
		appendSegment(name+":\n", theme.ColorNamePrimary, fyne.TextStyle{Bold: true})
		appendSegment(value+"\n\n", theme.ColorNameForeground, fyne.TextStyle{Monospace: true})
	}

	appendSegment(fmt.Sprintf("[%d/%d] Sampler: %s\n", index, total, sampler.Name()), theme.ColorNamePrimary, fyne.TextStyle{Bold: true})
	appendField("Request", fmt.Sprintf("%s %s", requestMethod, requestURL))
	appendField("Duration", duration)
	appendBlockField("Outgoing headers", outgoingHeaders)
	appendBlockField("Request body", requestBody)
	appendSegment("Status: ", theme.ColorNamePrimary, fyne.TextStyle{Bold: true})
	appendSegment(statusText+"\n", statusColor, fyne.TextStyle{Bold: true, Monospace: true})
	appendBlockField("Incoming headers", incomingHeaders)
	appendBlockField("Response body", responseBody)

	if errorText != "" {
		appendSegment("Error: ", theme.ColorNamePrimary, fyne.TextStyle{Bold: true})
		appendSegment(errorText+"\n", theme.ColorNameError, fyne.TextStyle{Monospace: true})
	}

	logText := widget.NewRichText(segments...)
	logText.Wrapping = fyne.TextWrapWord

	borderColor := theme.Color(theme.ColorNameSeparator)
	if !success || errorText != "" {
		borderColor = theme.Color(theme.ColorNameError)
	}

	background := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	background.CornerRadius = 6

	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = borderColor
	border.StrokeWidth = 2
	border.CornerRadius = 6

	card := container.NewStack(
		background,
		border,
		container.NewPadded(logText),
	)

	pa.appendDebugItem(container.NewPadded(card))
}

func formatHeadersText(headers map[string][]string) string {
	if len(headers) == 0 {
		return "<empty>"
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
			fmt.Fprintf(&b, "%s:\n", key)
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&b, "%s: %s\n", key, value)
		}
	}
	return strings.TrimSpace(b.String())
}

func truncatePreview(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + fmt.Sprintf("\n...[truncated, %d more chars]", len(value)-maxLen)
}
