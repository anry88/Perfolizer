package core

import (
	"context"
	"time"
)

// TestElement is the base interface for all nodes in the test plan.
type TestElement interface {
	ID() string
	Name() string
	SetName(name string)
	SetID(id string)
	Clone() TestElement
	GetChildren() []TestElement
	AddChild(child TestElement)
	RemoveChild(childID string)
}

// Executable is implemented by elements that perform an action.
type Executable interface {
	Execute(ctx *Context) error
}

// Sampler represents an action that generates a sample (e.g., HTTP request).
type Sampler interface {
	TestElement
	Executable
}

// Controller acts as a container or logic flow control for other elements.
type Controller interface {
	TestElement
	// Next returns the next element to execute or nil if done.
	Next(ctx *Context) TestElement
}

// ThreadGroup manages the execution of threads (virtual users).
type ThreadGroup interface {
	TestElement
	Start(ctx context.Context, runner Runner)
}

// Runner is the interface used by ThreadGroups to report execution or orchestrate.
type Runner interface {
	ReportResult(result *SampleResult)
}

// BaseElement provides common implementation for TestElement.
type BaseElement struct {
	id       string
	name     string
	children []TestElement
}

func NewBaseElement(name string) BaseElement {
	return BaseElement{
		id:   GenerateID(), // We'll need a helper for this
		name: name,
	}
}

func (b *BaseElement) ID() string {
	return b.id
}

func (b *BaseElement) SetID(id string) {
	b.id = id
}

func (b *BaseElement) Name() string {
	return b.name
}

func (b *BaseElement) SetName(name string) {
	b.name = name
}

func (b *BaseElement) GetChildren() []TestElement {
	return b.children
}

func (b *BaseElement) AddChild(child TestElement) {
	b.children = append(b.children, child)
}

func (b *BaseElement) RemoveChild(childID string) {
	newChildren := make([]TestElement, 0, len(b.children))
	for _, c := range b.children {
		if c.ID() != childID {
			newChildren = append(newChildren, c)
		}
	}
	b.children = newChildren
}

func (b *BaseElement) Clone() TestElement {
	newB := *b
	if b.children != nil {
		newB.children = make([]TestElement, len(b.children))
		for i, c := range b.children {
			newB.children[i] = c.Clone()
		}
	}
	return &newB
}

// GenerateID is a placeholder for prompt UUID generation.
// In a real app we might use google/uuid or similar.
// For now, simple timestamp based or we can add uuid dependency.
// Let's assume we'll add uuid later or use a simple random string.
func GenerateID() string {
	return "id_" + time.Now().Format("20060102150405.000000")
}
