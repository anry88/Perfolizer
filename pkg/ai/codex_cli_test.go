package ai

import "testing"

func TestExtractCodexCallbackPrefix(t *testing.T) {
	authURL := "https://auth.openai.com/oauth/authorize?response_type=code&client_id=test&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback&scope=openid"

	got, err := extractCodexCallbackPrefix(authURL)
	if err != nil {
		t.Fatalf("extractCodexCallbackPrefix returned error: %v", err)
	}
	if got != "http://localhost:1455/auth/callback" {
		t.Fatalf("expected callback prefix %q, got %q", "http://localhost:1455/auth/callback", got)
	}
}

func TestAISettingsIsConfiguredForCodexRequiresAuthState(t *testing.T) {
	settings := DefaultSettings()
	settings.Provider = ProviderCodex
	settings.CodexAuthOK = false
	if settings.IsConfigured() {
		t.Fatal("expected Codex provider without auth to be treated as unconfigured")
	}

	settings.CodexAuthOK = true
	if !settings.IsConfigured() {
		t.Fatal("expected Codex provider with auth to be configured")
	}
}
