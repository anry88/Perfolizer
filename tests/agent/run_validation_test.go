package agent_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"perfolizer/pkg/agent"
	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

func TestHandleRunRejectsInvalidPlanWithValidationMessage(t *testing.T) {
	server := agent.NewServer(agent.ServerOptions{})

	root := core.NewBaseElement("Test Plan")
	root.AddChild(elements.NewSimpleThreadGroup("Broken Users", 0, 1))

	body, err := core.MarshalTestPlan(&root)
	if err != nil {
		t.Fatalf("failed to marshal test plan: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	message := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(message, "invalid test plan:") {
		t.Fatalf("expected validation prefix, got %q", message)
	}
	if !strings.Contains(message, `Simple Thread Group "Broken Users"`) {
		t.Fatalf("expected element context in message, got %q", message)
	}
	if !strings.Contains(message, "Users must be greater than or equal to 1") {
		t.Fatalf("expected field reason in message, got %q", message)
	}

	running, _ := server.Snapshot()
	if running {
		t.Fatal("expected server to remain stopped after invalid run request")
	}
}
