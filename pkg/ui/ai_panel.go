package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aipkg "perfolizer/pkg/ai"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	aiActionDraft   = "Draft plan from URL/stats"
	aiActionRefine  = "Refine current plan"
	aiActionExplain = "Explain selected node / suggest better thread group"
)

func (pa *PerfolizerApp) buildAIPanel() fyne.CanvasObject {
	pa.aiStatusLabel = widget.NewLabel(pa.AISettings.Summary())
	pa.aiStatusLabel.Wrapping = fyne.TextWrapWord

	pa.aiContextLabel = widget.NewLabel(pa.buildAIContextSummary())
	pa.aiContextLabel.Wrapping = fyne.TextWrapWord

	pa.aiActionSelect = widget.NewSelect([]string{aiActionDraft, aiActionRefine, aiActionExplain}, nil)
	pa.aiActionSelect.SetSelected(aiActionDraft)

	pa.aiURLEntry = widget.NewEntry()
	pa.aiURLEntry.SetPlaceHolder("https://example.com")

	pa.aiGoalEntry = widget.NewMultiLineEntry()
	pa.aiGoalEntry.SetMinRowsVisible(4)
	pa.aiGoalEntry.SetPlaceHolder("Describe the workload or refinement you want.")
	pa.aiGoalEntry.Wrapping = fyne.TextWrapWord

	pa.aiStatsEntry = widget.NewMultiLineEntry()
	pa.aiStatsEntry.SetMinRowsVisible(6)
	pa.aiStatsEntry.SetPlaceHolder("Optional request stats or request list, one per line.")
	pa.aiStatsEntry.Wrapping = fyne.TextWrapWord

	pa.aiPreviewEntry = NewReadOnlyEntry()
	pa.aiPreviewEntry.SetMinRowsVisible(14)
	pa.aiPreviewEntry.Wrapping = fyne.TextWrapWord

	previewButton := widget.NewButtonWithIcon("Preview", theme.SearchReplaceIcon(), func() {
		pa.runAIAction()
	})
	pa.aiApplyButton = widget.NewButtonWithIcon("Apply", theme.DocumentSaveIcon(), func() {
		pa.applyAIResult()
	})
	pa.aiApplyButton.Disable()

	clearButton := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		pa.clearAIResult()
	})
	configureButton := widget.NewButtonWithIcon("Configure", theme.SettingsIcon(), func() {
		pa.showPreferencesWithSection("AI")
	})

	return container.NewPadded(widget.NewCard("AI Assistant", "", container.NewBorder(
		container.NewVBox(
			pa.aiStatusLabel,
			widget.NewSeparator(),
			widget.NewLabel("Context"),
			pa.aiContextLabel,
		),
		nil,
		nil,
		nil,
		container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Action", pa.aiActionSelect),
				widget.NewFormItem("Base URL", pa.aiURLEntry),
				widget.NewFormItem("Goal", pa.aiGoalEntry),
				widget.NewFormItem("Request stats", pa.aiStatsEntry),
			),
			container.NewHBox(previewButton, pa.aiApplyButton, clearButton, configureButton),
			widget.NewSeparator(),
			widget.NewLabel("Preview"),
			pa.aiPreviewEntry,
		),
	)))
}

func (pa *PerfolizerApp) toggleAIPanel() {
	if !pa.isAIAvailable() {
		pa.showPreferencesWithSection("AI")
		return
	}
	pa.aiPanelVisible = !pa.aiPanelVisible
	pa.updateAIPanelState()
}

func (pa *PerfolizerApp) updateAIPanelState() {
	if pa.aiStatusLabel != nil {
		pa.aiStatusLabel.SetText(pa.AISettings.Summary())
	}
	if pa.aiContextLabel != nil {
		pa.aiContextLabel.SetText(pa.buildAIContextSummary())
	}
	if pa.aiToggleButton != nil {
		switch {
		case !pa.AISettings.Enabled || !pa.AISettings.IsConfigured():
			pa.aiToggleButton.SetText("AI Setup")
		case pa.aiPanelVisible:
			pa.aiToggleButton.SetText("Hide AI")
		default:
			pa.aiToggleButton.SetText("Show AI")
		}
	}
	if pa.mainContentHost == nil {
		return
	}
	if pa.aiPanelVisible && pa.isAIAvailable() && pa.workspaceContent != nil && pa.aiPanel != nil {
		split := container.NewHSplit(pa.workspaceContent, pa.aiPanel)
		split.SetOffset(0.74)
		pa.mainContentHost.Objects = []fyne.CanvasObject{split}
	} else if pa.workspaceContent != nil {
		pa.mainContentHost.Objects = []fyne.CanvasObject{pa.workspaceContent}
	}
	pa.mainContentHost.Refresh()
}

