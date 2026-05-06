package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SplitTestPlanForAgents takes a test plan and splits it into agentCount copies,
// distributing the load (Users / RPS) evenly across agents.
// Indivisible remainders are distributed to the first agents.
//
// Returns agentCount deep-cloned plans with adjusted thread group parameters.
func SplitTestPlanForAgents(root TestElement, agentCount int) ([]TestElement, error) {
	if agentCount <= 0 {
		return nil, fmt.Errorf("agent count must be positive, got %d", agentCount)
	}
	if root == nil {
		return nil, fmt.Errorf("test plan root is nil")
	}
	if agentCount == 1 {
		return []TestElement{root.Clone()}, nil
	}

	// Serialize once into DTO, then deserialize N times to get N independent copies.
	dto := TestElementToDTO(root)
	plans := make([]TestElement, agentCount)
	for i := 0; i < agentCount; i++ {
		// Deep clone via JSON round-trip through the DTO layer
		raw, err := json.Marshal(dto)
		if err != nil {
			return nil, fmt.Errorf("marshal plan for split: %w", err)
		}
		var cloneDTO TestElementDTO
		if err := json.Unmarshal(raw, &cloneDTO); err != nil {
			return nil, fmt.Errorf("unmarshal plan for split: %w", err)
		}
		// Adjust load for this agent index
		adjustDTOLoad(&cloneDTO, i, agentCount)
		el, err := DTOToTestElement(cloneDTO)
		if err != nil {
			return nil, fmt.Errorf("reconstruct plan copy %d: %w", i, err)
		}
		plans[i] = el
	}
	return plans, nil
}

// adjustDTOLoad walks the DTO tree and divides Users/RPS in thread groups
// and TargetRPS in HTTP samplers for the given agentIndex (0-based) out of
// agentCount total agents.
func adjustDTOLoad(dto *TestElementDTO, agentIndex, agentCount int) {
	switch dto.Type {
	case "SimpleThreadGroup":
		splitIntProp(dto.Props, "Users", agentIndex, agentCount)
	case "RPSThreadGroup":
		splitFloatProp(dto.Props, "RPS", agentCount)
		splitIntProp(dto.Props, "Users", agentIndex, agentCount)
	case "HttpSampler":
		splitFloatProp(dto.Props, "TargetRPS", agentCount)
	}

	for i := range dto.Children {
		adjustDTOLoad(&dto.Children[i], agentIndex, agentCount)
	}
}

// splitIntProp divides an integer property across agents with remainder.
func splitIntProp(props map[string]interface{}, key string, agentIndex, agentCount int) {
	val, ok := props[key]
	if !ok {
		return
	}
	total := toInt(val)
	if total <= 0 {
		return
	}
	quotient := total / agentCount
	remainder := total % agentCount
	share := quotient
	if agentIndex < remainder {
		share++
	}
	// Ensure at least 1 per agent if total >= agentCount
	if share < 1 && total >= agentCount {
		share = 1
	}
	props[key] = share
}

// splitFloatProp divides a float property evenly across agents.
func splitFloatProp(props map[string]interface{}, key string, agentCount int) {
	val, ok := props[key]
	if !ok {
		return
	}
	total := toFloat64(val)
	if total > 0 {
		props[key] = total / float64(agentCount)
	}
}

// toInt converts a JSON-decoded value to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

// toFloat64 converts a JSON-decoded value to float64.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// SaveSplitTestPlans splits the test plan and saves each part to a separate file.
// Files are named as basePath_1.json, basePath_2.json, etc.
// basePath should be a path without extension (e.g. /path/to/plan).
// If a file exists at basePath itself (e.g. created by a file-save dialog),
// it is removed to avoid leaving an empty root file.
func SaveSplitTestPlans(basePath string, root TestElement, agentCount int) ([]string, error) {
	plans, err := SplitTestPlanForAgents(root, agentCount)
	if err != nil {
		return nil, err
	}

	// Remove extension if present to build numbered filenames
	ext := filepath.Ext(basePath)
	stem := strings.TrimSuffix(basePath, ext)
	if ext == "" {
		ext = ".json"
	}

	// Remove the empty root file left by the file-save dialog (if it exists)
	_ = os.Remove(basePath)

	savedPaths := make([]string, 0, len(plans))
	for i, plan := range plans {
		path := fmt.Sprintf("%s_%d%s", stem, i+1, ext)
		file, err := os.Create(path)
		if err != nil {
			return savedPaths, fmt.Errorf("create file %s: %w", path, err)
		}
		if err := WriteTestPlan(file, plan, true); err != nil {
			file.Close()
			return savedPaths, fmt.Errorf("write plan %d: %w", i+1, err)
		}
		file.Close()
		savedPaths = append(savedPaths, path)
	}
	return savedPaths, nil
}
