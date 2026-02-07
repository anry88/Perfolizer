package core

import (
	"encoding/json"
	"fmt"
	"io"
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
	}
	return def
}
