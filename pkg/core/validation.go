package core

import (
	"fmt"
	"strings"
	"unicode"
)

// Validatable is implemented by elements that can validate their own runtime configuration.
type Validatable interface {
	Validate() error
}

// ValidationError wraps a plan validation failure so callers can distinguish
// invalid user input from transport or runtime errors.
type ValidationError struct {
	Err error
}

func (e *ValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "validation failed"
	}
	return e.Err.Error()
}

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ValidateTestPlan checks the executable portion of a plan before a run starts.
// Disabled subtrees are skipped because they are not executed.
func ValidateTestPlan(root TestElement) error {
	if root == nil {
		return fmt.Errorf("test plan is required")
	}
	return validateElementForRun(root, true)
}

func validateElementForRun(el TestElement, isRoot bool) error {
	if el == nil {
		return nil
	}
	if !isRoot && !el.Enabled() {
		return nil
	}
	if validatable, ok := el.(Validatable); ok {
		if err := validatable.Validate(); err != nil {
			return fmt.Errorf("%s: %w", describeElement(el), err)
		}
	}
	for _, child := range el.GetChildren() {
		if err := validateElementForRun(child, false); err != nil {
			return err
		}
	}
	return nil
}

func describeElement(el TestElement) string {
	if el == nil {
		return "test element"
	}

	typeLabel := "Test Element"
	if serializable, ok := el.(Serializable); ok {
		typeLabel = humanizeElementType(serializable.GetType())
	}

	name := strings.TrimSpace(el.Name())
	if name == "" {
		return typeLabel
	}
	return fmt.Sprintf("%s %q", typeLabel, name)
}

func humanizeElementType(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return "Test Element"
	}
	switch typeName {
	case "SimpleThreadGroup":
		return "Simple Thread Group"
	case "RPSThreadGroup":
		return "RPS Thread Group"
	case "HttpSampler":
		return "HTTP Sampler"
	case "LoopController":
		return "Loop Controller"
	case "IfController":
		return "If Controller"
	case "PauseController":
		return "Pause Controller"
	}

	var b strings.Builder
	var prev rune
	for i, r := range typeName {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}
