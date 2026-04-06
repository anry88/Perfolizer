package core_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

func TestReadProjectRejectsSinglePlanJSON(t *testing.T) {
	const singlePlanJSON = `{
  "type": "TestPlan",
  "id": "plan_1",
  "name": "Standalone Plan",
  "enabled": true,
  "children": []
}`

	_, err := core.ReadProject(strings.NewReader(singlePlanJSON))
	if err == nil {
		t.Fatal("expected ReadProject to fail for standalone test plan json")
	}
}

func TestReadProjectLoadsProjectWithPlans(t *testing.T) {
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

	proj, err := core.ReadProject(strings.NewReader(projectJSON))
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
	if proj.Plans[0].Parameters == nil {
		t.Fatal("expected non-nil parameters slice")
	}
}

func TestWriteReadProjectRoundTrip(t *testing.T) {
	typeName := uniqueTypeName(t)
	registerSerializableFactory(typeName)

	root := newSerializableElement("Root", typeName, map[string]interface{}{"url": "https://example.com"})
	root.SetID("root-id")
	root.SetEnabled(false)

	child := newSerializableElement("Child", typeName, map[string]interface{}{"path": "/v1"})
	child.SetID("child-id")
	root.AddChild(child)

	proj := core.NewProject("Demo Project")
	proj.AddPlan("Main", root)
	proj.Plans[0].Parameters = []core.Parameter{{
		ID:    "p1",
		Name:  "token",
		Type:  core.ParamTypeStatic,
		Value: "abc",
	}}

	var buf bytes.Buffer
	if err := core.WriteProject(&buf, proj, true); err != nil {
		t.Fatalf("WriteProject failed: %v", err)
	}

	loaded, err := core.ReadProject(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadProject failed: %v", err)
	}

	if loaded.Name != "Demo Project" {
		t.Fatalf("expected project name %q, got %q", "Demo Project", loaded.Name)
	}
	if loaded.PlanCount() != 1 {
		t.Fatalf("expected 1 plan, got %d", loaded.PlanCount())
	}

	loadedRoot := loaded.Plans[0].Root
	if loadedRoot == nil {
		t.Fatal("expected loaded root")
	}
	if loadedRoot.ID() != "root-id" {
		t.Fatalf("expected root ID %q, got %q", "root-id", loadedRoot.ID())
	}
	if loadedRoot.Enabled() {
		t.Fatal("expected root to stay disabled")
	}
	if len(loadedRoot.GetChildren()) != 1 {
		t.Fatalf("expected 1 child, got %d", len(loadedRoot.GetChildren()))
	}
	if loadedRoot.GetChildren()[0].ID() != "child-id" {
		t.Fatalf("expected child ID %q, got %q", "child-id", loadedRoot.GetChildren()[0].ID())
	}

	typedRoot, ok := loadedRoot.(*serializableElement)
	if !ok {
		t.Fatalf("expected typed root from factory, got %T", loadedRoot)
	}
	if got := typedRoot.GetProps()["url"]; got != "https://example.com" {
		t.Fatalf("expected restored prop %q, got %#v", "https://example.com", got)
	}

	if len(loaded.Plans[0].Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(loaded.Plans[0].Parameters))
	}
	if loaded.Plans[0].Parameters[0].Name != "token" {
		t.Fatalf("expected parameter name %q, got %q", "token", loaded.Plans[0].Parameters[0].Name)
	}
}

func TestSaveAndLoadProjectFromFile(t *testing.T) {
	root := core.NewBaseElement("Root")
	root.SetID("root-id")
	proj := core.NewProject("File Project")
	proj.AddPlan("Main", &root)

	path := filepath.Join(t.TempDir(), "project.json")
	if err := core.SaveProject(path, proj); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}

	loaded, err := core.LoadProject(path)
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}

	if loaded.PlanCount() != 1 {
		t.Fatalf("expected 1 plan, got %d", loaded.PlanCount())
	}
	if loaded.Plans[0].Root.Name() != "Root" {
		t.Fatalf("expected root name %q, got %q", "Root", loaded.Plans[0].Root.Name())
	}
}

