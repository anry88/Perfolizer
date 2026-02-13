package core

import (
	"context"
	"testing"
)

func TestContext_Substitute(t *testing.T) {
	ctx := NewContext(context.Background(), 0)
	ctx.SetVar("host", "localhost")
	ctx.SetVar("port", "8080")
	ctx.SetVar("path", "api")

	tests := []struct {
		input    string
		expected string
	}{
		{"http://${host}:${port}/${path}", "http://localhost:8080/api"},
		{"no variables", "no variables"},
		{"${missing}", "${missing}"},
		{"prefix-${host}-suffix", "prefix-localhost-suffix"},
	}

	for _, test := range tests {
		result := ctx.Substitute(test.input)
		if result != test.expected {
			t.Errorf("Substitute(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}
