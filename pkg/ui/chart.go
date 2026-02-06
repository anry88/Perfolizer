package ui

import (
	"image/color"
	"sync" // Added sync import

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type LineChart struct {
	widget.BaseWidget
	data      []float64
	maxPoints int
	mu        sync.RWMutex
}

var _ fyne.Widget = (*LineChart)(nil)

func NewLineChart(maxPoints int) *LineChart {
	lc := &LineChart{
		maxPoints: maxPoints,
		data:      make([]float64, 0, maxPoints),
	}
	lc.ExtendBaseWidget(lc)
	return lc
}

func (lc *LineChart) Add(value float64) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.data = append(lc.data, value)
	if len(lc.data) > lc.maxPoints {
		lc.data = lc.data[1:]
	}
	lc.Refresh()
}

func (lc *LineChart) CreateRenderer() fyne.WidgetRenderer {
	return &chartRenderer{lc: lc}
}

type chartRenderer struct {
	lc   *LineChart
	line *canvas.Line
}

func (r *chartRenderer) Destroy() {}

func (r *chartRenderer) Layout(size fyne.Size) {
}

func (r *chartRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 150)
}

func (r *chartRenderer) Refresh() {
	// No-op or clear cache if needed.
}

func (r *chartRenderer) Objects() []fyne.CanvasObject {
	r.lc.mu.RLock()
	defer r.lc.mu.RUnlock()

	data := r.lc.data
	if len(data) < 2 {
		return nil
	}

	size := r.lc.Size()
	width := float64(size.Width)
	height := float64(size.Height)

	// Min/Max for scaling
	minVal, maxVal := data[0], data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Add some padding to Y scaling
	rangeVal := maxVal - minVal
	if rangeVal == 0 {
		rangeVal = 1
	}

	// Create lines
	var objects []fyne.CanvasObject

	// Draw background grid lines? (Maybe later)

	// Connect points
	stepX := width / float64(r.lc.maxPoints-1)

	// Normalize function
	normY := func(val float64) float32 {
		// Invert Y because 0 is top
		ratio := (val - minVal) / rangeVal
		return float32(height - (ratio * height))
	}

	lineColor := theme.PrimaryColor()

	for i := 0; i < len(data)-1; i++ {
		x1 := float32(i) * float32(stepX)
		y1 := normY(data[i])
		x2 := float32(i+1) * float32(stepX)
		y2 := normY(data[i+1])

		line := canvas.NewLine(color.RGBA{R: 0, G: 0, B: 255, A: 255}) // Fallback color
		line.StrokeColor = lineColor
		line.StrokeWidth = 2
		line.Position1 = fyne.NewPos(x1, y1)
		line.Position2 = fyne.NewPos(x2, y2)
		objects = append(objects, line)
	}

	return objects
}