func (pa *PerfolizerApp) runAIAction() {
	if pa.aiPreviewEntry == nil {
		return
	}
	if !pa.isAIAvailable() {
		pa.showPreferencesWithSection("AI")
		return
	}

	pa.aiPreviewEntry.SetText("Preparing preview...")
	pa.aiApplyButton.Disable()

	action := pa.aiActionSelect.Selected
	if strings.TrimSpace(action) == "" {
		action = aiActionDraft
	}

	go func() {
		var previewText string
		var draft *aipkg.PlanDraft
		var patch *aipkg.PlanPatch
		var err error

		switch action {
		case aiActionDraft:
			brief := aipkg.WorkloadBrief{
				URL:              strings.TrimSpace(pa.aiURLEntry.Text),
				Goal:             strings.TrimSpace(pa.aiGoalEntry.Text),
				RequestStats:     strings.TrimSpace(pa.aiStatsEntry.Text),
				SelectionSummary: pa.buildAIContextSummary(),
			}
			var generated aipkg.PlanDraft
			generated, err = pa.aiEngine.GenerateDraft(context.Background(), brief)
			if err == nil {
				draft = &generated
				previewText = renderDraftPreview(generated)
			}
		case aiActionRefine:
			plan := pa.getCurrentPlan()
			if plan == nil {
				err = fmt.Errorf("no active test plan")
				break
			}
			var generated aipkg.PlanPatch
			generated, err = pa.aiEngine.SuggestPatch(
				context.Background(),
				plan,
				strings.TrimSpace(pa.aiGoalEntry.Text),
				selectedElementID(pa.CurrentNodeID),
				pa.currentPlanParameters(),
			)
			if err == nil {
				patch = &generated
				previewText = renderPatchPreview(generated)
			}
		case aiActionExplain:
			_, selected := pa.resolveNode(pa.CurrentNodeID)
			explanation := pa.aiEngine.ExplainSelection(pa.getCurrentPlan(), selected, strings.TrimSpace(pa.aiGoalEntry.Text))
			previewText = renderExplanationPreview(explanation)
		default:
			err = fmt.Errorf("unsupported AI action")
		}

		fyne.Do(func() {
			pa.lastAIDraft = draft
			pa.lastAIPatch = patch
			pa.lastAIAction = action
			if err != nil {
				pa.aiPreviewEntry.SetText(err.Error())
				pa.aiApplyButton.Disable()
				return
			}
			pa.aiPreviewEntry.SetText(previewText)
			if draft != nil || patch != nil {
				pa.aiApplyButton.Enable()
			} else {
				pa.aiApplyButton.Disable()
			}
		})
	}()
}

func (pa *PerfolizerApp) applyAIResult() {
	planIdx := pa.getCurrentPlanIndex()
	if pa.Project == nil || planIdx < 0 || planIdx >= pa.Project.PlanCount() {
		return
	}

	selectedNodeID := ""
	successMessage := ""

	switch {
	case pa.lastAIDraft != nil:
		root, err := aipkg.ValidateDraft(*pa.lastAIDraft)
		if err != nil {
			dialog.ShowError(err, pa.Window)
			return
		}
		insertedPlanIdx := pa.addAIDraftPlan(root)
		selectedNodeID = fmt.Sprintf("plan:%d", insertedPlanIdx)
		successMessage = "Preview added as a new test plan in the current project."
	case pa.lastAIPatch != nil:
		current := pa.Project.Plans[planIdx].Root
		root, err := aipkg.ApplyPatch(current, *pa.lastAIPatch)
		if err != nil {
			dialog.ShowError(err, pa.Window)
			return
		}
		pa.Project.Plans[planIdx].Root = root
		selectedNodeID = fmt.Sprintf("plan:%d", planIdx)
		successMessage = "Preview applied to the current test plan."
	default:
		return
	}

	if selectedNodeID == "" {
		return
	}

	pa.CurrentNodeID = selectedNodeID
	pa.Tree.Refresh()
	pa.Tree.OpenBranch(pa.CurrentNodeID)
	pa.Tree.Select(pa.CurrentNodeID)
	if pa.ParameterManager != nil {
		pa.ParameterManager.Refresh()
	}
	pa.clearAIResult()
	dialog.ShowInformation("AI", successMessage, pa.Window)
}

