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
	ErrChart *LineChart
	RpsLabel *widget.Label
	LatLabel *widget.Label
	ErrLabel *widget.Label
	Legend   *fyne.Container

	seriesMap map[string]bool // To track existing checkboxes
}

func NewDashboardWindow(a fyne.App) *DashboardWindow {
	w := a.NewWindow("Test Runtime Dashboard")
	w.Resize(fyne.NewSize(1000, 760))

	rpsChart := NewLineChart(100)
	latChart := NewLineChart(100)
	errChart := NewLineChart(100)

	rpsLabel := widget.NewLabel("Total RPS: 0")
	latLabel := widget.NewLabel("Avg Latency: 0 ms")
	errLabel := widget.NewLabel("Errors (total): 0")

	legend := container.NewHBox(widget.NewLabel("Series:"))

	content := container.NewVBox(
		rpsLabel,
		container.NewPadded(rpsChart),
		latLabel,
		container.NewPadded(latChart),
		errLabel,
		container.NewPadded(errChart),
		widget.NewLabel("Legend:"),
		container.NewHScroll(legend),
	)

	w.SetContent(content)

	return &DashboardWindow{
		Window:    w,
		App:       a,
		RpsChart:  rpsChart,
		LatChart:  latChart,
		ErrChart:  errChart,
		RpsLabel:  rpsLabel,
		LatLabel:  latLabel,
		ErrLabel:  errLabel,
		Legend:    legend,
		seriesMap: make(map[string]bool),
	}
}

func (d *DashboardWindow) Show() {
	d.Window.Show()
}

func (d *DashboardWindow) Update(data map[string]core.Metric) {
	totalRps := 0.0
	totalLat := 0.0
	totalErr := 0
	if t, ok := data["Total"]; ok {
		totalRps = t.RPS
		totalLat = t.AvgLatency
		totalErr = t.TotalErrors
	}

	fyne.Do(func() {
		for name := range data {
			if name == "Total" {
				continue
			}
			if _, exists := d.seriesMap[name]; !exists {
				d.seriesMap[name] = true
				cb := widget.NewCheck(name, func(enabled bool) {
					d.RpsChart.SetVisible(name, enabled)
					d.LatChart.SetVisible(name, enabled)
					d.ErrChart.SetVisible(name, enabled)
				})
				cb.SetChecked(true)
				d.Legend.Add(cb)
			}
		}

		for name, m := range data {
			if name == "Total" {
				continue
			}
			d.RpsChart.Add(name, m.RPS)
			d.LatChart.Add(name, m.AvgLatency)
			d.ErrChart.Add(name, float64(m.TotalErrors))
		}

		d.RpsLabel.SetText(fmt.Sprintf("Total RPS: %.2f", totalRps))
		d.LatLabel.SetText(fmt.Sprintf("Avg Latency: %.2f ms", totalLat))
		d.ErrLabel.SetText(fmt.Sprintf("Errors (total): %d", totalErr))
	})
}
