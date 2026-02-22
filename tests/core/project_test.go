package core_test

import (
	"testing"

	"perfolizer/pkg/core"
)

func TestProjectPlanManagement(t *testing.T) {
	proj := core.NewProject("Demo")
	if proj.Name != "Demo" {
		t.Fatalf("expected project name %q, got %q", "Demo", proj.Name)
	}
	if got := proj.PlanCount(); got != 0 {
		t.Fatalf("expected empty project, got %d plans", got)
	}

	root := newSerializableElement("PlanRoot", uniqueTypeName(t), nil)
	proj.AddPlan("Main Plan", root)

	if got := proj.PlanCount(); got != 1 {
		t.Fatalf("expected 1 plan, got %d", got)
	}
	if proj.Plans[0].Name != "Main Plan" {
		t.Fatalf("expected plan name %q, got %q", "Main Plan", proj.Plans[0].Name)
	}
	if proj.Plans[0].Root == nil {
		t.Fatal("expected non-nil plan root")
	}
	if proj.Plans[0].Parameters == nil {
		t.Fatal("expected initialized parameters slice")
	}

	proj.RemovePlanAt(-1)
	proj.RemovePlanAt(99)
	if got := proj.PlanCount(); got != 1 {
		t.Fatalf("expected out-of-range removals to keep 1 plan, got %d", got)
	}

	proj.RemovePlanAt(0)
	if got := proj.PlanCount(); got != 0 {
		t.Fatalf("expected plan removal, got %d plans", got)
	}
}
