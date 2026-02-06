package core

import (
	"encoding/json"
	"fmt"
	"os"
)

// DTOs for JSON serialization

type TestElementDTO struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`

	Props    map[string]interface{} `json:"props,omitempty"`
	Children []TestElementDTO       `json:"children,omitempty"`
}

func SaveTestPlan(path string, root TestElement) error {
	dto := toDTO(root)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(dto)
}

func LoadTestPlan(path string) (TestElement, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var dto TestElementDTO
	if err := json.NewDecoder(file).Decode(&dto); err != nil {
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

	// Determine type and specific props
	// This implies we need run-time type switching or an interface method `ToDTO`.
	// Since we are external to the specific types (in core, but types are in elements?),
	// actually `elements` depends on `core`. Only `core` types are visible here.
	// But `SimpleThreadGroup` etc are in `elements`.
	// PROBLEM: `core` cannot import `elements` (cycle).
	// SOLUTION: We should move `Save/Load` logic to a layer that sees both, OR `elements` types implement `Serializable` interface defined in `core`.
	// Let's define `Serializable` in `core`.

	// Actually, for MVP, keeping DTO logic in `core` but using a registry or Map-based properties is easier.
	// BUT we need to know the struct fields.
	// Let's rely on `Serializable` interface.

	// Attempt 2: Use an interface method.
	if s, ok := el.(Serializable); ok {
		dto.Type = s.GetType()
		dto.Props = s.GetProps()
	} else {
		// Fallback or error? For BaseElement (Test Plan)
		dto.Type = "TestPlan" // Default
	}

	dto.ID = el.ID() // Save ID

	for _, child := range el.GetChildren() {
		dto.Children = append(dto.Children, toDTO(child))
	}

	return dto
}

func fromDTO(dto TestElementDTO) (TestElement, error) {
	// We need a Factory.
	// Since `core` cannot know about `elements`, we need a registered factory.
	factory := GetFactory(dto.Type)
	var el TestElement

	if factory == nil {
		if dto.Type == "TestPlan" {
			// Special case for root
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
		// JSON numbers are float64
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
	}
	return def
}
