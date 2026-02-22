package elements_test

import (
	"testing"

	"perfolizer/pkg/elements"
)

func TestExtractJSONPathSimple(t *testing.T) {
	const payload = `{
  "user": {
    "name": "alice",
    "active": true,
    "score": 42.5,
    "roles": ["admin", "qa"]
  },
  "items": [
    {"id": "a1", "count": 2},
    {"id": "b2", "count": 5}
  ]
}`

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "nested string", path: "user.name", expected: "alice"},
		{name: "bool", path: "user.active", expected: "true"},
		{name: "number", path: "user.score", expected: "42.5"},
		{name: "array item field", path: "items.1.id", expected: "b2"},
		{name: "array as json", path: "user.roles", expected: `["admin","qa"]`},
		{name: "object as json", path: "items.0", expected: `{"count":2,"id":"a1"}`},
	}

	for _, tc := range tests {
		if got := elements.ExtractJSONPathSimple(payload, tc.path); got != tc.expected {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.expected, got)
		}
	}
}

func TestExtractJSONPathSimpleReturnsEmptyForInvalidInput(t *testing.T) {
	if got := elements.ExtractJSONPathSimple("", "user.name"); got != "" {
		t.Fatalf("expected empty for empty JSON, got %q", got)
	}
	if got := elements.ExtractJSONPathSimple(`{"user":{"name":"alice"}}`, ""); got != "" {
		t.Fatalf("expected empty for empty path, got %q", got)
	}
	if got := elements.ExtractJSONPathSimple(`{"user":{"name":"alice"}}`, "user.unknown"); got != "" {
		t.Fatalf("expected empty for missing field, got %q", got)
	}
	if got := elements.ExtractJSONPathSimple(`{"items":[{"id":"a"}]}`, "items.bad.id"); got != "" {
		t.Fatalf("expected empty for invalid array index, got %q", got)
	}
	if got := elements.ExtractJSONPathSimple(`{broken`, "user.name"); got != "" {
		t.Fatalf("expected empty for invalid json, got %q", got)
	}
}