func (pa *PerfolizerApp) clearAIResult() {
	pa.lastAIDraft = nil
	pa.lastAIPatch = nil
	pa.lastAIAction = ""
	if pa.aiPreviewEntry != nil {
		pa.aiPreviewEntry.SetText("")
	}
	if pa.aiApplyButton != nil {
		pa.aiApplyButton.Disable()
	}
}

func (pa *PerfolizerApp) buildAIContextSummary() string {
	parts := make([]string, 0, 4)
	parts = append(parts, fmt.Sprintf("Plan: %s", pa.currentPlanDisplayName()))

	_, selected := pa.resolveNode(pa.CurrentNodeID)
	if selected == nil {
		parts = append(parts, "Selected node: none")
	} else {
		parts = append(parts, fmt.Sprintf("Selected node: %s", selected.Name()))
		switch current := selected.(type) {
		case *elements.HttpSampler:
			parts = append(parts, fmt.Sprintf("Request: %s %s", strings.ToUpper(current.Method), current.Url))
		case *elements.SimpleThreadGroup:
			parts = append(parts, fmt.Sprintf("Users: %d, Iterations: %d", current.Users, current.Iterations))
		case *elements.RPSThreadGroup:
			parts = append(parts, fmt.Sprintf("Users: %d, RPS: %.2f", current.Users, current.RPS))
		}
	}

	params := pa.currentPlanParameters()
	if len(params) > 0 {
		parts = append(parts, fmt.Sprintf("Plan parameters: %d", len(params)))
	}
	return strings.Join(parts, "\n")
}

func (pa *PerfolizerApp) currentPlanParameters() []core.Parameter {
	planIdx := pa.getCurrentPlanIndex()
	if pa.Project == nil || planIdx < 0 || planIdx >= pa.Project.PlanCount() {
		return nil
	}
	params := pa.Project.Plans[planIdx].Parameters
	copied := make([]core.Parameter, len(params))
	copy(copied, params)
	return copied
}

func (pa *PerfolizerApp) addAIDraftPlan(root core.TestElement) int {
	planName := nextGeneratedPlanName(pa.Project, root.Name())
	root.SetName(planName)
	pa.Project.AddPlan(planName, root)
	return pa.Project.PlanCount() - 1
}

func nextGeneratedPlanName(project *core.Project, suggested string) string {
	baseName := strings.TrimSpace(suggested)
	if baseName == "" {
		baseName = "AI Draft"
	}
	if project == nil {
		return baseName
	}

	nameTaken := func(candidate string) bool {
		for _, plan := range project.Plans {
			if strings.EqualFold(strings.TrimSpace(plan.Name), candidate) {
				return true
			}
		}
		return false
	}

	if !nameTaken(baseName) {
		return baseName
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s (%d)", baseName, suffix)
		if !nameTaken(candidate) {
			return candidate
		}
	}
}

func renderDraftPreview(draft aipkg.PlanDraft) string {
	var b strings.Builder
	if draft.Rationale != "" {
		b.WriteString("Rationale:\n")
		b.WriteString(draft.Rationale)
		b.WriteString("\n\n")
	}
	if len(draft.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range draft.Warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Draft JSON:\n")
	b.WriteString(prettyJSON(draft.Root))
	return strings.TrimSpace(b.String())
}

func renderPatchPreview(patch aipkg.PlanPatch) string {
	var b strings.Builder
	lines := aipkg.SummarizePatch(patch)
	if len(lines) > 0 {
		b.WriteString("Summary:\n")
		for _, line := range lines {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(patch.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range patch.Warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Patch JSON:\n")
	b.WriteString(prettyJSON(patch))
	return strings.TrimSpace(b.String())
}

func renderExplanationPreview(explanation aipkg.SelectionExplanation) string {
	var b strings.Builder
	if explanation.Summary != "" {
		b.WriteString("Summary:\n")
		b.WriteString(explanation.Summary)
		b.WriteString("\n\n")
	}
	if explanation.Recommendation != "" {
		b.WriteString("Recommendation:\n")
		b.WriteString(explanation.Recommendation)
		b.WriteString("\n")
	}
	if len(explanation.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, warning := range explanation.Warnings {
			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func prettyJSON(value interface{}) string {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func selectedElementID(nodeID string) string {
	parts := strings.SplitN(nodeID, ":", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return ""
}
