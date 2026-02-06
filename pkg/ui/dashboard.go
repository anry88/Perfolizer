package ui

import (
	"fmt"
	"perfolizer/pkg/core"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type DashboardWindow struct {
	App      fyne.App
	Window   fyne.Window
	RpsChart *LineChart
	LatChart *LineChart
	RpsLabel *widget.Label
	LatLabel *widget.Label
	Legend   *fyne.Container

	seriesMap map[string]bool // To track existing checkboxes
}

func NewDashboardWindow(a fyne.App) *DashboardWindow {
	w := a.NewWindow("Test Runtime Dashboard")
	w.Resize(fyne.NewSize(1000, 700))

	rpsChart := NewLineChart(100)
	latChart := NewLineChart(100)

	rpsLabel := widget.NewLabel("Total RPS: 0")
	latLabel := widget.NewLabel("Avg Latency: 0 ms")

	legend := container.NewHBox(widget.NewLabel("Series:"))

	content := container.NewVBox(
		rpsLabel,
		container.NewPadded(rpsChart),
		latLabel,
		container.NewPadded(latChart),
		widget.NewLabel("Legend:"),
		container.NewHScroll(legend),
	)

	w.SetContent(content)

	return &DashboardWindow{
		Window:    w,
		App:       a,
		RpsChart:  rpsChart,
		LatChart:  latChart,
		RpsLabel:  rpsLabel,
		LatLabel:  latLabel,
		Legend:    legend,
		seriesMap: make(map[string]bool),
	}
}

func (d *DashboardWindow) Show() {
	d.Window.Show()
}

func (d *DashboardWindow) Update(data map[string]core.Metric) {
	// Aggregate total for labels
	totalRps := 0.0
	totalLat := 0.0
	if t, ok := data["Total"]; ok {
		totalRps = t.RPS
		totalLat = t.AvgLatency
	}

	// Make sure to use Thread-safe call for UI
	// Assuming fyne.Do is available in the imported version, otherwise use Window.Canvas().Refresh() approach?
	// The error message specifically requested fyne.Do
	fyne.Do(func() {
		// Detect new series for Legend
		for name := range data {
			if name == "Total" {
				continue
			}
			if _, exists := d.seriesMap[name]; !exists {
				// Add Checkbox
				d.seriesMap[name] = true
				cb := widget.NewCheck(name, func(b bool) {
					d.RpsChart.SetVisible(name, b)
					d.LatChart.SetVisible(name, b)
				})
				cb.SetChecked(true)
				d.Legend.Add(cb)
			}
		}

		for name, m := range data {
			if name == "Total" {
				continue
			}
			fmt.Printf("Updating Chart Series: %s -> RPS: %f\n", name, m.RPS)
			d.RpsChart.Add(name, m.RPS)
			d.LatChart.Add(name, m.AvgLatency)
		}

		d.RpsLabel.SetText(fmt.Sprintf("Total RPS: %.2f", totalRps))
		d.LatLabel.SetText(fmt.Sprintf("Avg Latency: %.2f ms", totalLat))
	})
}
