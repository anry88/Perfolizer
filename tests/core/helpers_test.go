package core_test

import (
	"strings"
	"testing"

	"perfolizer/pkg/core"
)

type serializableElement struct {
	core.BaseElement
	typeName string
	props    map[string]interface{}
}

func newSerializableElement(name, typeName string, props map[string]interface{}) *serializableElement {
	base := core.NewBaseElement(name)
	clonedProps := cloneAnyMap(props)
	return &serializableElement{
		BaseElement: base,
		typeName:    typeName,
		props:       clonedProps,
	}
}

func (e *serializableElement) Clone() core.TestElement {
	cloned := newSerializableElement(e.Name(), e.typeName, e.props)
	cloned.SetID(e.ID())
	cloned.SetEnabled(e.Enabled())
	for _, child := range e.GetChildren() {
		cloned.AddChild(child.Clone())
	}
	return cloned
}

func (e *serializableElement) GetType() string {
	return e.typeName
}

func (e *serializableElement) GetProps() map[string]interface{} {
	return cloneAnyMap(e.props)
}

func cloneAnyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func uniqueTypeName(t *testing.T) string {
	t.Helper()
	replacer := strings.NewReplacer("/", "_", " ", "_")
	return "test_stub_" + replacer.Replace(t.Name())
}

func registerSerializableFactory(typeName string) {
	core.RegisterFactory(typeName, func(name string, props map[string]interface{}) core.TestElement {
		return newSerializableElement(name, typeName, props)
	})
}
