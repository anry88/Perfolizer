package ai

import (
	"os"
	"path/filepath"
	"strings"

	"perfolizer/pkg/core"
)

type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderLocal  ProviderType = "local"
	ProviderHybrid ProviderType = "hybrid"
	ProviderCodex  ProviderType = "codex"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-5.4-mini"
	defaultHeavyModel    = "gpt-5.4"
	defaultCodexModel    = "gpt-5.3-codex"
	defaultLocalModel    = "gpt-oss-20b"
	defaultCodexCLIPath  = "codex"
)

type AISettings struct {
	Enabled       bool         `json:"enabled"`
	Provider      ProviderType `json:"provider"`
	DefaultModel  string       `json:"default_model"`
	HeavyModel    string       `json:"heavy_model"`
	CodexModel    string       `json:"codex_model"`
	LocalModel    string       `json:"local_model"`
	OpenAIBaseURL string       `json:"openai_base_url,omitempty"`
	LocalBaseURL  string       `json:"local_base_url,omitempty"`
	APIKey        string       `json:"api_key,omitempty"`
	CodexCLIPath  string       `json:"codex_cli_path,omitempty"`
	CodexHomeDir  string       `json:"codex_home_dir,omitempty"`
	CodexAuthOK   bool         `json:"codex_auth_ok,omitempty"`
	RedactSecrets bool         `json:"redact_secrets"`
	RequireReview bool         `json:"require_review"`
}

func DefaultSettings() AISettings {
	return AISettings{
		Enabled:       false,
		Provider:      ProviderHybrid,
		DefaultModel:  defaultOpenAIModel,
		HeavyModel:    defaultHeavyModel,
		CodexModel:    defaultCodexModel,
		LocalModel:    defaultLocalModel,
		OpenAIBaseURL: defaultOpenAIBaseURL,
		CodexCLIPath:  defaultCodexCLIPath,
		CodexHomeDir:  defaultCodexHomeDir(),
		RedactSecrets: true,
		RequireReview: true,
	}
}

func (s AISettings) Normalize() AISettings {
	out := DefaultSettings()
	out.Enabled = s.Enabled
	out.Provider = ProviderType(strings.ToLower(strings.TrimSpace(string(s.Provider))))
	if out.Provider == "" {
		out.Provider = ProviderHybrid
	}
	if model := strings.TrimSpace(s.DefaultModel); model != "" {
		out.DefaultModel = model
	}
	if model := strings.TrimSpace(s.HeavyModel); model != "" {
		out.HeavyModel = model
	}
	if model := strings.TrimSpace(s.CodexModel); model != "" {
		out.CodexModel = model
	}
	if model := strings.TrimSpace(s.LocalModel); model != "" {
		out.LocalModel = model
	}
	if cliPath := strings.TrimSpace(s.CodexCLIPath); cliPath != "" {
		out.CodexCLIPath = cliPath
	}
	if baseURL := strings.TrimSpace(s.OpenAIBaseURL); baseURL != "" {
		out.OpenAIBaseURL = strings.TrimRight(baseURL, "/")
	}
	out.LocalBaseURL = strings.TrimRight(strings.TrimSpace(s.LocalBaseURL), "/")
	out.APIKey = strings.TrimSpace(s.APIKey)
	if codexHome := strings.TrimSpace(s.CodexHomeDir); codexHome != "" {
		out.CodexHomeDir = codexHome
	}
	out.CodexAuthOK = s.CodexAuthOK
	out.RedactSecrets = s.RedactSecrets
	out.RequireReview = s.RequireReview
	return out
}

func (s AISettings) IsCloudConfigured() bool {
	normalized := s.Normalize()
	return normalized.OpenAIBaseURL != "" && normalized.APIKey != ""
}

func (s AISettings) IsLocalConfigured() bool {
	normalized := s.Normalize()
	return normalized.LocalBaseURL != ""
}

