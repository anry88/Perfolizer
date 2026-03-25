package ui

import (
	"testing"

	aipkg "perfolizer/pkg/ai"
	"perfolizer/pkg/core"
)

func TestSettingsSectionsIncludeAIAfterGeneral(t *testing.T) {
	got := settingsSections()
	if len(got) < 2 {
		t.Fatalf("expected at least two settings sections, got %v", got)
	}
	if got[0] != "General" || got[1] != "AI" {
		t.Fatalf("expected settings order [General AI ...], got %v", got)
	}
}

func TestShouldOpenAIPanelOnStartup(t *testing.T) {
	if shouldOpenAIPanelOnStartup(aipkg.DefaultSettings()) {
		t.Fatal("expected disabled default settings to keep AI panel closed")
	}

	settings := aipkg.DefaultSettings()
	settings.Enabled = true
	settings.Provider = aipkg.ProviderOpenAI
	settings.APIKey = "test-key"
	if !shouldOpenAIPanelOnStartup(settings) {
		t.Fatal("expected configured enabled settings to open AI panel")
	}
}

func TestScrubAISettingsForPreferencesRemovesAPIKey(t *testing.T) {
	settings := aipkg.DefaultSettings()
	settings.APIKey = "super-secret"

	scrubbed := scrubAISettingsForPreferences(settings)
	if scrubbed.APIKey != "" {
		t.Fatal("expected API key to be removed before writing preferences")
	}
	if scrubbed.Provider != settings.Provider {
		t.Fatal("expected other AI settings fields to remain intact")
	}
}

func TestNextGeneratedPlanNameAddsSuffixInsideProject(t *testing.T) {
	project := core.NewProject("Demo")
	root := core.NewBaseElement("Checkout API")
	project.AddPlan("Checkout API", &root)

	got := nextGeneratedPlanName(project, "Checkout API")
	if got != "Checkout API (2)" {
		t.Fatalf("expected duplicate AI plan name to get a suffix, got %q", got)
	}
}
