package ai

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

var (
	methodLinePattern    = regexp.MustCompile(`(?i)^\s*(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+([^\s,]+)`)
	urlPattern           = regexp.MustCompile(`https?://[^\s",]+`)
	iterationsPattern    = regexp.MustCompile(`(?i)(\d[\d\s_,.]*)\s*(million|mln|m)?\s+iterations?`)
	targetRPSPattern     = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(?:rps|рпс)(?:\s|$|[.,;:])`)
	usersPattern         = regexp.MustCompile(`(?i)(\d+)\s*(?:users|threads|vus|virtual users?)\b`)
	durationLongPattern  = regexp.MustCompile(`(?i)\bfor\s+(\d+)\s*(milliseconds?|ms|seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h)\b`)
	durationShortPattern = regexp.MustCompile(`(?i)\b(\d+)\s*(ms|s|m|h)\b`)
)

func TryGenerateRuleBasedDraft(brief WorkloadBrief) (PlanDraft, bool, error) {
	baseURL := strings.TrimSpace(brief.URL)
	if baseURL == "" {
		baseURL = extractFirstURL(brief.Goal + "\n" + brief.RequestStats)
	}
	requestSet := normalizeRequestSpecs(baseURL, brief.Requests, brief.RequestStats)
	requests := requestSet.Requests

	intent := strings.TrimSpace(strings.Join([]string{brief.Goal, brief.SelectionSummary}, "\n"))
	iterations, hasIterations := resolveIterations(intent, brief.Constraints)
	targetRPS, hasTargetRPS := resolveTargetRPS(intent, brief.Constraints)
	maxUsers := resolveUsers(intent, brief.Constraints)
	duration := resolveDuration(intent, brief.Constraints)
	hasExplicitWorkload := hasIterations || hasTargetRPS || maxUsers > 0 || duration > 0

	if len(requests) == 0 && baseURL != "" && canSeedBaseRequest(brief, requestSet, hasExplicitWorkload) {
		requests = []HTTPRequestSpec{{
			Method: "GET",
			URL:    baseURL,
		}}
	}
	if len(requests) == 0 {
		return PlanDraft{}, false, nil
	}
	if !requestSet.HasStructuredInput && !hasExplicitWorkload {
		return PlanDraft{}, false, nil
	}

	root := core.NewBaseElement("Test Plan")
	var threadGroup core.TestElement
	rationaleParts := make([]string, 0, 2)
	warnings := make([]string, 0)

	if hasTargetRPS || duration > 0 {
		if targetRPS <= 0 {
			targetRPS = 10
			warnings = append(warnings, "No explicit target RPS was found; using 10 RPS as a fallback.")
		}
		tg := elements.NewRPSThreadGroup("Traffic Profile", targetRPS)
		if maxUsers > 0 {
			tg.Users = maxUsers
		}
		if duration > 0 {
			tg.ProfileBlocks = []elements.RPSProfileBlock{{
				RampUp:         0,
				StepDuration:   duration,
				ProfilePercent: 100,
			}}
		}
		threadGroup = tg
		rationaleParts = append(rationaleParts, "Used an RPS Thread Group because the intent specifies throughput or a timed load profile.")
	} else {
		if !hasIterations {
			iterations = 1
			warnings = append(warnings, "No explicit iteration count was found; using a single iteration.")
		}
		if maxUsers <= 0 {
			maxUsers = 1
		}
		tg := elements.NewSimpleThreadGroup("Thread Group 1", maxUsers, iterations)
		threadGroup = tg
		if hasIterations {
			rationaleParts = append(rationaleParts, "Used a Simple Thread Group because the intent describes a fixed number of iterations.")
		} else {
			rationaleParts = append(rationaleParts, "Used a Simple Thread Group for a compact request-driven draft without an explicit throughput profile.")
		}
	}

	root.AddChild(threadGroup)
	for i, req := range requests {
		sampler := elements.NewHttpSampler(samplerNameForRequest(i+1, req), req.Method, req.URL)
		sampler.Body = req.Body
		threadGroup.AddChild(sampler)
	}
	rationaleParts = append(rationaleParts, fmt.Sprintf("Added %d HTTP sampler(s) from the provided URL and request stats.", len(requests)))

	return PlanDraft{
		Root:      core.TestElementToDTO(&root),
		Rationale: strings.Join(rationaleParts, " "),
		Warnings:  warnings,
		Source:    "rules",
	}, true, nil
}

func TrySuggestRuleBasedPatch(root core.TestElement, req SuggestPatchRequest) (PlanPatch, bool, error) {
	if root == nil {
		return PlanPatch{}, false, fmt.Errorf("current plan is required")
	}
	intent := strings.TrimSpace(req.Intent)
	if intent == "" {
		return PlanPatch{}, false, nil
	}

	iterations, hasIterations := resolveIterations(intent, WorkloadConstraints{})
	targetRPS, hasTargetRPS := resolveTargetRPS(intent, WorkloadConstraints{})
	maxUsers := resolveUsers(intent, WorkloadConstraints{})
	duration := resolveDuration(intent, WorkloadConstraints{})
	if !hasIterations && !hasTargetRPS && duration == 0 {
		return PlanPatch{}, false, nil
	}

	target := findPatchTarget(root, req.SelectedID)
	if target == nil {
		return PlanPatch{}, false, nil
	}

	switch current := target.(type) {
	case *elements.SimpleThreadGroup:
		if hasTargetRPS || duration > 0 {
			replacement := elements.NewRPSThreadGroup(current.Name(), maxFloat(targetRPS, 10))
			if maxUsers > 0 {
				replacement.Users = maxUsers
			} else {
				replacement.Users = current.Users
			}
			if duration > 0 {
				replacement.ProfileBlocks = []elements.RPSProfileBlock{{
					RampUp:         0,
					StepDuration:   duration,
					ProfilePercent: 100,
				}}
			}
			dto := core.TestElementToDTO(replacement)
			return PlanPatch{
				Operations: []PatchOperation{{
					Type:             "replace",
					TargetID:         current.ID(),
					Element:          &dto,
					PreserveChildren: true,
				}},
				Rationale: "Switching to an RPS Thread Group because the refinement request describes target throughput or a timed profile.",
				Source:    "rules",
			}, true, nil
		}
		if hasIterations {
			props := map[string]interface{}{
				"Iterations": iterations,
			}
			if maxUsers > 0 {
				props["Users"] = maxUsers
			}
			return PlanPatch{
				Operations: []PatchOperation{{
					Type:     "update",
					TargetID: current.ID(),
					Props:    props,
				}},
				Rationale: "Updated the Simple Thread Group to match the requested iteration-based workload.",
				Source:    "rules",
			}, true, nil
		}
	case *elements.RPSThreadGroup:
		if hasIterations && !hasTargetRPS && duration == 0 {
			replacement := elements.NewSimpleThreadGroup(current.Name(), maxInt(maxUsers, current.Users), iterations)
			dto := core.TestElementToDTO(replacement)
			return PlanPatch{
				Operations: []PatchOperation{{
					Type:             "replace",
					TargetID:         current.ID(),
					Element:          &dto,
					PreserveChildren: true,
				}},
				Rationale: "Switching to a Simple Thread Group because the refinement request describes a fixed iteration count.",
				Source:    "rules",
			}, true, nil
		}
		if hasTargetRPS || duration > 0 {
			props := map[string]interface{}{
				"RPS": maxFloat(targetRPS, current.RPS),
			}
			if maxUsers > 0 {
				props["Users"] = maxUsers
			}
			if duration > 0 {
				props["ProfileBlocks"] = []map[string]interface{}{{
					"RampUpMS":       0,
					"StepDurationMS": duration.Milliseconds(),
					"ProfilePercent": 100.0,
				}}
			}
			return PlanPatch{
				Operations: []PatchOperation{{
					Type:     "update",
					TargetID: current.ID(),
					Props:    props,
				}},
				Rationale: "Updated the RPS Thread Group to match the requested throughput profile.",
				Source:    "rules",
			}, true, nil
		}
	}

	return PlanPatch{}, false, nil
}

func ExplainSelection(root core.TestElement, selected core.TestElement, intent string) SelectionExplanation {
	if selected == nil {
		return SelectionExplanation{
			Summary: "No plan node is selected.",
		}
	}

	summary := describeSelection(selected)
	recommendation := ""
	intent = strings.TrimSpace(intent)

	switch selected.(type) {
	case *elements.SimpleThreadGroup:
		recommendation = "Simple Thread Group is the better fit for fixed user and iteration counts."
		if _, hasRPS := resolveTargetRPS(intent, WorkloadConstraints{}); hasRPS || resolveDuration(intent, WorkloadConstraints{}) > 0 {
			recommendation = "The request sounds throughput-driven. Refine the plan to an RPS Thread Group for a steadier target rate."
		}
	case *elements.RPSThreadGroup:
		recommendation = "RPS Thread Group is the better fit when you care about target throughput over a timed profile."
		if _, hasIterations := resolveIterations(intent, WorkloadConstraints{}); hasIterations {
			recommendation = "The request sounds iteration-driven. Refine the plan to a Simple Thread Group if you want a fixed number of loops."
		}
	case *elements.HttpSampler:
		recommendation = "Use this sampler for a single HTTP request shape. Add more samplers or controllers if the workflow has multiple steps."
	case *elements.LoopController:
		recommendation = "Loop Controller repeats its child nodes inside the current thread-group execution."
	case *elements.PauseController:
		recommendation = "Pause Controller adds think time between requests."
	case *elements.IfController:
		recommendation = "If Controller is useful only for conditional branches. Keep it minimal because persisted conditions are currently limited."
	default:
		if root != nil && root.ID() == selected.ID() {
			recommendation = "Top-level test plans should usually contain one or more thread groups."
		}
	}

	return SelectionExplanation{
		Summary:        summary,
		Recommendation: recommendation,
	}
}

type normalizedRequestSpecs struct {
	Requests           []HTTPRequestSpec
	HasStructuredInput bool
	HasAmbiguousRaw    bool
}

func normalizeRequestSpecs(baseURL string, seeded []HTTPRequestSpec, raw string) normalizedRequestSpecs {
	result := normalizedRequestSpecs{
		Requests: make([]HTTPRequestSpec, 0, len(seeded)),
	}
	for _, req := range seeded {
		if normalized, ok := normalizeRequestSpec(baseURL, req.Method, req.URL, req.Body); ok {
			result.Requests = append(result.Requests, normalized)
			result.HasStructuredInput = true
		}
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		method, target, ok := parseStructuredRequestLine(line)
		if !ok {
			result.HasAmbiguousRaw = true
			continue
		}

		if normalized, ok := normalizeRequestSpec(baseURL, method, target, ""); ok {
			result.Requests = append(result.Requests, normalized)
			result.HasStructuredInput = true
		} else {
			result.HasAmbiguousRaw = true
		}
	}

	if len(result.Requests) == 0 {
		return result
	}

	seen := make(map[string]bool)
	deduped := make([]HTTPRequestSpec, 0, len(result.Requests))
	for _, req := range result.Requests {
		key := req.Method + " " + req.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, req)
	}
	result.Requests = deduped
	return result
}

func parseStructuredRequestLine(line string) (string, string, bool) {
	if matches := methodLinePattern.FindStringSubmatch(line); len(matches) == 3 {
		return strings.ToUpper(matches[1]), matches[2], true
	}
	if strings.Contains(line, ",") {
		parts := strings.SplitN(line, ",", 3)
		if len(parts) >= 2 {
			candidateMethod := strings.TrimSpace(parts[0])
			candidateURL := strings.TrimSpace(parts[1])
			if isHTTPMethod(candidateMethod) && looksLikeRequestTarget(candidateURL) {
				return strings.ToUpper(candidateMethod), candidateURL, true
			}
		}
	}
	if standaloneURL := extractStandaloneURL(line); standaloneURL != "" {
		return "GET", standaloneURL, true
	}
	if target := strings.TrimSpace(line); strings.HasPrefix(target, "/") && !strings.ContainsAny(target, " \t") {
		return "GET", target, true
	}
	return "", "", false
}

func looksLikeRequestTarget(raw string) bool {
	target := strings.TrimSpace(raw)
	if target == "" {
		return false
	}
	if strings.HasPrefix(target, "/") && !strings.ContainsAny(target, " \t") {
		return true
	}
	return extractStandaloneURL(target) != ""
}

func extractStandaloneURL(line string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(line, ",;"))
	match := extractFirstURL(trimmed)
	if match == "" {
		return ""
	}
	if match != trimmed {
		return ""
	}
	return match
}

func canSeedBaseRequest(brief WorkloadBrief, requestSet normalizedRequestSpecs, hasExplicitWorkload bool) bool {
	if strings.TrimSpace(brief.URL) == "" {
		return false
	}
	if requestSet.HasStructuredInput || requestSet.HasAmbiguousRaw {
		return false
	}
	if hasExplicitWorkload {
		return true
	}
	return strings.TrimSpace(brief.Goal) == "" &&
		strings.TrimSpace(brief.SelectionSummary) == "" &&
		strings.TrimSpace(brief.RequestStats) == ""
}

func normalizeRequestSpec(baseURL, method, rawURL, body string) (HTTPRequestSpec, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}
	if !isHTTPMethod(method) {
		return HTTPRequestSpec{}, false
	}

	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return HTTPRequestSpec{}, false
	}
	if !strings.Contains(rawURL, "://") && !strings.HasPrefix(rawURL, "/") {
		if extracted := extractFirstURL(rawURL); extracted != "" {
			rawURL = extracted
		}
	}
	if !strings.Contains(rawURL, "://") {
		if strings.TrimSpace(baseURL) == "" {
			return HTTPRequestSpec{}, false
		}
		base, err := url.Parse(strings.TrimSpace(baseURL))
		if err != nil {
			return HTTPRequestSpec{}, false
		}
		ref, err := url.Parse(rawURL)
		if err != nil {
			return HTTPRequestSpec{}, false
		}
		rawURL = base.ResolveReference(ref).String()
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return HTTPRequestSpec{}, false
	}

	return HTTPRequestSpec{
		Method: method,
		URL:    rawURL,
		Body:   body,
	}, true
}

func extractFirstURL(raw string) string {
	match := urlPattern.FindString(raw)
	return strings.TrimSpace(match)
}

func resolveIterations(intent string, constraints WorkloadConstraints) (int, bool) {
	if constraints.Iterations > 0 {
		return constraints.Iterations, true
	}
	matches := iterationsPattern.FindStringSubmatch(intent)
	if len(matches) != 3 {
		return 0, false
	}
	value, ok := parseFlexibleInt(matches[1])
	if !ok {
		return 0, false
	}
	suffix := strings.ToLower(strings.TrimSpace(matches[2]))
	if suffix == "million" || suffix == "mln" || suffix == "m" {
		value *= 1_000_000
	}
	return value, true
}

func resolveTargetRPS(intent string, constraints WorkloadConstraints) (float64, bool) {
	if constraints.TargetRPS > 0 {
		return constraints.TargetRPS, true
	}
	matches := targetRPSPattern.FindStringSubmatch(intent)
	if len(matches) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(matches[1], ",", "."), 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func resolveUsers(intent string, constraints WorkloadConstraints) int {
	if constraints.MaxUsers > 0 {
		return constraints.MaxUsers
	}
	matches := usersPattern.FindStringSubmatch(intent)
	if len(matches) != 2 {
		return 0
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func resolveDuration(intent string, constraints WorkloadConstraints) time.Duration {
	if constraints.DurationMS > 0 {
		return time.Duration(constraints.DurationMS) * time.Millisecond
	}
	if matches := durationLongPattern.FindStringSubmatch(intent); len(matches) == 3 {
		if duration, ok := parseDurationValue(matches[1], matches[2]); ok {
			return duration
		}
	}
	if matches := durationShortPattern.FindStringSubmatch(intent); len(matches) == 3 {
		if duration, ok := parseDurationValue(matches[1], matches[2]); ok {
			return duration
		}
	}
	return 0
}

func parseDurationValue(valueText, unitText string) (time.Duration, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(valueText))
	if err != nil || value <= 0 {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(unitText)) {
	case "ms", "millisecond", "milliseconds":
		return time.Duration(value) * time.Millisecond, true
	case "s", "sec", "secs", "second", "seconds":
		return time.Duration(value) * time.Second, true
	case "m", "min", "mins", "minute", "minutes":
		return time.Duration(value) * time.Minute, true
	case "h", "hr", "hrs", "hour", "hours":
		return time.Duration(value) * time.Hour, true
	default:
		return 0, false
	}
}

func parseFlexibleInt(raw string) (int, bool) {
	clean := strings.NewReplacer(" ", "", ",", "", "_", "", ".", "").Replace(strings.TrimSpace(raw))
	value, err := strconv.Atoi(clean)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func isHTTPMethod(raw string) bool {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func samplerNameForRequest(index int, req HTTPRequestSpec) string {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	u, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Sprintf("%s Request %d", method, index)
	}
	name := path.Clean(u.Path)
	if name == "." || name == "/" {
		name = u.Host
	}
	name = strings.Trim(name, "/")
	if name == "" {
		name = u.Host
	}
	return fmt.Sprintf("%s %s", method, name)
}

func findPatchTarget(root core.TestElement, selectedID string) core.TestElement {
	if selectedID != "" {
		if selected := findElementByID(root, selectedID); selected != nil {
			if _, ok := selected.(core.ThreadGroup); ok {
				return selected
			}
		}
	}
	for _, child := range root.GetChildren() {
		if _, ok := child.(core.ThreadGroup); ok {
			return child
		}
	}
	return nil
}

func findElementByID(root core.TestElement, id string) core.TestElement {
	if root == nil || id == "" {
		return nil
	}
	if root.ID() == id {
		return root
	}
	for _, child := range root.GetChildren() {
		if found := findElementByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

func describeSelection(selected core.TestElement) string {
	switch current := selected.(type) {
	case *elements.SimpleThreadGroup:
		return fmt.Sprintf("Simple Thread Group %q runs %d user(s) for %d iteration(s).", current.Name(), current.Users, current.Iterations)
	case *elements.RPSThreadGroup:
		return fmt.Sprintf("RPS Thread Group %q targets %.2f RPS with up to %d user(s).", current.Name(), current.RPS, current.Users)
	case *elements.HttpSampler:
		return fmt.Sprintf("HTTP Sampler %q sends %s %s.", current.Name(), strings.ToUpper(current.Method), current.Url)
	case *elements.LoopController:
		return fmt.Sprintf("Loop Controller %q repeats its child nodes %d time(s).", current.Name(), current.Loops)
	case *elements.PauseController:
		return fmt.Sprintf("Pause Controller %q waits for %s.", current.Name(), current.Duration)
	case *elements.IfController:
		return fmt.Sprintf("If Controller %q conditionally runs its child nodes.", current.Name())
	default:
		return fmt.Sprintf("Selected node: %s.", strings.TrimSpace(selected.Name()))
	}
}

func maxInt(values ...int) int {
	maxValue := 0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func maxFloat(values ...float64) float64 {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func sortedKeys(input map[string]core.Metric) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
