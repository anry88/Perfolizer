package ui

import (
	"fmt"
	"perfolizer/pkg/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ParameterManager manages the UI for project parameters.
type ParameterManager struct {
	Container *fyne.Container
	List      *widget.List
	App       *PerfolizerApp

	params []core.Parameter // Local copy for display
}

func NewParameterManager(app *PerfolizerApp) *ParameterManager {
	pm := &ParameterManager{
		App: app,
	}
	pm.setupUI()
	return pm
}

func (pm *ParameterManager) setupUI() {
	pm.List = widget.NewList(
		func() int {
			return len(pm.params)
		},
		func() fyne.CanvasObject {
			// Create template:Grid with Type, Name, Value, Expression, and Action buttons
			return container.NewGridWithColumns(5,
				widget.NewLabel(""), // Type
				widget.NewLabel(""), // Name
				widget.NewLabel(""), // Value
				widget.NewLabel(""), // Expression
				container.NewHBox( // Action buttons
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
				),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			p := pm.params[i]
			grid := o.(*fyne.Container)

			// Update Type
			typeLabel := grid.Objects[0].(*widget.Label)
			typeLabel.SetText(p.Type)

			// Update Name
			nameLabel := grid.Objects[1].(*widget.Label)
			nameLabel.SetText(p.Name)

			// Update Value
			valueLabel := grid.Objects[2].(*widget.Label)
			valueLabel.SetText(p.Value)

			// Update Expression
			exprLabel := grid.Objects[3].(*widget.Label)
			if p.Type == core.ParamTypeRegexp {
				exprLabel.SetText(p.Expression)
			} else {
				exprLabel.SetText("-")
			}

			// Update Action buttons
			btns := grid.Objects[4].(*fyne.Container)
			editBtn := btns.Objects[0].(*widget.Button)
			delBtn := btns.Objects[1].(*widget.Button)

			editBtn.OnTapped = func() {
				pm.showEditDialog(i)
			}
			delBtn.OnTapped = func() {
				pm.deleteParameter(i)
			}
		},
	)

	addBtn := widget.NewButtonWithIcon("Add Parameter", theme.ContentAddIcon(), func() {
		pm.showAddDialog()
	})

	// Header
	header := container.NewGridWithColumns(5,
		widget.NewLabel("Type"),
		widget.NewLabel("Name"),
		widget.NewLabel("Value / Default"),
		widget.NewLabel("Expression"),
		widget.NewLabel("Action"),
	)

	pm.Container = container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Parameters", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			header,
		),
		addBtn, nil, nil,
		pm.List,
	)
}

func (pm *ParameterManager) Refresh() {
	planIdx := pm.App.getCurrentPlanIndex()
	if pm.App.Project == nil || planIdx < 0 || planIdx >= pm.App.Project.PlanCount() {
		pm.params = nil
	} else {
		pm.params = pm.App.Project.Plans[planIdx].Parameters
	}
	pm.List.Refresh()
}

func (pm *ParameterManager) showAddDialog() {
	planIdx := pm.App.getCurrentPlanIndex()
	if pm.App.Project == nil || planIdx < 0 {
		return
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Name")
	valueEntry := widget.NewEntry()
	valueEntry.SetPlaceHolder("Value")
	exprEntry := widget.NewEntry()
	exprEntry.SetPlaceHolder("Expression (Regex or JSON Path)")

	typeSelect := widget.NewSelect([]string{core.ParamTypeStatic, core.ParamTypeRegexp, core.ParamTypeJSON}, nil)
	typeSelect.SetSelected(core.ParamTypeStatic) // Default to static

	// Create form container
	formContainer := container.NewVBox()

	// Track if dialog is still open
	dialogOpen := true

	// Helper to update form based on type
	updateForm := func() {
		if !dialogOpen {
			return // Prevent updates after dialog is closed
		}
		formContainer.Objects = nil
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Type:"), nil,
			typeSelect,
		))
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Name:"), nil,
			nameEntry,
		))
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Value:"), nil,
			valueEntry,
		))

		if typeSelect.Selected == core.ParamTypeRegexp || typeSelect.Selected == core.ParamTypeJSON {
			formContainer.Add(container.NewBorder(nil, nil,
				widget.NewLabel("Expression:"), nil,
				exprEntry,
			))
		}
		formContainer.Refresh()
	}

	// Update form when type changes
	typeSelect.OnChanged = func(s string) {
		updateForm()
	}

	// Initialize form
	updateForm()

	// Create dialog
	customDialog := dialog.NewCustomConfirm("Add Parameter", "Add", "Cancel", formContainer, func(confirm bool) {
		dialogOpen = false // Mark dialog as closed
		if !confirm {
			return
		}

		// Validate name is not empty
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Parameter name cannot be empty"), pm.App.Window)
			return
		}

		// Check for uniqueness
		for _, existing := range pm.App.Project.Plans[planIdx].Parameters {
			if existing.Name == nameEntry.Text {
				dialog.ShowError(fmt.Errorf("Parameter with name %q already exists", nameEntry.Text), pm.App.Window)
				return
			}
		}

		if pm.App.Project.Plans[planIdx].Parameters == nil {
			pm.App.Project.Plans[planIdx].Parameters = make([]core.Parameter, 0)
		}
		newParam := core.Parameter{
			ID:         core.GenerateID(),
			Name:       nameEntry.Text,
			Type:       typeSelect.Selected,
			Value:      valueEntry.Text,
			Expression: exprEntry.Text,
		}
		pm.App.Project.Plans[planIdx].Parameters = append(pm.App.Project.Plans[planIdx].Parameters, newParam)
		pm.Refresh()
	}, pm.App.Window)

	customDialog.Resize(fyne.NewSize(500, 200))
	customDialog.Show()
}

