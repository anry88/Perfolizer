package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"perfolizer/pkg/core"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model          string                 `json:"model"`
	Messages       []chatMessage          `json:"messages"`
	ResponseFormat map[string]string      `json:"response_format,omitempty"`
	Temperature    float64                `json:"temperature,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type chatCompletionProvider struct {
	name       string
	baseURL    string
	apiKey     string
	settings   AISettings
	httpClient *http.Client
}

type hybridProvider struct {
	cloud Provider
	local Provider
}

func newProvider(settings AISettings) Provider {
	switch settings.Provider {
	case ProviderOpenAI:
		if settings.IsCloudConfigured() {
			return newChatCompletionProvider("openai", settings.OpenAIBaseURL, settings.APIKey, settings)
		}
	case ProviderCodex:
		if settings.CodexCLIPath != "" && settings.CodexHomeDir != "" {
			return newCodexCLIProvider(settings)
		}
	case ProviderLocal:
		if settings.IsLocalConfigured() {
			return newChatCompletionProvider("local", settings.LocalBaseURL, "", settings)
		}
	case ProviderHybrid:
		var cloud Provider
		var local Provider
		if settings.IsCloudConfigured() {
			cloud = newChatCompletionProvider("openai", settings.OpenAIBaseURL, settings.APIKey, settings)
		}
		if settings.IsLocalConfigured() {
			local = newChatCompletionProvider("local", settings.LocalBaseURL, "", settings)
		}
		if cloud != nil || local != nil {
			return &hybridProvider{cloud: cloud, local: local}
		}
	}
	return nil
}

func newChatCompletionProvider(name, baseURL, apiKey string, settings AISettings) Provider {
	return &chatCompletionProvider{
		name:       name,
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		settings:   settings.Normalize(),
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

func (p *chatCompletionProvider) Name() string {
	return p.name
}

func (p *chatCompletionProvider) GenerateDraft(ctx context.Context, req GenerateDraftRequest) (PlanDraft, error) {
	brief := req.Brief
	if p.name == "openai" && p.settings.RedactSecrets {
		brief = redactBrief(brief)
	}

	payload := chatCompletionRequest{
		Model:       p.modelFor("draft"),
		Temperature: 0.1,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
		Messages: []chatMessage{
			{Role: "system", Content: draftSystemPrompt},
			{Role: "user", Content: mustJSON(struct {
				Brief WorkloadBrief `json:"brief"`
			}{Brief: brief})},
		},
	}

	raw, err := p.completeJSON(ctx, payload)
	if err != nil {
		return PlanDraft{}, err
	}

	var draft PlanDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return PlanDraft{}, fmt.Errorf("decode draft response: %w", err)
	}
	root, err := ValidateDraft(draft)
	if err != nil {
		return PlanDraft{}, fmt.Errorf("validate draft response: %w", err)
	}
	draft.Root = core.TestElementToDTO(root)
	draft.Source = p.name
	return draft, nil
}

func (p *chatCompletionProvider) SuggestPatch(ctx context.Context, req SuggestPatchRequest) (PlanPatch, error) {
	if p.name == "openai" && p.settings.RedactSecrets {
		req = redactPatchRequest(req)
	}

	payload := chatCompletionRequest{
		Model:       p.modelFor("patch"),
		Temperature: 0.1,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
		Messages: []chatMessage{
			{Role: "system", Content: patchSystemPrompt},
			{Role: "user", Content: mustJSON(req)},
		},
	}

	raw, err := p.completeJSON(ctx, payload)
	if err != nil {
		return PlanPatch{}, err
	}

	var patch PlanPatch
	if err := json.Unmarshal(raw, &patch); err != nil {
		return PlanPatch{}, fmt.Errorf("decode patch response: %w", err)
	}
	root, err := core.DTOToTestElement(req.CurrentPlan)
	if err != nil {
		return PlanPatch{}, fmt.Errorf("decode current plan: %w", err)
	}
	if _, err := ApplyPatch(root, patch); err != nil {
		return PlanPatch{}, fmt.Errorf("validate patch response: %w", err)
	}
	patch.Source = p.name
	return patch, nil
}

func (p *chatCompletionProvider) AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error) {
	payload := chatCompletionRequest{
		Model:       p.modelFor("analysis"),
		Temperature: 0.1,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
		Messages: []chatMessage{
			{Role: "system", Content: analysisSystemPrompt},
			{Role: "user", Content: mustJSON(req)},
		},
	}

	raw, err := p.completeJSON(ctx, payload)
	if err != nil {
		return RunAnalysis{}, err
	}

	var analysis RunAnalysis
	if err := json.Unmarshal(raw, &analysis); err != nil {
		return RunAnalysis{}, fmt.Errorf("decode analysis response: %w", err)
	}
	analysis.Source = p.name
	return analysis, nil
}

func (p *chatCompletionProvider) TestConnection(ctx context.Context) error {
	return probeModelsEndpoint(ctx, p.baseURL, p.apiKey, p.httpClient)
}

func (p *chatCompletionProvider) modelFor(kind string) string {
	switch kind {
	case "patch":
		if p.name == "local" {
			return p.settings.LocalModel
		}
		if p.settings.CodexModel != "" {
			return p.settings.CodexModel
		}
	case "analysis":
		if p.name == "local" {
			return p.settings.LocalModel
		}
		if p.settings.HeavyModel != "" {
			return p.settings.HeavyModel
		}
	}
	if p.name == "local" {
		return p.settings.LocalModel
	}
	return p.settings.DefaultModel
}

func (p *chatCompletionProvider) completeJSON(ctx context.Context, payload chatCompletionRequest) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint, err := url.JoinPath(p.baseURL, "chat/completions")
	if err != nil {
		return nil, fmt.Errorf("resolve chat endpoint: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send chat request: %w", err)
	}
	defer response.Body.Close()

	payloadBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("provider returned %d: %s", response.StatusCode, strings.TrimSpace(string(payloadBytes)))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(payloadBytes, &completion); err != nil {
		return nil, fmt.Errorf("decode chat response envelope: %w", err)
	}
	if completion.Error != nil && strings.TrimSpace(completion.Error.Message) != "" {
		return nil, fmt.Errorf("provider error: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("provider returned no choices")
	}

	content, err := extractChatContent(completion.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

func (p *hybridProvider) Name() string {
	return "hybrid"
}

func (p *hybridProvider) GenerateDraft(ctx context.Context, req GenerateDraftRequest) (PlanDraft, error) {
	if p.cloud != nil {
		return p.cloud.GenerateDraft(ctx, req)
	}
	if p.local != nil {
		return p.local.GenerateDraft(ctx, req)
	}
	return PlanDraft{}, ErrProviderUnavailable
}

func (p *hybridProvider) SuggestPatch(ctx context.Context, req SuggestPatchRequest) (PlanPatch, error) {
	if p.cloud != nil {
		return p.cloud.SuggestPatch(ctx, req)
	}
	if p.local != nil {
		return p.local.SuggestPatch(ctx, req)
	}
	return PlanPatch{}, ErrProviderUnavailable
}

func (p *hybridProvider) AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error) {
	if p.cloud != nil {
		return p.cloud.AnalyzeRun(ctx, req)
	}
	if p.local != nil {
		return p.local.AnalyzeRun(ctx, req)
	}
	return RunAnalysis{}, ErrProviderUnavailable
}

func (p *hybridProvider) TestConnection(ctx context.Context) error {
	var errors []string
	if p.cloud != nil {
		if err := p.cloud.TestConnection(ctx); err == nil {
			return nil
		} else {
			errors = append(errors, err.Error())
		}
	}
	if p.local != nil {
		if err := p.local.TestConnection(ctx); err == nil {
			return nil
		} else {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) == 0 {
		return ErrProviderUnavailable
	}
	return fmt.Errorf("%s", strings.Join(errors, "; "))
}

func extractChatContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("provider returned empty content")
	}

	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		return strings.TrimSpace(content), nil
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		if b.Len() > 0 {
			return strings.TrimSpace(b.String()), nil
		}
	}
	return "", fmt.Errorf("provider returned an unsupported content shape")
}

func mustJSON(value interface{}) string {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func redactBrief(brief WorkloadBrief) WorkloadBrief {
	brief.URL = redactString(brief.URL)
	brief.RequestStats = redactString(brief.RequestStats)
	brief.Goal = redactString(brief.Goal)
	brief.SelectionSummary = redactString(brief.SelectionSummary)
	for i := range brief.Requests {
		brief.Requests[i].URL = redactString(brief.Requests[i].URL)
		brief.Requests[i].Body = redactString(brief.Requests[i].Body)
	}
	return brief
}

func redactPatchRequest(req SuggestPatchRequest) SuggestPatchRequest {
	payload := mustJSON(req.CurrentPlan)
	if err := json.Unmarshal([]byte(redactString(payload)), &req.CurrentPlan); err != nil {
		// Keep the unredacted plan if the masking step produced invalid JSON.
	}
	req.Intent = redactString(req.Intent)
	return req
}

func redactString(raw string) string {
	raw = redactURLSecrets(raw)
	pattern := regexp.MustCompile(`(?i)(authorization|api[_-]?key|token|secret|password)(\s*[:=]\s*"?)([^"\s,]+)`)
	return pattern.ReplaceAllString(raw, `${1}${2}[REDACTED]`)
}

func redactURLSecrets(raw string) string {
	replaced := urlPattern.ReplaceAllStringFunc(raw, func(match string) string {
		parsed, err := url.Parse(match)
		if err != nil {
			return match
		}
		query := parsed.Query()
		changed := false
		for key := range query {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
				query.Set(key, "[REDACTED]")
				changed = true
			}
		}
		if changed {
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
		return match
	})
	return replaced
}

func probeModelsEndpoint(ctx context.Context, baseURL, apiKey string, client *http.Client) error {
	endpoint, err := url.JoinPath(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "models")
	if err != nil {
		return fmt.Errorf("resolve models endpoint: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create models request: %w", err)
	}
	if strings.TrimSpace(apiKey) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send models request: %w", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("models probe returned %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

const draftSystemPrompt = `You generate Perfolizer test-plan drafts as JSON.

