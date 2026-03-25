package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"perfolizer/pkg/core"
)

var codexAuthURLPattern = regexp.MustCompile(`https://auth\.openai\.com/oauth/authorize\?[^\s]+`)

type CodexAuthSession struct {
	AuthURL           string
	CallbackURLPrefix string

	cmd    *exec.Cmd
	doneCh chan struct{}

	mu     sync.Mutex
	result error
	output strings.Builder
}

type codexCLIProvider struct {
	settings AISettings
}

func newCodexCLIProvider(settings AISettings) Provider {
	return &codexCLIProvider{settings: settings.Normalize()}
}

func (p *codexCLIProvider) Name() string {
	return "codex"
}

func (p *codexCLIProvider) GenerateDraft(ctx context.Context, req GenerateDraftRequest) (PlanDraft, error) {
	var envelope struct {
		RootJSON  string   `json:"root_json"`
		Rationale string   `json:"rationale"`
		Warnings  []string `json:"warnings"`
	}

	raw, err := p.runStructuredPrompt(ctx, p.settings.CodexModel, buildCodexDraftPrompt(req), codexDraftSchema())
	if err != nil {
		return PlanDraft{}, err
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return PlanDraft{}, fmt.Errorf("decode Codex draft envelope: %w", err)
	}

	var dto core.TestElementDTO
	if err := json.Unmarshal([]byte(envelope.RootJSON), &dto); err != nil {
		return PlanDraft{}, fmt.Errorf("decode Codex root JSON: %w", err)
	}
	draft := PlanDraft{
		Root:      dto,
		Rationale: envelope.Rationale,
		Warnings:  envelope.Warnings,
		Source:    "codex",
	}
	root, err := ValidateDraft(draft)
	if err != nil {
		return PlanDraft{}, err
	}
	draft.Root = core.TestElementToDTO(root)
	return draft, nil
}

func (p *codexCLIProvider) SuggestPatch(ctx context.Context, req SuggestPatchRequest) (PlanPatch, error) {
	var envelope struct {
		PatchJSON string   `json:"patch_json"`
		Rationale string   `json:"rationale"`
		Warnings  []string `json:"warnings"`
	}

	raw, err := p.runStructuredPrompt(ctx, p.settings.CodexModel, buildCodexPatchPrompt(req), codexPatchSchema())
	if err != nil {
		return PlanPatch{}, err
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return PlanPatch{}, fmt.Errorf("decode Codex patch envelope: %w", err)
	}

	var patch PlanPatch
	if err := json.Unmarshal([]byte(envelope.PatchJSON), &patch); err != nil {
		return PlanPatch{}, fmt.Errorf("decode Codex patch JSON: %w", err)
	}
	patch.Rationale = firstNonEmpty(patch.Rationale, envelope.Rationale)
	if len(patch.Warnings) == 0 {
		patch.Warnings = envelope.Warnings
	}
	patch.Source = "codex"

	root, err := core.DTOToTestElement(req.CurrentPlan)
	if err != nil {
		return PlanPatch{}, fmt.Errorf("decode current plan: %w", err)
	}
	if _, err := ApplyPatch(root, patch); err != nil {
		return PlanPatch{}, err
	}
	return patch, nil
}

func (p *codexCLIProvider) AnalyzeRun(ctx context.Context, req AnalyzeRunRequest) (RunAnalysis, error) {
	raw, err := p.runStructuredPrompt(ctx, p.settings.HeavyModel, buildCodexAnalysisPrompt(req), codexAnalysisSchema())
	if err != nil {
		return RunAnalysis{}, err
	}
	var analysis RunAnalysis
	if err := json.Unmarshal(raw, &analysis); err != nil {
		return RunAnalysis{}, fmt.Errorf("decode Codex analysis: %w", err)
	}
	analysis.Source = "codex"
	return analysis, nil
}

func (p *codexCLIProvider) TestConnection(ctx context.Context) error {
	return testCodexConnection(ctx, p.settings)
}

