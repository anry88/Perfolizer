package ai_test

import (
	"testing"
	"time"

	aipkg "perfolizer/pkg/ai"
	"perfolizer/pkg/elements"
)

func TestTryGenerateRuleBasedDraftUsesSimpleThreadGroupForIterations(t *testing.T) {
	draft, matched, err := aipkg.TryGenerateRuleBasedDraft(aipkg.WorkloadBrief{
		URL:  "https://example.com/api/orders",
		Goal: "Create a test plan with 1 million iterations for this link",
	})
	if err != nil {
		t.Fatalf("TryGenerateRuleBasedDraft returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected rule-based draft generation to match")
	}

	root, err := aipkg.ValidateDraft(draft)
	if err != nil {
		t.Fatalf("ValidateDraft returned error: %v", err)
	}

	tg, ok := root.GetChildren()[0].(*elements.SimpleThreadGroup)
	if !ok {
		t.Fatalf("expected SimpleThreadGroup, got %T", root.GetChildren()[0])
	}
	if tg.Iterations != 1_000_000 {
		t.Fatalf("expected 1000000 iterations, got %d", tg.Iterations)
	}
	if tg.Users != 1 {
		t.Fatalf("expected default 1 user, got %d", tg.Users)
	}
	if len(tg.GetChildren()) != 1 {
		t.Fatalf("expected 1 sampler, got %d", len(tg.GetChildren()))
	}
}

func TestTryGenerateRuleBasedDraftUsesRPSThreadGroupForTargetRPS(t *testing.T) {
	draft, matched, err := aipkg.TryGenerateRuleBasedDraft(aipkg.WorkloadBrief{
		URL:  "https://example.com/api/search",
		Goal: "Run 250 rps for 5 minutes with 30 users",
	})
	if err != nil {
		t.Fatalf("TryGenerateRuleBasedDraft returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected rule-based draft generation to match")
	}

	root, err := aipkg.ValidateDraft(draft)
	if err != nil {
		t.Fatalf("ValidateDraft returned error: %v", err)
	}

	tg, ok := root.GetChildren()[0].(*elements.RPSThreadGroup)
	if !ok {
		t.Fatalf("expected RPSThreadGroup, got %T", root.GetChildren()[0])
	}
	if tg.RPS != 250 {
		t.Fatalf("expected RPS 250, got %.2f", tg.RPS)
	}
	if tg.Users != 30 {
		t.Fatalf("expected max users 30, got %d", tg.Users)
	}
	if len(tg.ProfileBlocks) != 1 || tg.ProfileBlocks[0].StepDuration != 5*time.Minute {
		t.Fatalf("expected single 5 minute profile block, got %#v", tg.ProfileBlocks)
	}
}

func TestTryGenerateRuleBasedDraftRecognizesCyrillicRPS(t *testing.T) {
	draft, matched, err := aipkg.TryGenerateRuleBasedDraft(aipkg.WorkloadBrief{
		URL:  "https://example.com/api/search",
		Goal: "Run 250 рпс for 5 minutes with 30 users",
	})
	if err != nil {
		t.Fatalf("TryGenerateRuleBasedDraft returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected rule-based draft generation to match")
	}

	root, err := aipkg.ValidateDraft(draft)
	if err != nil {
		t.Fatalf("ValidateDraft returned error: %v", err)
	}

	tg, ok := root.GetChildren()[0].(*elements.RPSThreadGroup)
	if !ok {
		t.Fatalf("expected RPSThreadGroup, got %T", root.GetChildren()[0])
	}
	if tg.RPS != 250 {
		t.Fatalf("expected RPS 250, got %.2f", tg.RPS)
	}
}

func TestTryGenerateRuleBasedDraftCreatesSamplersFromRequestStats(t *testing.T) {
	draft, matched, err := aipkg.TryGenerateRuleBasedDraft(aipkg.WorkloadBrief{
		URL:          "https://example.com",
		RequestStats: "GET /api/orders\nPOST https://example.com/api/payments",
		Goal:         "Draft a plan from these requests",
	})
	if err != nil {
		t.Fatalf("TryGenerateRuleBasedDraft returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected rule-based draft generation to match")
	}

	root, err := aipkg.ValidateDraft(draft)
	if err != nil {
		t.Fatalf("ValidateDraft returned error: %v", err)
	}

	tg, ok := root.GetChildren()[0].(*elements.SimpleThreadGroup)
	if !ok {
		t.Fatalf("expected SimpleThreadGroup, got %T", root.GetChildren()[0])
	}
	if len(tg.GetChildren()) != 2 {
		t.Fatalf("expected 2 samplers, got %d", len(tg.GetChildren()))
	}

	firstSampler, ok := tg.GetChildren()[0].(*elements.HttpSampler)
	if !ok {
		t.Fatalf("expected first child HttpSampler, got %T", tg.GetChildren()[0])
	}
	if firstSampler.Url != "https://example.com/api/orders" {
		t.Fatalf("expected resolved first sampler URL, got %q", firstSampler.Url)
	}
}

func TestTryGenerateRuleBasedDraftDoesNotTreatNarrativeStatsAsURLPath(t *testing.T) {
	_, matched, err := aipkg.TryGenerateRuleBasedDraft(aipkg.WorkloadBrief{
		URL:          "https://www.google.com/",
		Goal:         "нужно собрать мини план для тестирования главной страницы + 5 простых поисковых запросов.",
		RequestStats: "основная страница 5 рпс, поисковые запросы каждый по 1 рпс.",
	})
	if err != nil {
		t.Fatalf("TryGenerateRuleBasedDraft returned error: %v", err)
	}
	if matched {
		t.Fatal("expected ambiguous narrative stats to fall through to an AI provider")
	}
}
