package ai

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"perfolizer/pkg/core"
)

func ValidateDraft(draft PlanDraft) (core.TestElement, error) {
	dto, err := normalizeDTO(draft.Root)
	if err != nil {
		return nil, fmt.Errorf("normalize draft root: %w", err)
	}
	root, err := core.DTOToTestElement(dto)
	if err != nil {
		return nil, fmt.Errorf("convert draft root: %w", err)
	}
	if err := core.ValidateTestPlan(root); err != nil {
		return nil, err
	}
	return root, nil
}

func ApplyPatch(root core.TestElement, patch PlanPatch) (core.TestElement, error) {
	if root == nil {
		return nil, fmt.Errorf("test plan is required")
	}
	if len(patch.Operations) == 0 {
		return nil, fmt.Errorf("patch has no operations")
	}

	dto, err := normalizeDTO(core.TestElementToDTO(root))
	if err != nil {
		return nil, fmt.Errorf("normalize cloned plan: %w", err)
	}
	currentRoot, err := core.DTOToTestElement(dto)
	if err != nil {
		return nil, fmt.Errorf("clone plan: %w", err)
	}

	for _, op := range patch.Operations {
		nextRoot, applyErr := applyPatchOperation(currentRoot, op)
		if applyErr != nil {
			return nil, applyErr
		}
		currentRoot = nextRoot
	}

	if err := core.ValidateTestPlan(currentRoot); err != nil {
		return nil, err
	}
	return currentRoot, nil
}

func SummarizePatch(patch PlanPatch) []string {
	lines := make([]string, 0, len(patch.Operations)+1)
	if patch.Rationale != "" {
		lines = append(lines, patch.Rationale)
	}
	for _, op := range patch.Operations {
		switch op.Type {
		case "replace":
			lines = append(lines, fmt.Sprintf("Replace element %q.", op.TargetID))
		case "update":
			lines = append(lines, fmt.Sprintf("Update element %q.", op.TargetID))
		case "add_child":
			lines = append(lines, fmt.Sprintf("Add a child under %q.", op.ParentID))
		case "remove":
			lines = append(lines, fmt.Sprintf("Remove element %q.", op.TargetID))
		default:
			lines = append(lines, fmt.Sprintf("Operation %q on %q.", op.Type, op.TargetID))
		}
	}
	return lines
}

func applyPatchOperation(root core.TestElement, op PatchOperation) (core.TestElement, error) {
	switch op.Type {
	case "replace":
		return applyReplace(root, op)
	case "update":
		return applyUpdate(root, op)
	case "add_child":
		return applyAddChild(root, op)
	case "remove":
		return applyRemove(root, op)
	default:
		return nil, fmt.Errorf("unsupported patch operation: %s", op.Type)
	}
}

func applyReplace(root core.TestElement, op PatchOperation) (core.TestElement, error) {
	if op.Element == nil {
		return nil, fmt.Errorf("replace operation requires an element payload")
	}
	target, parent, index := findElementWithParent(root, op.TargetID, nil)
	if target == nil {
		return nil, fmt.Errorf("replace target %q was not found", op.TargetID)
	}

	dto := *op.Element
	if dto.ID == "" {
		dto.ID = target.ID()
	}
	if dto.Name == "" {
		dto.Name = target.Name()
	}
	if dto.Enabled == nil {
		enabled := target.Enabled()
		dto.Enabled = &enabled
	}
	if op.PreserveChildren && len(dto.Children) == 0 {
		children := target.GetChildren()
		dto.Children = make([]core.TestElementDTO, 0, len(children))
		for _, child := range children {
			dto.Children = append(dto.Children, core.TestElementToDTO(child))
		}
	}

	normalizedDTO, err := normalizeDTO(dto)
	if err != nil {
		return nil, fmt.Errorf("normalize replacement for %q: %w", op.TargetID, err)
	}
	replacement, err := core.DTOToTestElement(normalizedDTO)
	if err != nil {
		return nil, fmt.Errorf("replace target %q: %w", op.TargetID, err)
	}
	if parent == nil {
		return replacement, nil
	}

	children := parent.GetChildren()
	children[index] = replacement
	return root, nil
}