func (pm *ParameterManager) showEditDialog(index int) {
	planIdx := pm.App.getCurrentPlanIndex()
	if pm.App.Project == nil || planIdx < 0 || index >= len(pm.App.Project.Plans[planIdx].Parameters) {
		return
	}

	p := pm.App.Project.Plans[planIdx].Parameters[index]

	// Form fields
	nameEntry := widget.NewEntry()
	nameEntry.SetText(p.Name)
	nameEntry.SetPlaceHolder("Parameter Name")

	typeSelect := widget.NewSelect([]string{core.ParamTypeStatic, core.ParamTypeRegexp, core.ParamTypeJSON}, nil)
	typeSelect.SetSelected(p.Type)
	if typeSelect.Selected == "" {
		typeSelect.SetSelected(core.ParamTypeStatic)
	}

	valueEntry := widget.NewEntry()
	valueEntry.SetText(p.Value)
	valueEntry.SetPlaceHolder("Default Value")

	exprEntry := widget.NewEntry()
	exprEntry.SetText(p.Expression)
	exprEntry.SetPlaceHolder("Regex / JSON Path")

	// Create form container
	formContainer := container.NewVBox()

	// Track if dialog is still open
	dialogOpen := true

	// Helper to update form based on type
	updateForm := func() {
		if !dialogOpen {
			return // Prevent updates after dialog is closed
		}
		formContainer.Objects = nil
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Type:"), nil,
			typeSelect,
		))
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Name:"), nil,
			nameEntry,
		))
		formContainer.Add(container.NewBorder(nil, nil,
			widget.NewLabel("Value:"), nil,
			valueEntry,
		))

		if typeSelect.Selected == core.ParamTypeRegexp || typeSelect.Selected == core.ParamTypeJSON {
			formContainer.Add(container.NewBorder(nil, nil,
				widget.NewLabel("Expression:"), nil,
				exprEntry,
			))
		}
		formContainer.Refresh()
	}

	// Update form when type changes
	typeSelect.OnChanged = func(s string) {
		updateForm()
	}

	// Initialize form
	updateForm()

	// Create dialog
	customDialog := dialog.NewCustomConfirm("Edit Parameter", "Save", "Cancel", formContainer, func(confirm bool) {
		dialogOpen = false // Mark dialog as closed
		if !confirm {
			return
		}

		// Validate name is not empty
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Parameter name cannot be empty"), pm.App.Window)
			return
		}

		// Check for uniqueness (excluding current parameter)
		for i, existing := range pm.App.Project.Plans[planIdx].Parameters {
			if i != index && existing.Name == nameEntry.Text {
				dialog.ShowError(fmt.Errorf("Parameter with name %q already exists", nameEntry.Text), pm.App.Window)
				return
			}
		}

		pm.App.Project.Plans[planIdx].Parameters[index].Name = nameEntry.Text
		pm.App.Project.Plans[planIdx].Parameters[index].Type = typeSelect.Selected
		pm.App.Project.Plans[planIdx].Parameters[index].Value = valueEntry.Text
		pm.App.Project.Plans[planIdx].Parameters[index].Expression = exprEntry.Text
		pm.Refresh()
	}, pm.App.Window)

	customDialog.Show()
}

func (pm *ParameterManager) deleteParameter(index int) {
	planIdx := pm.App.getCurrentPlanIndex()
	if pm.App.Project == nil || planIdx < 0 {
		return
	}

	params := pm.App.Project.Plans[planIdx].Parameters
	pm.App.Project.Plans[planIdx].Parameters = append(params[:index], params[index+1:]...)
	pm.Refresh()
}
