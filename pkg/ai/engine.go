package ai

import (
	"context"
	"errors"
	"fmt"

	"perfolizer/pkg/core"
)

var ErrProviderUnavailable = errors.New("ai provider is not configured")

type Provider interface {
	Name() string
	GenerateDraft(ctx context.Context, req GenerateDraftRequest) (PlanDraft, error)
	SuggestPatch(ctx context.Context, req SuggestPatchRequest) (PlanPatch, error)
	AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error)
	TestConnection(ctx context.Context) error
}

type Engine struct {
	settings AISettings
	provider Provider
}

func NewEngine(settings AISettings) *Engine {
	normalized := settings.Normalize()
	return &Engine{
		settings: normalized,
		provider: newProvider(normalized),
	}
}

func (e *Engine) Settings() AISettings {
	return e.settings
}

func (e *Engine) GenerateDraft(ctx context.Context, brief WorkloadBrief) (PlanDraft, error) {
	if draft, matched, err := TryGenerateRuleBasedDraft(brief); matched || err != nil {
		return draft, err
	}
	if e.provider == nil {
		return PlanDraft{}, ErrProviderUnavailable
	}
	return e.provider.GenerateDraft(ctx, GenerateDraftRequest{Brief: brief})
}

func (e *Engine) SuggestPatch(ctx context.Context, root core.TestElement, intent, selectedID string, params []core.Parameter) (PlanPatch, error) {
	if root == nil {
		return PlanPatch{}, fmt.Errorf("current plan is required")
	}
	req := SuggestPatchRequest{
		Intent:      intent,
		CurrentPlan: core.TestElementToDTO(root),
		SelectedID:  selectedID,
		Parameters:  params,
	}
	if patch, matched, err := TrySuggestRuleBasedPatch(root, req); matched || err != nil {
		return patch, err
	}
	if e.provider == nil {
		return PlanPatch{}, ErrProviderUnavailable
	}
	return e.provider.SuggestPatch(ctx, req)
}

func (e *Engine) AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error) {
	if e.provider == nil {
		return RunAnalysis{}, ErrProviderUnavailable
	}
	return e.provider.AnalyzeRun(ctx, req)
}

func (e *Engine) TestConnection(ctx context.Context) error {
	if e.provider == nil {
		return ErrProviderUnavailable
	}
	return e.provider.TestConnection(ctx)
}

func (e *Engine) ExplainSelection(root core.TestElement, selected core.TestElement, intent string) SelectionExplanation {
	return ExplainSelection(root, selected, intent)
}
