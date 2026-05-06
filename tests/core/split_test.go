package core_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

func TestSplitTestPlanForAgents_SingleAgent(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 10, 5)
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	loadedTG, ok := plans[0].GetChildren()[0].(*elements.SimpleThreadGroup)
	if !ok {
		t.Fatalf("expected SimpleThreadGroup, got %T", plans[0].GetChildren()[0])
	}
	if loadedTG.Users != 10 {
		t.Fatalf("expected 10 users, got %d", loadedTG.Users)
	}
}

func TestSplitTestPlanForAgents_SimpleThreadGroupEvenSplit(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 9, 5)
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}
	for i, plan := range plans {
		loadedTG, ok := plan.GetChildren()[0].(*elements.SimpleThreadGroup)
		if !ok {
			t.Fatalf("plan %d: expected SimpleThreadGroup, got %T", i, plan.GetChildren()[0])
		}
		if loadedTG.Users != 3 {
			t.Fatalf("plan %d: expected 3 users, got %d", i, loadedTG.Users)
		}
	}
}

func TestSplitTestPlanForAgents_SimpleThreadGroupWithRemainder(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 10, 5)
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}

	// 10 / 3 = 3 remainder 1. First agent gets 4, others get 3.
	expected := []int{4, 3, 3}
	totalUsers := 0
	for i, plan := range plans {
		loadedTG, ok := plan.GetChildren()[0].(*elements.SimpleThreadGroup)
		if !ok {
			t.Fatalf("plan %d: expected SimpleThreadGroup, got %T", i, plan.GetChildren()[0])
		}
		if loadedTG.Users != expected[i] {
			t.Fatalf("plan %d: expected %d users, got %d", i, expected[i], loadedTG.Users)
		}
		totalUsers += loadedTG.Users
	}
	if totalUsers != 10 {
		t.Fatalf("expected total 10 users, got %d", totalUsers)
	}
}

func TestSplitTestPlanForAgents_RPSThreadGroup(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewRPSThreadGroup("RPS-TG", 100.0)
	tg.Users = 10
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}

	totalRPS := 0.0
	totalUsers := 0
	for i, plan := range plans {
		loadedTG, ok := plan.GetChildren()[0].(*elements.RPSThreadGroup)
		if !ok {
			t.Fatalf("plan %d: expected RPSThreadGroup, got %T", i, plan.GetChildren()[0])
		}
		totalRPS += loadedTG.RPS
		totalUsers += loadedTG.Users
	}

	// RPS should be evenly split: 100/3 ≈ 33.33 * 3 ≈ 100
	if totalRPS < 99.9 || totalRPS > 100.1 {
		t.Fatalf("expected total RPS ~100, got %f", totalRPS)
	}
	// Users 10 / 3 = 3,3,4 → total 10
	if totalUsers != 10 {
		t.Fatalf("expected total 10 users, got %d", totalUsers)
	}
}

func TestSplitTestPlanForAgents_MultipleThreadGroups(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg1 := elements.NewSimpleThreadGroup("TG1", 7, 5)
	tg2 := elements.NewSimpleThreadGroup("TG2", 12, 5)
	root.AddChild(tg1)
	root.AddChild(tg2)

	plans, err := core.SplitTestPlanForAgents(&root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	totalUsers1, totalUsers2 := 0, 0
	for _, plan := range plans {
		children := plan.GetChildren()
		if len(children) != 2 {
			t.Fatalf("expected 2 children, got %d", len(children))
		}
		loadedTG1 := children[0].(*elements.SimpleThreadGroup)
		loadedTG2 := children[1].(*elements.SimpleThreadGroup)
		totalUsers1 += loadedTG1.Users
		totalUsers2 += loadedTG2.Users
	}
	if totalUsers1 != 7 {
		t.Fatalf("expected total 7 users for TG1, got %d", totalUsers1)
	}
	if totalUsers2 != 12 {
		t.Fatalf("expected total 12 users for TG2, got %d", totalUsers2)
	}
}

func TestSplitTestPlanForAgents_PreservesOtherProperties(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 6, 10)
	tg.HTTPRequestTimeout = 3000 * time.Millisecond
	tg.HTTPKeepAlive = false

	sampler := &elements.HttpSampler{
		BaseElement: core.NewBaseElement("HTTP Request"),
		Method:      "POST",
		Url:         "http://example.com/api",
		Body:        `{"key":"value"}`,
		TargetRPS:   50.0,
	}
	tg.AddChild(sampler)
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, plan := range plans {
		loadedTG := plan.GetChildren()[0].(*elements.SimpleThreadGroup)
		if loadedTG.Iterations != 10 {
			t.Fatalf("plan %d: expected 10 iterations, got %d", i, loadedTG.Iterations)
		}
		if loadedTG.HTTPRequestTimeout != 3000*time.Millisecond {
			t.Fatalf("plan %d: expected 3000ms timeout, got %v", i, loadedTG.HTTPRequestTimeout)
		}
		if loadedTG.HTTPKeepAlive {
			t.Fatalf("plan %d: expected keep-alive=false", i)
		}
		loadedSampler := loadedTG.GetChildren()[0].(*elements.HttpSampler)
		if loadedSampler.Url != "http://example.com/api" {
			t.Fatalf("plan %d: expected URL preserved, got %s", i, loadedSampler.Url)
		}
		if loadedSampler.Method != "POST" {
			t.Fatalf("plan %d: expected POST method, got %s", i, loadedSampler.Method)
		}
	}
}

