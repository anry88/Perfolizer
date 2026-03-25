package ai

import (
	"context"
	"testing"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

type testProvider struct {
	generateDraftCalls int
}

func (p *testProvider) Name() string {
	return "test"
}

func (p *testProvider) GenerateDraft(ctx context.Context, req GenerateDraftRequest) (PlanDraft, error) {
	p.generateDraftCalls++
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("Provider TG", 1, 1)
	root.AddChild(tg)
	return PlanDraft{
		Root:      core.TestElementToDTO(&root),
		Rationale: "provider draft",
		Source:    "test",
	}, nil
}

func (p *testProvider) SuggestPatch(ctx context.Context, req SuggestPatchRequest) (PlanPatch, error) {
	return PlanPatch{}, nil
}

func (p *testProvider) AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error) {
	return RunAnalysis{}, nil
}

func (p *testProvider) TestConnection(ctx context.Context) error {
	return nil
}

func TestEngineGenerateDraftFallsBackToProviderForNarrativeScenario(t *testing.T) {
	provider := &testProvider{}
	engine := &Engine{
		settings: DefaultSettings(),
		provider: provider,
	}

	draft, err := engine.GenerateDraft(context.Background(), WorkloadBrief{
		URL:          "https://www.google.com/",
		Goal:         "нужно собрать мини план для тестирования главной страницы + 5 простых поисковых запросов.",
		RequestStats: "основная страница 5 рпс, поисковые запросы каждый по 1 рпс.",
	})
	if err != nil {
		t.Fatalf("GenerateDraft returned error: %v", err)
	}
	if provider.generateDraftCalls != 1 {
		t.Fatalf("expected provider GenerateDraft to be called once, got %d", provider.generateDraftCalls)
	}
	if draft.Source != "test" {
		t.Fatalf("expected provider draft source, got %q", draft.Source)
	}
}