func applyUpdate(root core.TestElement, op PatchOperation) (core.TestElement, error) {
	target, parent, index := findElementWithParent(root, op.TargetID, nil)
	if target == nil {
		return nil, fmt.Errorf("update target %q was not found", op.TargetID)
	}

	dto := core.TestElementToDTO(target)
	if op.Name != "" {
		dto.Name = op.Name
	}
	if op.Enabled != nil {
		dto.Enabled = op.Enabled
	}
	if len(op.Props) > 0 {
		if dto.Props == nil {
			dto.Props = make(map[string]interface{}, len(op.Props))
		}
		for key, value := range op.Props {
			dto.Props[key] = value
		}
	}

	normalizedDTO, err := normalizeDTO(dto)
	if err != nil {
		return nil, fmt.Errorf("normalize update for %q: %w", op.TargetID, err)
	}
	updated, err := core.DTOToTestElement(normalizedDTO)
	if err != nil {
		return nil, fmt.Errorf("update target %q: %w", op.TargetID, err)
	}
	if parent == nil {
		return updated, nil
	}

	children := parent.GetChildren()
	children[index] = updated
	return root, nil
}

func applyAddChild(root core.TestElement, op PatchOperation) (core.TestElement, error) {
	if op.Element == nil {
		return nil, fmt.Errorf("add_child operation requires an element payload")
	}
	parent, _, _ := findElementWithParent(root, op.ParentID, nil)
	if parent == nil {
		return nil, fmt.Errorf("parent %q was not found", op.ParentID)
	}

	dto, err := normalizeDTO(*op.Element)
	if err != nil {
		return nil, fmt.Errorf("normalize child for %q: %w", op.ParentID, err)
	}
	child, err := core.DTOToTestElement(dto)
	if err != nil {
		return nil, fmt.Errorf("add child under %q: %w", op.ParentID, err)
	}
	parent.AddChild(child)
	return root, nil
}

func applyRemove(root core.TestElement, op PatchOperation) (core.TestElement, error) {
	target, parent, _ := findElementWithParent(root, op.TargetID, nil)
	if target == nil {
		return nil, fmt.Errorf("remove target %q was not found", op.TargetID)
	}
	if parent == nil {
		return nil, fmt.Errorf("cannot remove the test plan root")
	}
	parent.RemoveChild(target.ID())
	return root, nil
}

func findElementWithParent(current core.TestElement, targetID string, parent core.TestElement) (core.TestElement, core.TestElement, int) {
	if current == nil || targetID == "" {
		return nil, nil, -1
	}
	if current.ID() == targetID {
		return current, parent, -1
	}

	children := current.GetChildren()
	for i, child := range children {
		if child.ID() == targetID {
			return child, current, i
		}
		if found, foundParent, index := findElementWithParent(child, targetID, current); found != nil {
			return found, foundParent, index
		}
	}
	return nil, nil, -1
}

func normalizeDTO(dto core.TestElementDTO) (core.TestElementDTO, error) {
	payload, err := json.Marshal(dto)
	if err != nil {
		return core.TestElementDTO{}, err
	}
	var normalized core.TestElementDTO
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return core.TestElementDTO{}, err
	}
	canonicalizeDTO(&normalized)
	return normalized, nil
}

func canonicalizeDTO(dto *core.TestElementDTO) {
	if dto == nil {
		return
	}
	dto.Props = canonicalizeProps(dto.Type, dto.Props)
	for i := range dto.Children {
		canonicalizeDTO(&dto.Children[i])
	}
}

func canonicalizeProps(elementType string, props map[string]interface{}) map[string]interface{} {
	if props == nil {
		props = make(map[string]interface{})
	}
	switch elementType {
	case "HttpSampler":
		return canonicalizeHTTPSamplerProps(props)
	case "SimpleThreadGroup":
		return canonicalizeSimpleThreadGroupProps(props)
	case "RPSThreadGroup":
		return canonicalizeRPSThreadGroupProps(props)
	case "PauseController":
		return canonicalizePauseControllerProps(props)
	case "LoopController":
		return canonicalizeLoopControllerProps(props)
	default:
		return props
	}
}

func canonicalizeHTTPSamplerProps(props map[string]interface{}) map[string]interface{} {
	out := cloneProps(props)
	rawURL, _ := firstStringProp(out, "URL", "url", "Url")
	if queryParams, ok := firstProp(out, "QueryParams", "queryParams", "query_params"); ok {
		rawURL = appendQueryParams(rawURL, queryParams)
	}
	if strings.TrimSpace(rawURL) != "" {
		out["Url"] = rawURL
	}
	if method, ok := firstStringProp(out, "method", "Method"); ok {
		out["Method"] = strings.ToUpper(strings.TrimSpace(method))
	}
	if targetRPS, ok := firstFloatProp(out, "TargetRPS", "target_rps", "rps"); ok {
		out["TargetRPS"] = targetRPS
	}
	if body, ok := firstStringProp(out, "body", "Body"); ok {
		out["Body"] = body
	}
	if extractVars, ok := firstStringSliceProp(out, "ExtractVars", "extractVars", "extract_vars"); ok {
		out["ExtractVars"] = extractVars
	}
	return out
}