func TestWriteReadMarshalAndUnmarshalTestPlan(t *testing.T) {
	typeName := uniqueTypeName(t)
	registerSerializableFactory(typeName)

	root := newSerializableElement("Root", typeName, map[string]interface{}{"method": "GET"})
	root.SetID("root-id")
	child := newSerializableElement("Child", typeName, nil)
	child.SetID("child-id")
	root.AddChild(child)

	var buf bytes.Buffer
	if err := core.WriteTestPlan(&buf, root, false); err != nil {
		t.Fatalf("WriteTestPlan failed: %v", err)
	}

	loaded, err := core.ReadTestPlan(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadTestPlan failed: %v", err)
	}
	if loaded.Name() != "Root" {
		t.Fatalf("expected loaded root name %q, got %q", "Root", loaded.Name())
	}
	if len(loaded.GetChildren()) != 1 {
		t.Fatalf("expected 1 child, got %d", len(loaded.GetChildren()))
	}

	marshaled, err := core.MarshalTestPlan(root)
	if err != nil {
		t.Fatalf("MarshalTestPlan failed: %v", err)
	}

	unmarshaled, err := core.UnmarshalTestPlan(marshaled)
	if err != nil {
		t.Fatalf("UnmarshalTestPlan failed: %v", err)
	}
	if unmarshaled.Name() != "Root" {
		t.Fatalf("expected unmarshaled root name %q, got %q", "Root", unmarshaled.Name())
	}
}

func TestThreadGroupHTTPSettingsPersistAcrossMarshalRoundTrip(t *testing.T) {
	root := core.NewBaseElement("Test Plan")
	tg := elements.NewSimpleThreadGroup("TG", 1, 1)
	tg.HTTPRequestTimeout = 1750 * time.Millisecond
	tg.HTTPKeepAlive = false
	root.AddChild(tg)

	payload, err := core.MarshalTestPlan(&root)
	if err != nil {
		t.Fatalf("MarshalTestPlan failed: %v", err)
	}

	loaded, err := core.UnmarshalTestPlan(payload)
	if err != nil {
		t.Fatalf("UnmarshalTestPlan failed: %v", err)
	}

	loadedTG, ok := loaded.GetChildren()[0].(*elements.SimpleThreadGroup)
	if !ok {
		t.Fatalf("expected simple thread group, got %T", loaded.GetChildren()[0])
	}

	if loadedTG.HTTPRequestTimeout != 1750*time.Millisecond {
		t.Fatalf("expected timeout %v, got %v", 1750*time.Millisecond, loadedTG.HTTPRequestTimeout)
	}
	if loadedTG.HTTPKeepAlive {
		t.Fatal("expected keep-alive=false to survive round-trip")
	}
}

func TestSaveAndLoadTestPlanFromFile(t *testing.T) {
	root := core.NewBaseElement("Plan Root")
	path := filepath.Join(t.TempDir(), "plan.json")

	if err := core.SaveTestPlan(path, &root); err != nil {
		t.Fatalf("SaveTestPlan failed: %v", err)
	}

	loaded, err := core.LoadTestPlan(path)
	if err != nil {
		t.Fatalf("LoadTestPlan failed: %v", err)
	}
	if loaded.Name() != "Plan Root" {
		t.Fatalf("expected root name %q, got %q", "Plan Root", loaded.Name())
	}
}

func TestReadTestPlanUnknownTypeFails(t *testing.T) {
	const raw = `{"type":"UnknownElement","name":"X","children":[]}`
	if _, err := core.ReadTestPlan(strings.NewReader(raw)); err == nil {
		t.Fatal("expected error for unknown element type")
	}
}

