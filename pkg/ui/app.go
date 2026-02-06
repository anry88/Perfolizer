package ui

import (
	"context"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type PerfolizerApp struct {
	FyneApp fyne.App
	Window  fyne.Window
	Tree    *widget.Tree
	Content *fyne.Container

	TestPlan      core.TestElement // Root of the plan
	CurrentNodeID string
}

func NewPerfolizerApp() *PerfolizerApp {
	a := app.New()
	w := a.NewWindow("Perfolizer")
	w.Resize(fyne.NewSize(1024, 768))

	pa := &PerfolizerApp{
		FyneApp: a,
		Window:  w,
		Content: container.NewMax(widget.NewLabel("Select a node to edit")),
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

	// 2. Toolbar (Top)
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ContentAddIcon(), func() { pa.addElement() }),       // Add
		widget.NewToolbarAction(theme.ContentRemoveIcon(), func() { pa.removeElement() }), // Remove
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.MediaPlayIcon(), func() { pa.runTest() }), // Start
		widget.NewToolbarAction(theme.MediaStopIcon(), func() {}),               // Stop
	)

	// 3. Layout
	split := container.NewHSplit(
		container.NewBorder(nil, nil, nil, nil, pa.Tree),
		pa.Content,
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

		form.Append("URL", urlEntry)
		form.Append("Method", methodEntry)

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
	}

	pa.Content.Objects = []fyne.CanvasObject{container.NewVBox(widget.NewLabel("Properties"), form)}
	pa.Content.Refresh()
}

func (pa *PerfolizerApp) runTest() {
	dashboard := NewDashboardWindow(pa.FyneApp)
	dashboard.Show()

	runner := core.NewStatsRunner(func(rps float64, avgLat float64) {
		dashboard.Update(rps, avgLat)
	})

	go func() {
		// Find all thread groups and execute them
		// TODO: This should recursive search
		children := pa.TestPlan.GetChildren()
		for _, child := range children {
			if tg, ok := child.(core.ThreadGroup); ok {
				// We need a context for cancellation
				// For now simple background
				tg.Start(context.Background(), runner)
			}
		}
	}()
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
