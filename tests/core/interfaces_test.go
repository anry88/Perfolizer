package core_test

import (
	"strings"
	"testing"

	"perfolizer/pkg/core"
)

func TestBaseElementLifecycle(t *testing.T) {
	parent := core.NewBaseElement("parent")
	if parent.Name() != "parent" {
		t.Fatalf("expected name %q, got %q", "parent", parent.Name())
	}
	if !parent.Enabled() {
		t.Fatal("new element should be enabled")
	}
	if !strings.HasPrefix(parent.ID(), "id_") {
		t.Fatalf("expected generated ID to start with id_, got %q", parent.ID())
	}

	parent.SetName("renamed")
	parent.SetEnabled(false)
	parent.SetID("custom-id")

	if parent.Name() != "renamed" {
		t.Fatalf("expected updated name %q, got %q", "renamed", parent.Name())
	}
	if parent.Enabled() {
		t.Fatal("expected element to be disabled")
	}
	if parent.ID() != "custom-id" {
		t.Fatalf("expected updated ID %q, got %q", "custom-id", parent.ID())
	}

	child1 := newSerializableElement("child-1", uniqueTypeName(t), nil)
	child1.SetID("child-1-id")
	child2 := newSerializableElement("child-2", uniqueTypeName(t), nil)
	child2.SetID("child-2-id")

	parent.AddChild(child1)
	parent.AddChild(child2)

	if got := len(parent.GetChildren()); got != 2 {
		t.Fatalf("expected 2 children, got %d", got)
	}

	parent.RemoveChild("unknown")
	if got := len(parent.GetChildren()); got != 2 {
		t.Fatalf("expected unchanged child count for unknown ID, got %d", got)
	}

	parent.RemoveChild("child-1-id")
	if got := len(parent.GetChildren()); got != 1 {
		t.Fatalf("expected 1 child after removal, got %d", got)
	}
	if parent.GetChildren()[0].ID() != "child-2-id" {
		t.Fatalf("expected remaining child id %q, got %q", "child-2-id", parent.GetChildren()[0].ID())
	}
}

func TestBaseElementCloneDeepCopy(t *testing.T) {
	typeName := uniqueTypeName(t)
	root := newSerializableElement("root", typeName, map[string]interface{}{"key": "value"})
	root.SetID("root-id")
	root.SetEnabled(false)

	child := newSerializableElement("child", typeName, nil)
	child.SetID("child-id")
	root.AddChild(child)

	cloneAny := root.Clone()
	cloned, ok := cloneAny.(*serializableElement)
	if !ok {
		t.Fatalf("expected *serializableElement clone, got %T", cloneAny)
	}

	if cloned == root {
		t.Fatal("clone should be a different pointer")
	}
	if cloned.ID() != root.ID() {
		t.Fatalf("expected cloned ID %q, got %q", root.ID(), cloned.ID())
	}
	if cloned.Enabled() != root.Enabled() {
		t.Fatalf("expected cloned enabled=%v, got %v", root.Enabled(), cloned.Enabled())
	}
	if len(cloned.GetChildren()) != 1 {
		t.Fatalf("expected cloned child count 1, got %d", len(cloned.GetChildren()))
	}

	cloned.SetName("changed-root")
	cloned.GetChildren()[0].SetName("changed-child")

	if root.Name() == "changed-root" {
		t.Fatal("changing clone name should not mutate source")
	}
	if root.GetChildren()[0].Name() == "changed-child" {
		t.Fatal("changing cloned child should not mutate source child")
	}
}

func TestGenerateIDUnique(t *testing.T) {
	id1 := core.GenerateID()
	id2 := core.GenerateID()

	if id1 == id2 {
		t.Fatalf("expected different IDs, got identical values %q", id1)
	}
	if !strings.HasPrefix(id1, "id_") || !strings.HasPrefix(id2, "id_") {
		t.Fatalf("expected IDs to have id_ prefix, got %q and %q", id1, id2)
	}
}