Return one JSON object with this shape:
{
  "root": <Perfolizer TestElementDTO>,
  "rationale": "short explanation",
  "warnings": ["optional warning"]
}

Rules:
- Only use these element types: TestPlan, SimpleThreadGroup, RPSThreadGroup, HttpSampler, LoopController, IfController, PauseController.
- The root must be a TestPlan object with children.
- The root TestPlan may contain only thread groups as direct children.
- Put samplers and controllers inside a thread group, never directly under the TestPlan root.
- Use SimpleThreadGroup for fixed-iteration workloads.
- Use RPSThreadGroup for explicit target-RPS or timed load profiles.
- Use HttpSampler for HTTP requests.
- Use the exact persisted prop names that Perfolizer supports:
  - HttpSampler props: Url, Method, TargetRPS, Body, ExtractVars
  - SimpleThreadGroup props: Users, Iterations, HTTPRequestTimeoutMS, HTTPKeepAlive
  - RPSThreadGroup props: Users, RPS, ProfileBlocks, GracefulShutdownMS, HTTPRequestTimeoutMS, HTTPKeepAlive
  - ProfileBlocks items: RampUpMS, StepDurationMS, ProfilePercent
  - PauseController props: DurationMS
  - LoopController props: Loops
- Encode query parameters directly into HttpSampler.Url. Do not emit QueryParams.
- Do not use unsupported prop names such as URL, Threads, TargetRPS for thread groups, DurationSeconds, or RampUpSeconds.
- If the goal or request stats describe multiple request shapes, create separate samplers instead of collapsing the prose into one URL.
- Use HttpSampler.TargetRPS when request stats include per-request throughput hints.
- If an endpoint or query value is implied but not fully specified, keep the requests separate and add a warning about the assumption you made.
- Keep the draft compact and practical.
- Do not include markdown or prose outside the JSON object.`

const patchSystemPrompt = `You generate Perfolizer plan patches as JSON.

