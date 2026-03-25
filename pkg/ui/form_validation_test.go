package ui

import (
	"strings"
	"testing"

	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestValidatedEntryTracksErrorsInsideForm(t *testing.T) {
	testApp := fynetest.NewApp()
	defer testApp.Quit()

	pa := &PerfolizerApp{
		propertyValidationErrors: make(map[string]error),
	}

	entry := pa.newValidatedIntEntry("Users", "1", parseUsersInput, func(int) {})
	form := widget.NewForm(widget.NewFormItem("Users", entry))
	fynetest.NewTempWindow(t, form)

	entry.SetText("-1")

	err := pa.currentPropertyValidationError()
	if err == nil {
		t.Fatal("expected validation error to be tracked")
	}
	if !strings.Contains(err.Error(), "Users must be greater than or equal to 1") {
		t.Fatalf("unexpected error: %v", err)
	}

	entry.SetText("2")

	if err := pa.currentPropertyValidationError(); err != nil {
		t.Fatalf("expected validation state to clear, got %v", err)
	}
}