func canonicalizeSimpleThreadGroupProps(props map[string]interface{}) map[string]interface{} {
	out := cloneProps(props)
	if users, ok := firstIntProp(out, "Threads", "threads", "MaxUsers", "max_users", "users", "Users"); ok {
		out["Users"] = users
	}
	if iterations, ok := firstIntProp(out, "Iterations", "iterations", "loops", "Loops"); ok {
		out["Iterations"] = iterations
	}
	if timeoutMS, ok := firstDurationMSProp(out,
		[]string{"HTTPRequestTimeoutMS", "HTTPRequestTimeoutMs", "RequestTimeoutMS", "TimeoutMS"},
		[]string{"HTTPRequestTimeoutSeconds", "RequestTimeoutSeconds", "TimeoutSeconds"},
	); ok {
		out["HTTPRequestTimeoutMS"] = timeoutMS
	}
	if keepAlive, ok := firstBoolProp(out, "HTTPKeepAlive", "http_keep_alive", "KeepAlive", "keep_alive"); ok {
		out["HTTPKeepAlive"] = keepAlive
	}
	if parameters, ok := firstProp(out, "Parameters", "parameters"); ok {
		out["Parameters"] = parameters
	}
	return out
}

func canonicalizeRPSThreadGroupProps(props map[string]interface{}) map[string]interface{} {
	out := cloneProps(props)
	if users, ok := firstIntProp(out, "Threads", "threads", "MaxUsers", "max_users", "users", "Users"); ok {
		out["Users"] = users
	}
	if rps, ok := firstFloatProp(out, "TargetRPS", "target_rps", "rps", "RPS"); ok {
		out["RPS"] = rps
	}
	if timeoutMS, ok := firstDurationMSProp(out,
		[]string{"HTTPRequestTimeoutMS", "HTTPRequestTimeoutMs", "RequestTimeoutMS", "TimeoutMS"},
		[]string{"HTTPRequestTimeoutSeconds", "RequestTimeoutSeconds", "TimeoutSeconds"},
	); ok {
		out["HTTPRequestTimeoutMS"] = timeoutMS
	}
	if shutdownMS, ok := firstDurationMSProp(out,
		[]string{"GracefulShutdownMS", "GracefulShutdownMs"},
		[]string{"GracefulShutdownSeconds"},
	); ok {
		out["GracefulShutdownMS"] = shutdownMS
	}
	if keepAlive, ok := firstBoolProp(out, "HTTPKeepAlive", "http_keep_alive", "KeepAlive", "keep_alive"); ok {
		out["HTTPKeepAlive"] = keepAlive
	}
	if parameters, ok := firstProp(out, "Parameters", "parameters"); ok {
		out["Parameters"] = parameters
	}

	blocks := canonicalizeProfileBlocks(out["ProfileBlocks"])
	if len(blocks) == 0 {
		stepDurationMS, hasStepDuration := firstDurationMSProp(out,
			[]string{"DurationMS", "DurationMs", "StepDurationMS", "StepDurationMs"},
			[]string{"DurationSeconds", "StepDurationSeconds", "ProfileDurationSeconds"},
		)
		rampUpMS, hasRampUp := firstDurationMSProp(out,
			[]string{"RampUpMS", "RampUpMs"},
			[]string{"RampUpSeconds"},
		)
		profilePercent, hasPercent := firstFloatProp(out, "ProfilePercent", "profile_percent", "Percent", "percent")
		if !hasPercent {
			profilePercent = 100
		}
		if hasStepDuration || hasRampUp {
			blocks = []interface{}{
				map[string]interface{}{
					"RampUpMS":       rampUpMS,
					"StepDurationMS": stepDurationMS,
					"ProfilePercent": profilePercent,
				},
			}
		}
	}
	if len(blocks) > 0 {
		out["ProfileBlocks"] = blocks
	}

	return out
}

func canonicalizePauseControllerProps(props map[string]interface{}) map[string]interface{} {
	out := cloneProps(props)
	if durationMS, ok := firstDurationMSProp(out,
		[]string{"DurationMS", "DurationMs"},
		[]string{"DurationSeconds"},
	); ok {
		out["DurationMS"] = durationMS
	}
	return out
}

func canonicalizeLoopControllerProps(props map[string]interface{}) map[string]interface{} {
	out := cloneProps(props)
	if loops, ok := firstIntProp(out, "Loops", "loops", "Iterations", "iterations"); ok {
		out["Loops"] = loops
	}
	return out
}

