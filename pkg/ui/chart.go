package ui

import (
	"fmt"
	"image/color"
	"sync" // Added sync import

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type Series struct {
	Name    string
	Color   color.Color
	Data    []float64
	Visible bool
}

type LineChart struct {
	widget.BaseWidget
	series    map[string]*Series
	maxPoints int
	mu        sync.RWMutex
}

var _ fyne.Widget = (*LineChart)(nil)

func NewLineChart(maxPoints int) *LineChart {
	lc := &LineChart{
		maxPoints: maxPoints,
		series:    make(map[string]*Series),
	}
	lc.ExtendBaseWidget(lc)
	return lc
}

func (lc *LineChart) Add(name string, value float64) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	s, ok := lc.series[name]
	if !ok {
		// Assign a color based on hash or random?
		// For MVP, simple rotation or hashing
		c := pickColor(name)
		s = &Series{Name: name, Color: c, Data: make([]float64, 0, lc.maxPoints), Visible: true}
		lc.series[name] = s
	}

	s.Data = append(s.Data, value)
	if len(s.Data) > lc.maxPoints {
		s.Data = s.Data[1:]
	}
	lc.Refresh()
}

// Simple color picker
func pickColor(s string) color.Color {
	hash := 0
	for _, c := range s {
		hash = int(c) + ((hash << 5) - hash)
	}
	r := uint8((hash >> 0) & 0xFF)
	g := uint8((hash >> 8) & 0xFF)
	b := uint8((hash >> 16) & 0xFF)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

func (lc *LineChart) SetVisible(name string, visible bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if s, ok := lc.series[name]; ok {
		s.Visible = visible
	} else {
		// If not exists, create it but set visibility
		c := pickColor(name)
		lc.series[name] = &Series{Name: name, Color: c, Data: make([]float64, 0, lc.maxPoints), Visible: visible}
	}
	lc.Refresh()
}

func (lc *LineChart) CreateRenderer() fyne.WidgetRenderer {
	return &chartRenderer{lc: lc}
}

type chartRenderer struct {
	lc *LineChart
}

func (r *chartRenderer) Destroy() {}

func (r *chartRenderer) Layout(size fyne.Size) {
}

func (r *chartRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 150)
}

func (r *chartRenderer) Refresh() {
}

func (r *chartRenderer) Objects() []fyne.CanvasObject {
	r.lc.mu.RLock()
	defer r.lc.mu.RUnlock()

	size := r.lc.Size()
	width := float64(size.Width)
	height := float64(size.Height)

	// Find global Min/Max
	minVal, maxVal := 0.0, 0.0 // Start at 0 usually
	first := true

	for _, s := range r.lc.series {
		if !s.Visible {
			continue
		}
		for _, v := range s.Data {
			if first {
				minVal, maxVal = v, v
				first = false
			} else {
				if v < minVal {
					minVal = v
				}
				if v > maxVal {
					maxVal = v
				}
			}
		}
	}

	// Ensure range
	if minVal > 0 {
		minVal = 0
	} // Anchor to 0 if positive

	rangeVal := maxVal - minVal
	if rangeVal == 0 {
		rangeVal = 1
	}

	// Add padding
	rangeVal *= 1.1

	var objects []fyne.CanvasObject

	// Draw Axis Labels
	// Max
	maxText := canvas.NewText(fmt.Sprintf("%.1f", maxVal), theme.ForegroundColor())
	maxText.TextSize = 10
	maxText.Move(fyne.NewPos(0, 0))
	objects = append(objects, maxText)

	// Zero/Min
	minText := canvas.NewText(fmt.Sprintf("%.1f", minVal), theme.ForegroundColor())
	minText.TextSize = 10
	minText.Move(fyne.NewPos(0, float32(height)-12))
	objects = append(objects, minText)

	stepX := width / float64(r.lc.maxPoints-1)

	// Normalize function
	normY := func(val float64) float32 {
		ratio := (val - minVal) / rangeVal
		return float32(height - (ratio * height))
	}

	for _, s := range r.lc.series {
		if !s.Visible {
			continue
		}
		data := s.Data
		if len(data) < 2 {
			continue
		}

		for i := 0; i < len(data)-1; i++ {
			x1 := float32(i) * float32(stepX)
			y1 := normY(data[i])
			x2 := float32(i+1) * float32(stepX)
			y2 := normY(data[i+1])

			line := canvas.NewLine(s.Color)
			line.StrokeWidth = 2
			line.Position1 = fyne.NewPos(x1, y1)
			line.Position2 = fyne.NewPos(x2, y2)
			objects = append(objects, line)
		}
	}

	return objects
}
