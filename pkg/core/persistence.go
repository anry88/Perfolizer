package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// DTOs for JSON serialization

type TestElementDTO struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled,omitempty"` // nil/omit = true for backward compatibility

	Props    map[string]interface{} `json:"props,omitempty"`
	Children []TestElementDTO       `json:"children,omitempty"`
}

// ProjectDTO is the JSON shape for a saved project (one file, multiple plans).
type ProjectDTO struct {
	Name  string         `json:"name"`
	Plans []PlanEntryDTO `json:"plans"`
}

// PlanEntryDTO is one test plan inside a project file.
// PlanEntryDTO is one test plan inside a project file.
type PlanEntryDTO struct {
	Name       string         `json:"name"`
	Plan       TestElementDTO `json:"plan"`
	Parameters []Parameter    `json:"parameters,omitempty"`
}

func SaveProject(path string, proj *Project) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return WriteProject(file, proj, true)
}

func LoadProject(path string) (*Project, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ReadProject(file)
}

func WriteProject(w io.Writer, proj *Project, pretty bool) error {
	dto := ProjectDTO{Name: proj.Name, Plans: make([]PlanEntryDTO, 0, len(proj.Plans))}
	for _, pe := range proj.Plans {
		dto.Plans = append(dto.Plans, PlanEntryDTO{
			Name:       pe.Name,
			Plan:       toDTO(pe.Root),
			Parameters: pe.Parameters,
		})
	}
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(dto)
}

func ReadProject(r io.Reader) (*Project, error) {
	var dto ProjectDTO
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return nil, err
	}
	proj := &Project{Name: dto.Name, Plans: make([]PlanEntry, 0, len(dto.Plans))}
	for _, pe := range dto.Plans {
		root, err := fromDTO(pe.Plan)
		if err != nil {
			return nil, err
		}
		// Ensure non-nil parameters
		params := pe.Parameters
		if params == nil {
			params = make([]Parameter, 0)
		}
		proj.Plans = append(proj.Plans, PlanEntry{Name: pe.Name, Root: root, Parameters: params})
	}
	return proj, nil
}

func SaveTestPlan(path string, root TestElement) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return WriteTestPlan(file, root, true)
}

func LoadTestPlan(path string) (TestElement, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return ReadTestPlan(file)
}

func WriteTestPlan(w io.Writer, root TestElement, pretty bool) error {
	dto := toDTO(root)

	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(dto)
}

func ReadTestPlan(r io.Reader) (TestElement, error) {
	var dto TestElementDTO
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return nil, err
	}
	return fromDTO(dto)
}

func MarshalTestPlan(root TestElement) ([]byte, error) {
	dto := toDTO(root)
	return json.Marshal(dto)
}

func UnmarshalTestPlan(data []byte) (TestElement, error) {
	var dto TestElementDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}
	return fromDTO(dto)
}

func toDTO(el TestElement) TestElementDTO {
	dto := TestElementDTO{
		Name:     el.Name(),
		Props:    make(map[string]interface{}),
		Children: make([]TestElementDTO, 0),
	}

	if s, ok := el.(Serializable); ok {
		dto.Type = s.GetType()
		dto.Props = s.GetProps()
	} else {
		dto.Type = "TestPlan"
	}

	dto.ID = el.ID()
	enabled := el.Enabled()
	dto.Enabled = &enabled

	for _, child := range el.GetChildren() {
		dto.Children = append(dto.Children, toDTO(child))
	}

	return dto
}

func fromDTO(dto TestElementDTO) (TestElement, error) {
	factory := GetFactory(dto.Type)
	var el TestElement

	if factory == nil {
		if dto.Type == "TestPlan" {
			e := NewBaseElement(dto.Name)
			el = &e
		} else {
			return nil, fmt.Errorf("unknown element type: %s", dto.Type)
		}
	} else {
		el = factory(dto.Name, dto.Props)
	}

	if dto.ID != "" {
		el.SetID(dto.ID)
	}
	if dto.Enabled != nil {
		el.SetEnabled(*dto.Enabled)
	}

	loadChildren(el, dto.Children)
	return el, nil
}

func loadChildren(parent TestElement, children []TestElementDTO) {
	for _, childDTO := range children {
		if child, err := fromDTO(childDTO); err == nil {
			parent.AddChild(child)
		}
	}
}

// Factory Registry

type ElementFactory func(name string, props map[string]interface{}) TestElement

var factories = make(map[string]ElementFactory)

func RegisterFactory(typeName string, factory ElementFactory) {
	factories[typeName] = factory
}

func GetFactory(typeName string) ElementFactory {
	return factories[typeName]
}

// Serializable Interface

type Serializable interface {
	GetType() string
	GetProps() map[string]interface{}
}

// Helpers for Property extraction (to be used by elements)
func GetString(props map[string]interface{}, key string, def string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func GetInt(props map[string]interface{}, key string, def int) int {
	if v, ok := props[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
		if i, ok := v.(int); ok {
			return i
		}
	}
	return def
}

func GetFloat(props map[string]interface{}, key string, def float64) float64 {
	if v, ok := props[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
		// JSON numbers might be int
		if i, ok := v.(int); ok {
			return float64(i)
		}
	}
	return def
}

func GetStringMap(props map[string]interface{}, key string) map[string]string {
	if v, ok := props[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			result := make(map[string]string)
			for k, val := range m {
				if str, ok := val.(string); ok {
					result[k] = str
				}
			}
			return result
		}
		if m, ok := v.(map[string]string); ok {
			return m
		}
	}
	return nil
}

func GetStringSlice(props map[string]interface{}, key string) []string {
	if v, ok := props[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
		if arr, ok := v.([]string); ok {
			return arr
		}
	}
	return nil
}

func GetParameters(props map[string]interface{}, key string) []Parameter {
	if v, ok := props[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			params := make([]Parameter, 0, len(arr))
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					params = append(params, Parameter{
						ID:         GetString(m, "ID", ""),
						Name:       GetString(m, "Name", ""),
						Value:      GetString(m, "Value", ""),
						Type:       GetString(m, "Type", "Static"),
						Expression: GetString(m, "Expression", ""),
					})
				}
			}
			return params
		}
		// If already []Parameter (e.g. from internal clone/copy)
		if arr, ok := v.([]Parameter); ok {
			return arr
		}
	}
	return nil
}