func canonicalizeProfileBlocks(raw interface{}) []interface{} {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	blocks := make([]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		block := cloneProps(m)
		if rampUpMS, ok := firstDurationMSProp(block,
			[]string{"RampUpMS", "RampUpMs"},
			[]string{"RampUpSeconds"},
		); ok {
			block["RampUpMS"] = rampUpMS
		}
		if stepDurationMS, ok := firstDurationMSProp(block,
			[]string{"StepDurationMS", "StepDurationMs", "DurationMS", "DurationMs"},
			[]string{"StepDurationSeconds", "DurationSeconds"},
		); ok {
			block["StepDurationMS"] = stepDurationMS
		}
		if profilePercent, ok := firstFloatProp(block, "ProfilePercent", "profile_percent", "Percent", "percent"); ok {
			block["ProfilePercent"] = profilePercent
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func cloneProps(props map[string]interface{}) map[string]interface{} {
	if len(props) == 0 {
		return make(map[string]interface{})
	}
	out := make(map[string]interface{}, len(props))
	for key, value := range props {
		out[key] = value
	}
	return out
}

func firstProp(props map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, ok := props[key]; ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func firstStringProp(props map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		switch current := value.(type) {
		case string:
			if strings.TrimSpace(current) != "" {
				return current, true
			}
		default:
			text := strings.TrimSpace(fmt.Sprint(current))
			if text != "" {
				return text, true
			}
		}
	}
	return "", false
}

func firstStringSliceProp(props map[string]interface{}, keys ...string) ([]string, bool) {
	for _, key := range keys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		switch current := value.(type) {
		case []string:
			return current, len(current) > 0
		case []interface{}:
			out := make([]string, 0, len(current))
			for _, item := range current {
				text := strings.TrimSpace(fmt.Sprint(item))
				if text != "" {
					out = append(out, text)
				}
			}
			return out, len(out) > 0
		}
	}
	return nil, false
}

func firstIntProp(props map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		if intValue, ok := toInt(value); ok {
			return intValue, true
		}
	}
	return 0, false
}

func firstFloatProp(props map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		if floatValue, ok := toFloat64(value); ok {
			return floatValue, true
		}
	}
	return 0, false
}

func firstBoolProp(props map[string]interface{}, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		switch current := value.(type) {
		case bool:
			return current, true
		case string:
			boolValue, err := strconv.ParseBool(strings.TrimSpace(current))
			if err == nil {
				return boolValue, true
			}
		}
	}
	return false, false
}

func firstDurationMSProp(props map[string]interface{}, millisecondKeys []string, secondKeys []string) (int, bool) {
	for _, key := range millisecondKeys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		if intValue, ok := toInt(value); ok {
			return intValue, true
		}
	}
	for _, key := range secondKeys {
		value, ok := props[key]
		if !ok || value == nil {
			continue
		}
		if floatValue, ok := toFloat64(value); ok {
			return int(floatValue * 1000), true
		}
	}
	return 0, false
}

func toFloat64(value interface{}) (float64, bool) {
	switch current := value.(type) {
	case float64:
		return current, true
	case float32:
		return float64(current), true
	case int:
		return float64(current), true
	case int64:
		return float64(current), true
	case int32:
		return float64(current), true
	case json.Number:
		floatValue, err := current.Float64()
		return floatValue, err == nil
	case string:
		floatValue, err := strconv.ParseFloat(strings.TrimSpace(current), 64)
		return floatValue, err == nil
	default:
		return 0, false
	}
}

func toInt64(value interface{}) (int64, bool) {
	switch current := value.(type) {
	case int:
		return int64(current), true
	case int64:
		return current, true
	case int32:
		return int64(current), true
	case float64:
		return int64(current), true
	case float32:
		return int64(current), true
	case json.Number:
		intValue, err := current.Int64()
		return intValue, err == nil
	case string:
		intValue, err := strconv.ParseInt(strings.TrimSpace(current), 10, 64)
		return intValue, err == nil
	default:
		return 0, false
	}
}

func toInt(value interface{}) (int, bool) {
	intValue, ok := toInt64(value)
	return int(intValue), ok
}

func appendQueryParams(rawURL string, rawParams interface{}) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	params, ok := rawParams.(map[string]interface{})
	if !ok || len(params) == 0 {
		return rawURL
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys)+1)
	if strings.TrimSpace(parsedURL.RawQuery) != "" {
		pairs = append(pairs, parsedURL.RawQuery)
	}
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(params[key]))
		if value == "" {
			continue
		}
		pairs = append(pairs, url.QueryEscape(key)+"="+encodeQueryValue(value))
	}
	parsedURL.RawQuery = strings.Join(pairs, "&")
	return parsedURL.String()
}

func encodeQueryValue(value string) string {
	if isTemplatePlaceholder(value) {
		return value
	}
	return url.QueryEscape(value)
}

func isTemplatePlaceholder(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}