func TestPropertyGetterHelpers(t *testing.T) {
	props := map[string]interface{}{
		"s":               "value",
		"i_float":         float64(7),
		"i_int":           11,
		"f_float":         2.5,
		"f_int":           3,
		"b_true":          true,
		"map_interface":   map[string]interface{}{"a": "1", "skip": 2},
		"map_string":      map[string]string{"b": "2"},
		"slice_interface": []interface{}{"x", 2, "y"},
		"slice_string":    []string{"k", "v"},
		"params_interface": []interface{}{
			map[string]interface{}{
				"ID":         "p1",
				"Name":       "token",
				"Value":      "abc",
				"Type":       core.ParamTypeJSON,
				"Expression": "$.token",
			},
		},
		"params_typed": []core.Parameter{{ID: "p2", Name: "id", Type: core.ParamTypeStatic, Value: "42"}},
	}

	if got := core.GetString(props, "s", "default"); got != "value" {
		t.Fatalf("GetString returned %q", got)
	}
	if got := core.GetString(props, "missing", "default"); got != "default" {
		t.Fatalf("GetString default returned %q", got)
	}

	if got := core.GetInt(props, "i_float", 0); got != 7 {
		t.Fatalf("GetInt(float) returned %d", got)
	}
	if got := core.GetInt(props, "i_int", 0); got != 11 {
		t.Fatalf("GetInt(int) returned %d", got)
	}
	if got := core.GetInt(props, "missing", 9); got != 9 {
		t.Fatalf("GetInt(default) returned %d", got)
	}

	if got := core.GetFloat(props, "f_float", 0); got != 2.5 {
		t.Fatalf("GetFloat(float) returned %f", got)
	}
	if got := core.GetFloat(props, "f_int", 0); got != 3 {
		t.Fatalf("GetFloat(int) returned %f", got)
	}
	if got := core.GetFloat(props, "missing", 1.25); got != 1.25 {
		t.Fatalf("GetFloat(default) returned %f", got)
	}
	if got := core.GetBool(props, "b_true", false); !got {
		t.Fatal("GetBool(bool) returned false")
	}
	if got := core.GetBool(props, "missing_bool", true); !got {
		t.Fatal("GetBool(default) returned false")
	}

	m1 := core.GetStringMap(props, "map_interface")
	if len(m1) != 1 || m1["a"] != "1" {
		t.Fatalf("GetStringMap(interface) returned %#v", m1)
	}
	m2 := core.GetStringMap(props, "map_string")
	if len(m2) != 1 || m2["b"] != "2" {
		t.Fatalf("GetStringMap(string) returned %#v", m2)
	}
	if got := core.GetStringMap(props, "missing"); got != nil {
		t.Fatalf("GetStringMap(missing) expected nil, got %#v", got)
	}

	s1 := core.GetStringSlice(props, "slice_interface")
	if len(s1) != 2 || s1[0] != "x" || s1[1] != "y" {
		t.Fatalf("GetStringSlice(interface) returned %#v", s1)
	}
	s2 := core.GetStringSlice(props, "slice_string")
	if len(s2) != 2 || s2[0] != "k" || s2[1] != "v" {
		t.Fatalf("GetStringSlice(string) returned %#v", s2)
	}
	if got := core.GetStringSlice(props, "missing"); got != nil {
		t.Fatalf("GetStringSlice(missing) expected nil, got %#v", got)
	}

	p1 := core.GetParameters(props, "params_interface")
	if len(p1) != 1 || p1[0].Expression != "$.token" {
		t.Fatalf("GetParameters(interface) returned %#v", p1)
	}
	p2 := core.GetParameters(props, "params_typed")
	if len(p2) != 1 || p2[0].Value != "42" {
		t.Fatalf("GetParameters(typed) returned %#v", p2)
	}
	if got := core.GetParameters(props, "missing"); got != nil {
		t.Fatalf("GetParameters(missing) expected nil, got %#v", got)
	}
}

func TestLoadFunctionsReturnErrorOnBrokenJSON(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "broken-project.json")
	planPath := filepath.Join(dir, "broken-plan.json")

	if err := os.WriteFile(projectPath, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write project fixture failed: %v", err)
	}
	if err := os.WriteFile(planPath, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write plan fixture failed: %v", err)
	}

	if _, err := core.LoadProject(projectPath); err == nil {
		t.Fatal("expected LoadProject error for broken json")
	}
	if _, err := core.LoadTestPlan(planPath); err == nil {
		t.Fatal("expected LoadTestPlan error for broken json")
	}
}

func TestReadProjectWithBrokenChildFails(t *testing.T) {
	const projectJSON = `{
  "name": "Project",
  "plans": [
    {
      "name": "Plan 1",
      "plan": {
        "type": "TestPlan",
        "name": "Root",
        "children": [
          {
            "type": "UnknownType",
            "name": "Broken Child"
          }
        ]
      }
    }
  ]
}`

	_, err := core.ReadProject(strings.NewReader(projectJSON))
	if err == nil {
		t.Fatal("expected ReadProject to fail for unknown child type")
	}

	expectedPart := "plan[0]/UnknownType[0]"
	if !strings.Contains(err.Error(), expectedPart) {
		t.Fatalf("expected error to contain %q, got: %v", expectedPart, err)
	}
}

func TestReadTestPlanDeepNestedError(t *testing.T) {
	const planJSON = `{
  "type": "TestPlan",
  "name": "Root",
  "children": [
    {
      "type": "TestPlan",
      "name": "Level 1",
      "children": [
        {
          "type": "UnknownType",
          "name": "Level 2"
        }
      ]
    }
  ]
}`

	_, err := core.ReadTestPlan(strings.NewReader(planJSON))
	if err == nil {
		t.Fatal("expected ReadTestPlan to fail for deep unknown child type")
	}

	expectedPart := "root/TestPlan[0]/UnknownType[0]"
	if !strings.Contains(err.Error(), expectedPart) {
		t.Fatalf("expected error to contain %q, got: %v", expectedPart, err)
	}
}