func (s AISettings) IsCodexConfigured() bool {
	normalized := s.Normalize()
	return normalized.CodexCLIPath != "" && normalized.CodexHomeDir != "" && normalized.CodexAuthOK
}

func (s AISettings) IsConfigured() bool {
	normalized := s.Normalize()
	switch normalized.Provider {
	case ProviderOpenAI:
		return normalized.IsCloudConfigured()
	case ProviderLocal:
		return normalized.IsLocalConfigured()
	case ProviderHybrid:
		return normalized.IsCloudConfigured() || normalized.IsLocalConfigured()
	case ProviderCodex:
		return normalized.IsCodexConfigured()
	default:
		return false
	}
}

func (s AISettings) Summary() string {
	normalized := s.Normalize()
	if !normalized.Enabled {
		return "AI disabled"
	}
	if !normalized.IsConfigured() {
		return "AI not configured"
	}
	switch normalized.Provider {
	case ProviderOpenAI:
		return "AI enabled: OpenAI"
	case ProviderLocal:
		return "AI enabled: Local"
	case ProviderHybrid:
		return "AI enabled: Hybrid"
	case ProviderCodex:
		return "AI enabled: Codex"
	default:
		return "AI enabled"
	}
}

func defaultCodexHomeDir() string {
	if userConfigDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(userConfigDir) != "" {
		return filepath.Join(userConfigDir, "perfolizer", "codex")
	}
	return filepath.Join(os.TempDir(), "perfolizer-codex")
}

type WorkloadConstraints struct {
	Iterations int     `json:"iterations,omitempty"`
	TargetRPS  float64 `json:"target_rps,omitempty"`
	MaxUsers   int     `json:"max_users,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
}

type HTTPRequestSpec struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   string `json:"body,omitempty"`
}

type WorkloadBrief struct {
	URL              string              `json:"url,omitempty"`
	RequestStats     string              `json:"request_stats,omitempty"`
	Goal             string              `json:"goal,omitempty"`
	SelectionSummary string              `json:"selection_summary,omitempty"`
	Constraints      WorkloadConstraints `json:"constraints,omitempty"`
	Requests         []HTTPRequestSpec   `json:"requests,omitempty"`
}

type GenerateDraftRequest struct {
	Brief WorkloadBrief `json:"brief"`
}

type SuggestPatchRequest struct {
	Intent      string              `json:"intent"`
	CurrentPlan core.TestElementDTO `json:"current_plan"`
	SelectedID  string              `json:"selected_id,omitempty"`
	Parameters  []core.Parameter    `json:"parameters,omitempty"`
}

type AnalyzeRunRequest struct {
	Intent  string                 `json:"intent,omitempty"`
	Metrics map[string]core.Metric `json:"metrics,omitempty"`
	Notes   string                 `json:"notes,omitempty"`
}

type PlanDraft struct {
	Root      core.TestElementDTO `json:"root"`
	Rationale string              `json:"rationale,omitempty"`
	Warnings  []string            `json:"warnings,omitempty"`
	Source    string              `json:"source,omitempty"`
}

type PatchOperation struct {
	Type             string                 `json:"type"`
	TargetID         string                 `json:"target_id,omitempty"`
	ParentID         string                 `json:"parent_id,omitempty"`
	Element          *core.TestElementDTO   `json:"element,omitempty"`
	Props            map[string]interface{} `json:"props,omitempty"`
	Name             string                 `json:"name,omitempty"`
	Enabled          *bool                  `json:"enabled,omitempty"`
	PreserveChildren bool                   `json:"preserve_children,omitempty"`
}

type PlanPatch struct {
	Operations []PatchOperation `json:"operations"`
	Rationale  string           `json:"rationale,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
	Source     string           `json:"source,omitempty"`
}

type RunAnalysis struct {
	Findings        []string `json:"findings,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	Source          string   `json:"source,omitempty"`
}

type SelectionExplanation struct {
	Summary        string   `json:"summary,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}