func TestSplitTestPlanForAgents_InvalidInputs(t *testing.T) {
	root := core.NewBaseElement("Test Plan")

	if _, err := core.SplitTestPlanForAgents(&root, 0); err == nil {
		t.Fatal("expected error for 0 agents")
	}
	if _, err := core.SplitTestPlanForAgents(&root, -1); err == nil {
		t.Fatal("expected error for negative agents")
	}
	if _, err := core.SplitTestPlanForAgents(nil, 3); err == nil {
		t.Fatal("expected error for nil root")
	}
}

func TestSaveSplitTestPlans(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 10, 5)
	root.AddChild(tg)

	dir := t.TempDir()
	basePath := filepath.Join(dir, "plan.json")

	paths, err := core.SaveSplitTestPlans(basePath, &root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(paths))
	}

	totalUsers := 0
	for i, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file %d not found: %v", i, err)
		}
		plan, err := core.LoadTestPlan(path)
		if err != nil {
			t.Fatalf("failed to load plan %d: %v", i, err)
		}
		loadedTG := plan.GetChildren()[0].(*elements.SimpleThreadGroup)
		totalUsers += loadedTG.Users
	}
	if totalUsers != 10 {
		t.Fatalf("expected total 10 users across split plans, got %d", totalUsers)
	}
}

func TestSplitTestPlanForAgents_HttpSamplerTargetRPS(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 6, 5)
	sampler := &elements.HttpSampler{
		BaseElement: core.NewBaseElement("HTTP Request"),
		Method:      "GET",
		Url:         "http://example.com",
		TargetRPS:   90.0,
	}
	tg.AddChild(sampler)
	root.AddChild(tg)

	plans, err := core.SplitTestPlanForAgents(&root, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(plans))
	}

	totalRPS := 0.0
	for i, plan := range plans {
		loadedTG := plan.GetChildren()[0].(*elements.SimpleThreadGroup)
		loadedSampler := loadedTG.GetChildren()[0].(*elements.HttpSampler)
		totalRPS += loadedSampler.TargetRPS
		// Each sampler should get 90/3 = 30 RPS
		if loadedSampler.TargetRPS < 29.9 || loadedSampler.TargetRPS > 30.1 {
			t.Fatalf("plan %d: expected ~30 TargetRPS, got %f", i, loadedSampler.TargetRPS)
		}
	}
	if totalRPS < 89.9 || totalRPS > 90.1 {
		t.Fatalf("expected total ~90 TargetRPS, got %f", totalRPS)
	}
}

func TestSaveSplitTestPlans_RemovesEmptyRootFile(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG1", 4, 1)
	root.AddChild(tg)

	dir := t.TempDir()
	basePath := filepath.Join(dir, "plan.json")

	// Simulate the empty file that Fyne's file-save dialog creates
	if err := os.WriteFile(basePath, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create root file: %v", err)
	}

	_, err := core.SaveSplitTestPlans(basePath, &root, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The empty root file should be removed
	if _, err := os.Stat(basePath); err == nil {
		t.Fatal("expected root file to be removed, but it still exists")
	}

	// The split files should exist
	for i := 1; i <= 2; i++ {
		path := filepath.Join(dir, fmt.Sprintf("plan_%d.json", i))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("split file %d not found: %v", i, err)
		}
	}
}