func StartCodexLogin(settings AISettings) (*CodexAuthSession, error) {
	settings = settings.Normalize()
	if err := ensureCodexHome(settings); err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	cmd := exec.Command(settings.CodexCLIPath, "login")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+settings.CodexHomeDir)
	cmd.Stdout = pw
	cmd.Stderr = pw

	session := &CodexAuthSession{
		cmd:    cmd,
		doneCh: make(chan struct{}),
	}

	authReady := make(chan struct{})
	authErr := make(chan error, 1)
	go session.captureLoginOutput(pr, authReady, authErr)

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return nil, fmt.Errorf("start Codex login: %w", err)
	}

	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		session.finish(err)
	}()

	select {
	case <-authReady:
		return session, nil
	case err := <-authErr:
		_ = session.Cancel()
		return nil, err
	case <-time.After(15 * time.Second):
		_ = session.Cancel()
		return nil, fmt.Errorf("timed out waiting for Codex auth URL")
	}
}

func (s *CodexAuthSession) captureLoginOutput(reader io.Reader, authReady chan<- struct{}, authErr chan<- error) {
	scanner := bufio.NewScanner(reader)
	readySent := false
	for scanner.Scan() {
		line := scanner.Text()
		s.appendOutput(line)
		if readySent {
			continue
		}
		authURL := extractCodexAuthURL(line)
		if authURL == "" {
			continue
		}
		callbackPrefix, err := extractCodexCallbackPrefix(authURL)
		if err != nil {
			authErr <- err
			return
		}
		s.mu.Lock()
		s.AuthURL = authURL
		s.CallbackURLPrefix = callbackPrefix
		s.mu.Unlock()
		readySent = true
		close(authReady)
	}
	if err := scanner.Err(); err != nil {
		authErr <- fmt.Errorf("read Codex login output: %w", err)
		return
	}
	if !readySent {
		authErr <- fmt.Errorf("Codex login did not emit an auth URL")
	}
}

func (s *CodexAuthSession) Complete(ctx context.Context, callbackURL string) error {
	if done, result := s.finished(); done {
		return result
	}

	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return fmt.Errorf("callback URL is required")
	}

	s.mu.Lock()
	expectedPrefix := s.CallbackURLPrefix
	s.mu.Unlock()
	if expectedPrefix == "" {
		return fmt.Errorf("Codex auth session is missing its callback URL prefix")
	}
	if !strings.HasPrefix(callbackURL, expectedPrefix) {
		return fmt.Errorf("callback URL must start with %s", expectedPrefix)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		return fmt.Errorf("build callback request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		if done, result := s.finished(); done {
			return result
		}
		return fmt.Errorf("send callback request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode >= 400 {
		return fmt.Errorf("Codex callback returned %d", response.StatusCode)
	}
	return s.wait(30 * time.Second)
}

func (s *CodexAuthSession) Cancel() error {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	if done, _ := s.finished(); done {
		return nil
	}
	if err := s.cmd.Process.Kill(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "finished") {
		return err
	}
	return nil
}

func (s *CodexAuthSession) appendOutput(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.output.Len() > 0 {
		s.output.WriteByte('\n')
	}
	s.output.WriteString(line)
}

func (s *CodexAuthSession) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = err
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
}

func (s *CodexAuthSession) finished() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.doneCh:
		return true, s.result
	default:
		return false, nil
	}
}

func (s *CodexAuthSession) wait(timeout time.Duration) error {
	select {
	case <-s.doneCh:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.result
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for Codex login to finish")
	}
}

func (s *CodexAuthSession) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String()
}

func testCodexConnection(ctx context.Context, settings AISettings) error {
	settings = settings.Normalize()
	if err := ensureCodexHome(settings); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, settings.CodexCLIPath, "login", "status")
	command.Env = append(os.Environ(), "CODEX_HOME="+settings.CodexHomeDir)
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return fmt.Errorf("Codex login status failed: %s", trimmed)
	}
	return nil
}

