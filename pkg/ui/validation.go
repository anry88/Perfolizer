package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2/widget"

	"perfolizer/pkg/elements"
)

func (pa *PerfolizerApp) resetPropertyValidationErrors() {
	pa.propertyValidationMu.Lock()
	defer pa.propertyValidationMu.Unlock()
	pa.propertyValidationErrors = make(map[string]error)
}

func (pa *PerfolizerApp) setPropertyValidationError(field string, err error) {
	pa.propertyValidationMu.Lock()
	defer pa.propertyValidationMu.Unlock()

	if err == nil {
		delete(pa.propertyValidationErrors, field)
		return
	}
	pa.propertyValidationErrors[field] = err
}

func (pa *PerfolizerApp) clearPropertyValidationErrorsWithPrefix(prefix string) {
	pa.propertyValidationMu.Lock()
	defer pa.propertyValidationMu.Unlock()

	for field := range pa.propertyValidationErrors {
		if strings.HasPrefix(field, prefix) {
			delete(pa.propertyValidationErrors, field)
		}
	}
}

func (pa *PerfolizerApp) currentPropertyValidationError() error {
	pa.propertyValidationMu.Lock()
	defer pa.propertyValidationMu.Unlock()

	if len(pa.propertyValidationErrors) == 0 {
		return nil
	}

	fields := make([]string, 0, len(pa.propertyValidationErrors))
	for field := range pa.propertyValidationErrors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return pa.propertyValidationErrors[fields[0]]
}

func (pa *PerfolizerApp) bindPropertyValidation(entry *widget.Entry, field string) {
	entry.AlwaysShowValidationError = true
	pa.setPropertyValidationError(field, nil)
}

func (pa *PerfolizerApp) newValidatedIntEntry(field, initialText string, parse func(string) (int, error), apply func(int)) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetText(initialText)
	pa.bindPropertyValidation(entry, field)
	entry.OnChanged = func(s string) {
		value, err := parse(s)
		pa.setPropertyValidationError(field, err)
		entry.SetValidationError(err)
		if err == nil {
			apply(value)
		}
	}
	return entry
}

func (pa *PerfolizerApp) newValidatedInt64Entry(field, initialText string, parse func(string) (int64, error), apply func(int64)) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetText(initialText)
	pa.bindPropertyValidation(entry, field)
	entry.OnChanged = func(s string) {
		value, err := parse(s)
		pa.setPropertyValidationError(field, err)
		entry.SetValidationError(err)
		if err == nil {
			apply(value)
		}
	}
	return entry
}

func (pa *PerfolizerApp) newValidatedFloatEntry(field, initialText string, parse func(string) (float64, error), apply func(float64)) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetText(initialText)
	pa.bindPropertyValidation(entry, field)
	entry.OnChanged = func(s string) {
		value, err := parse(s)
		pa.setPropertyValidationError(field, err)
		entry.SetValidationError(err)
		if err == nil {
			apply(value)
		}
	}
	return entry
}

func parseRequiredInt(field, raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number", field)
	}
	return value, nil
}

func parseRequiredInt64(field, raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number", field)
	}
	return value, nil
}

func parseRequiredFloat(field, raw string) (float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", field)
	}
	return value, nil
}

func parseUsersInput(raw string) (int, error) {
	value, err := parseRequiredInt("Users", raw)
	if err != nil {
		return 0, err
	}
	return value, elements.ValidateUsers(value)
}

func parseIterationsInput(raw string) (int, error) {
	value, err := parseRequiredInt("Iterations", raw)
	if err != nil {
		return 0, err
	}
	return value, elements.ValidateIterations(value)
}

func parseRPSInput(field, raw string) (float64, error) {
	value, err := parseRequiredFloat(field, raw)
	if err != nil {
		return 0, err
	}
	return value, elements.ValidateRPS(field, value)
}

func parseDurationMillisInput(field, raw string) (int64, error) {
	value, err := parseRequiredInt64(field, raw)
	if err != nil {
		return 0, err
	}
	if err := elements.ValidateDuration(field, time.Duration(value)*time.Millisecond); err != nil {
		return 0, err
	}
	return value, nil
}

func parsePositiveDurationMillisInput(field, raw string) (int64, error) {
	value, err := parseDurationMillisInput(field, raw)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0 ms", field)
	}
	return value, nil
}
