package ai_test

import (
	"testing"
	"time"

	aipkg "perfolizer/pkg/ai"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

func TestValidateDraftRejectsUnknownElementType(t *testing.T) {
	_, err := aipkg.ValidateDraft(aipkg.PlanDraft{
		Root: core.TestElementDTO{
			Type: "UnknownElement",
			Name: "Broken",
		},
	})
	if err == nil {
		t.Fatal("expected draft validation to fail for an unknown element type")
	}
}

func TestApplyPatchRejectsRootRemoval(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG", 1, 1)
	root.AddChild(tg)

	_, err := aipkg.ApplyPatch(&root, aipkg.PlanPatch{
		Operations: []aipkg.PatchOperation{{
			Type:     "remove",
			TargetID: root.ID(),
		}},
	})
	if err == nil {
		t.Fatal("expected removing the root to fail")
	}
}

func TestTrySuggestRuleBasedPatchPreservesScenarioChildrenWhenSwitchingThreadGroup(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG", 1, 1)
	sampler := elements.NewHttpSampler("Orders", "GET", "https://example.com/api/orders")
	tg.AddChild(sampler)
	root.AddChild(tg)

	patch, matched, err := aipkg.TrySuggestRuleBasedPatch(&root, aipkg.SuggestPatchRequest{
		Intent:     "Change this to 200 rps for 2 minutes",
		SelectedID: tg.ID(),
	})
	if err != nil {
		t.Fatalf("TrySuggestRuleBasedPatch returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected rule-based patch generation to match")
	}

	patchedRoot, err := aipkg.ApplyPatch(&root, patch)
	if err != nil {
		t.Fatalf("ApplyPatch returned error: %v", err)
	}

	replacement, ok := patchedRoot.GetChildren()[0].(*elements.RPSThreadGroup)
	if !ok {
		t.Fatalf("expected RPSThreadGroup after patch, got %T", patchedRoot.GetChildren()[0])
	}
	if replacement.RPS != 200 {
		t.Fatalf("expected RPS 200, got %.2f", replacement.RPS)
	}
	if len(replacement.GetChildren()) != 1 {
		t.Fatalf("expected preserved child sampler, got %d child(ren)", len(replacement.GetChildren()))
	}
	if _, ok := replacement.GetChildren()[0].(*elements.HttpSampler); !ok {
		t.Fatalf("expected preserved HttpSampler, got %T", replacement.GetChildren()[0])
	}
}

func TestValidateDraftNormalizesProviderStyleAliases(t *testing.T) {
	root, err := aipkg.ValidateDraft(aipkg.PlanDraft{
		Root: core.TestElementDTO{
			Type: "TestPlan",
			Name: "Google Home + Random Searches",
			Children: []core.TestElementDTO{
				{
					Type: "RPSThreadGroup",
					Name: "Home Page Open RPS",
					Props: map[string]interface{}{
						"TargetRPS":       5,
						"DurationSeconds": 60,
						"RampUpSeconds":   10,
					},
					Children: []core.TestElementDTO{
						{
							Type: "HttpSampler",
							Name: "GET Google Home",
							Props: map[string]interface{}{
								"Method": "GET",
								"URL":    "https://www.google.com/",
							},
						},
					},
				},
				{
					Type: "SimpleThreadGroup",
					Name: "Five Random Searches",
					Props: map[string]interface{}{
						"Threads":    1,
						"Iterations": 5,
					},
					Children: []core.TestElementDTO{
						{
							Type: "HttpSampler",
							Name: "GET Google Search Random Query",
							Props: map[string]interface{}{
								"Method":    "GET",
								"URL":       "https://www.google.com/search",
								"TargetRPS": 1,
								"QueryParams": map[string]interface{}{
									"q": "${RANDOM_QUERY}",
								},
							},
						},
					},
				},
				{
					Type: "PauseController",
					Name: "Short Pause",
					Props: map[string]interface{}{
						"DurationMs": 500,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ValidateDraft returned error: %v", err)
	}

	children := root.GetChildren()
	if len(children) != 3 {
		t.Fatalf("expected 3 top-level children, got %d", len(children))
	}

	homeTG, ok := children[0].(*elements.RPSThreadGroup)
	if !ok {
		t.Fatalf("expected first child RPSThreadGroup, got %T", children[0])
	}
	if homeTG.RPS != 5 {
		t.Fatalf("expected RPS 5, got %.2f", homeTG.RPS)
	}
	if len(homeTG.ProfileBlocks) != 1 {
		t.Fatalf("expected a single profile block, got %#v", homeTG.ProfileBlocks)
	}
	if homeTG.ProfileBlocks[0].RampUp != 10*time.Second {
		t.Fatalf("expected 10s ramp-up, got %s", homeTG.ProfileBlocks[0].RampUp)
	}
	if homeTG.ProfileBlocks[0].StepDuration != 60*time.Second {
		t.Fatalf("expected 60s step duration, got %s", homeTG.ProfileBlocks[0].StepDuration)
	}
	homeSampler, ok := homeTG.GetChildren()[0].(*elements.HttpSampler)
	if !ok {
		t.Fatalf("expected home sampler, got %T", homeTG.GetChildren()[0])
	}
	if homeSampler.Url != "https://www.google.com/" {
		t.Fatalf("expected home sampler URL to be preserved, got %q", homeSampler.Url)
	}

	searchTG, ok := children[1].(*elements.SimpleThreadGroup)
	if !ok {
		t.Fatalf("expected second child SimpleThreadGroup, got %T", children[1])
	}
	if searchTG.Users != 1 {
		t.Fatalf("expected 1 user, got %d", searchTG.Users)
	}
	if searchTG.Iterations != 5 {
		t.Fatalf("expected 5 iterations, got %d", searchTG.Iterations)
	}
	searchSampler, ok := searchTG.GetChildren()[0].(*elements.HttpSampler)
	if !ok {
		t.Fatalf("expected search sampler, got %T", searchTG.GetChildren()[0])
	}
	if searchSampler.Url != "https://www.google.com/search?q=${RANDOM_QUERY}" {
		t.Fatalf("expected query parameter to be folded into sampler URL, got %q", searchSampler.Url)
	}
	if searchSampler.TargetRPS != 1 {
		t.Fatalf("expected sampler TargetRPS 1, got %.2f", searchSampler.TargetRPS)
	}

	pause, ok := children[2].(*elements.PauseController)
	if !ok {
		t.Fatalf("expected third child PauseController, got %T", children[2])
	}
	if pause.Duration != 500*time.Millisecond {
		t.Fatalf("expected 500ms pause, got %s", pause.Duration)
	}
}