func ensureCodexHome(settings AISettings) error {
	settings = settings.Normalize()
	if settings.CodexCLIPath == "" {
		return fmt.Errorf("Codex CLI path is required")
	}
	if settings.CodexHomeDir == "" {
		return fmt.Errorf("Codex home directory is required")
	}
	if err := os.MkdirAll(settings.CodexHomeDir, 0o755); err != nil {
		return fmt.Errorf("create Codex home: %w", err)
	}
	configPath := filepath.Join(settings.CodexHomeDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("model = %q\n", settings.CodexModel)), 0o644); err != nil {
		return fmt.Errorf("write Codex config: %w", err)
	}
	return nil
}

func extractCodexAuthURL(line string) string {
	return codexAuthURLPattern.FindString(line)
}

func extractCodexCallbackPrefix(authURL string) (string, error) {
	parsed, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("parse Codex auth URL: %w", err)
	}
	redirectURI := strings.TrimSpace(parsed.Query().Get("redirect_uri"))
	if redirectURI == "" {
		return "", fmt.Errorf("Codex auth URL does not contain redirect_uri")
	}
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("parse Codex redirect URI: %w", err)
	}
	return redirect.String(), nil
}

func (p *codexCLIProvider) runStructuredPrompt(ctx context.Context, model string, prompt string, schema map[string]interface{}) ([]byte, error) {
	if err := ensureCodexHome(p.settings); err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "perfolizer-codex-exec-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	schemaPath := filepath.Join(tempDir, "schema.json")
	schemaBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Codex output schema: %w", err)
	}
	if err := os.WriteFile(schemaPath, schemaBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write Codex output schema: %w", err)
	}

	outputPath := filepath.Join(tempDir, "last-message.json")
	command := exec.CommandContext(
		ctx,
		p.settings.CodexCLIPath,
		"exec",
		"-",
		"-m", firstNonEmpty(strings.TrimSpace(model), p.settings.CodexModel),
		"-s", "read-only",
		"--skip-git-repo-check",
		"--output-schema", schemaPath,
		"-o", outputPath,
	)
	command.Env = append(os.Environ(), "CODEX_HOME="+p.settings.CodexHomeDir)
	command.Stdin = strings.NewReader(prompt)

	var stderr bytes.Buffer
	command.Stdout = io.Discard
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("Codex exec failed: %s", message)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read Codex output: %w", err)
	}
	return payload, nil
}

func buildCodexDraftPrompt(req GenerateDraftRequest) string {
	return strings.TrimSpace(draftSystemPrompt + `

Return JSON matching the supplied schema. The field "root_json" must contain a JSON-encoded TestElementDTO object.

Input:
` + mustJSON(req))
}

func buildCodexPatchPrompt(req SuggestPatchRequest) string {
	return strings.TrimSpace(patchSystemPrompt + `

Return JSON matching the supplied schema. The field "patch_json" must contain a JSON-encoded PlanPatch object.

Input:
` + mustJSON(req))
}

func buildCodexAnalysisPrompt(req AnalyzeRunRequest) string {
	return strings.TrimSpace(analysisSystemPrompt + `

Input:
` + mustJSON(req))
}

func codexDraftSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"root_json", "rationale", "warnings"},
		"properties": map[string]interface{}{
			"root_json": map[string]interface{}{"type": "string"},
			"rationale": map[string]interface{}{"type": "string"},
			"warnings": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
	}
}

func codexPatchSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"patch_json", "rationale", "warnings"},
		"properties": map[string]interface{}{
			"patch_json": map[string]interface{}{"type": "string"},
			"rationale":  map[string]interface{}{"type": "string"},
			"warnings": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
	}
}

func codexAnalysisSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary", "findings", "recommendations", "warnings"},
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{"type": "string"},
			"findings": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"recommendations": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			"warnings": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