Return one JSON object with this shape:
{
  "operations": [
    {
      "type": "replace|update|add_child|remove",
      "target_id": "required for replace/update/remove",
      "parent_id": "required for add_child",
      "element": <optional TestElementDTO payload>,
      "props": <optional serializable props>,
      "name": "optional replacement name",
      "enabled": true,
      "preserve_children": true
    }
  ],
  "rationale": "short explanation",
  "warnings": ["optional warning"]
}

Rules:
- Only emit operations that can be applied safely to the provided plan.
- Prefer updating an existing thread group before replacing it.
- If you replace a thread group and want to keep the existing scenario children, set "preserve_children": true and omit children from the replacement element.
- When you emit element props, use the same exact persisted prop names as the draft contract: Url, Method, TargetRPS, Users, Iterations, RPS, ProfileBlocks, GracefulShutdownMS, HTTPRequestTimeoutMS, HTTPKeepAlive, DurationMS, Loops.
- Do not include markdown or prose outside the JSON object.`

const analysisSystemPrompt = `You analyze Perfolizer run results as JSON.

Return one JSON object with this shape:
{
  "summary": "short summary",
  "findings": ["finding"],
  "recommendations": ["recommendation"],
  "warnings": ["optional warning"]
}

Do not include markdown or prose outside the JSON object.`
