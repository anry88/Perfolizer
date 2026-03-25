package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func TestLineChartRendersSinglePointAsMarker(t *testing.T) {
	chart := NewLineChart(100)
	chart.Resize(fyne.NewSize(320, 180))
	chart.Add("single", 42)

	renderer := chart.CreateRenderer()
	objects := renderer.Objects()

	foundMarker := false
	for _, object := range objects {
		if _, ok := object.(*canvas.Circle); ok {
			foundMarker = true
			break
		}
	}
	if !foundMarker {
		t.Fatal("expected a single-point series to render a marker")
	}
}
