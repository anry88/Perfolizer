package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type DashboardWindow struct {
	Window   fyne.Window
	RpsChart *LineChart
	LatChart *LineChart
	RpsLabel *widget.Label
	LatLabel *widget.Label
}

func NewDashboardWindow(app fyne.App) *DashboardWindow {
	w := app.NewWindow("Test Dashboard")
	w.Resize(fyne.NewSize(800, 600))

	rpsChart := NewLineChart(60) // 60 seconds
	latChart := NewLineChart(60)

	rpsLabel := widget.NewLabel("RPS: 0")
	latLabel := widget.NewLabel("Avg Latency: 0 ms")

	w.SetContent(container.NewGridWithRows(2,
		container.NewBorder(rpsLabel, nil, nil, nil, rpsChart),
		container.NewBorder(latLabel, nil, nil, nil, latChart),
	))

	return &DashboardWindow{
		Window:   w,
		RpsChart: rpsChart,
		LatChart: latChart,
		RpsLabel: rpsLabel,
		LatLabel: latLabel,
	}
}

func (d *DashboardWindow) Show() {
	d.Window.Show()
}

func (d *DashboardWindow) Update(rps float64, avgLat float64) {
	d.RpsChart.Add(rps)
	d.LatChart.Add(avgLat)

	d.RpsLabel.SetText(fmt.Sprintf("RPS: %.2f", rps))
	d.LatLabel.SetText(fmt.Sprintf("Avg Latency: %.2f ms", avgLat))
}
