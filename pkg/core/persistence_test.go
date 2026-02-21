package core

import (
	"strings"
	"testing"
)

func TestReadProject_RejectsSinglePlanJSON(t *testing.T) {
	const singlePlanJSON = `{
	  "type": "TestPlan",
	  "id": "plan_1",
	  "name": "Standalone Plan",
	  "enabled": true,
	  "children": []
	}`

	_, err := ReadProject(strings.NewReader(singlePlanJSON))
	if err == nil {
		t.Fatal("expected ReadProject to fail for standalone test plan json")
	}
}

func TestReadProject_LoadsProjectWithPlans(t *testing.T) {
	const projectJSON = `{
	  "name": "Project",
	  "plans": [
	    {
	      "name": "Main Plan",
	      "plan": {
	        "type": "TestPlan",
	        "id": "plan_root",
	        "name": "Test Plan",
	        "enabled": true,
	        "children": []
	      }
	    }
	  ]
	}`

	proj, err := ReadProject(strings.NewReader(projectJSON))
	if err != nil {
		t.Fatalf("ReadProject returned error: %v", err)
	}
	if proj.PlanCount() != 1 {
		t.Fatalf("expected 1 plan, got %d", proj.PlanCount())
	}
	if proj.Plans[0].Name != "Main Plan" {
		t.Fatalf("expected plan name %q, got %q", "Main Plan", proj.Plans[0].Name)
	}
	if proj.Plans[0].Root == nil {
		t.Fatal("expected non-nil root plan")
	}
}
