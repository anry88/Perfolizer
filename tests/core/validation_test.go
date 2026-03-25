package core_test

import (
	"strings"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

func TestValidateTestPlanAcceptsBoundaryValues(t *testing.T) {
	root := core.NewBaseElement("Test Plan")

	simple := elements.NewSimpleThreadGroup("Simple", 1, -1)
	sampler := elements.NewHttpSampler("Sampler", "GET", "http://example.com")
	sampler.TargetRPS = 0
	simple.AddChild(sampler)
	simple.AddChild(elements.NewPauseController("Pause", 0))
	root.AddChild(simple)

	rps := elements.NewRPSThreadGroup("RPS", 0)
	rps.Users = 1
	rps.ProfileBlocks = []elements.RPSProfileBlock{
		{RampUp: 0, StepDuration: 0, ProfilePercent: 100},
	}
	rps.GracefulShutdown = 0
	root.AddChild(rps)

	if err := core.ValidateTestPlan(&root); err != nil {
		t.Fatalf("expected boundary values to be valid, got %v", err)
	}
}

func TestValidateTestPlanRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		child    core.TestElement
		contains []string
	}{
		{
			name:     "simple users below one",
			child:    elements.NewSimpleThreadGroup("Broken Users", 0, 1),
			contains: []string{`Simple Thread Group "Broken Users"`, "Users must be greater than or equal to 1"},
		},
		{
			name:     "simple iterations below minus one",
			child:    elements.NewSimpleThreadGroup("Broken Iterations", 1, -2),
			contains: []string{`Simple Thread Group "Broken Iterations"`, "Iterations must be greater than or equal to -1"},
		},
		{
			name: "negative sampler rps",
			child: func() core.TestElement {
				tg := elements.NewSimpleThreadGroup("Sampler Group", 1, 1)
				sampler := elements.NewHttpSampler("Sampler", "GET", "http://example.com")
				sampler.TargetRPS = -0.5
				tg.AddChild(sampler)
				return tg
			}(),
			contains: []string{`HTTP Sampler "Sampler"`, "Target RPS must be greater than or equal to 0"},
		},
		{
			name: "negative pause duration",
			child: func() core.TestElement {
				tg := elements.NewSimpleThreadGroup("Pause Group", 1, 1)
				tg.AddChild(elements.NewPauseController("Pause", -1*time.Millisecond))
				return tg
			}(),
			contains: []string{`Pause Controller "Pause"`, "Duration must be greater than or equal to 0 ms"},
		},
		{
			name: "negative profile block duration",
			child: func() core.TestElement {
				tg := elements.NewRPSThreadGroup("RPS Broken", 0)
				tg.Users = 1
				tg.ProfileBlocks = []elements.RPSProfileBlock{
					{RampUp: -1 * time.Millisecond, StepDuration: time.Second, ProfilePercent: 100},
				}
				return tg
			}(),
			contains: []string{`RPS Thread Group "RPS Broken"`, "Profile block 1 ramp-up must be greater than or equal to 0 ms"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := core.NewBaseElement("Test Plan")
			root.AddChild(tc.child)

			err := core.ValidateTestPlan(&root)
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, expected := range tc.contains {
				if !strings.Contains(err.Error(), expected) {
					t.Fatalf("expected error %q to contain %q", err, expected)
				}
			}
		})
	}
}

func TestValidateTestPlanSkipsDisabledSubtrees(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	disabled := elements.NewSimpleThreadGroup("Disabled Broken", 0, 1)
	disabled.SetEnabled(false)
	root.AddChild(disabled)

	if err := core.ValidateTestPlan(&root); err != nil {
		t.Fatalf("expected disabled invalid subtree to be ignored, got %v", err)
	}
}
