package ui

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
)

// GenerateIcon creates a simple icon for the application
func GenerateIcon(filename string) error {
	// Create a 512x512 image
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))

	// Background gradient (purple to blue)
	purple := color.RGBA{124, 58, 237, 255} // #7C3AED
	blue := color.RGBA{59, 130, 246, 255}   // #3B82F6

	// Fill with gradient
	for y := 0; y < 512; y++ {
		ratio := float64(y) / 512.0
		r := uint8(float64(purple.R)*(1-ratio) + float64(blue.R)*ratio)
		g := uint8(float64(purple.G)*(1-ratio) + float64(blue.G)*ratio)
		b := uint8(float64(purple.B)*(1-ratio) + float64(blue.B)*ratio)
		c := color.RGBA{r, g, b, 255}
		for x := 0; x < 512; x++ {
			img.Set(x, y, c)
		}
	}

	// Draw performance chart bars (ascending)
	white := color.RGBA{255, 255, 255, 255}
	barWidth := 60
	spacing := 20
	startX := 100

	// Draw 4 ascending bars
	heights := []int{120, 180, 240, 300}
	for i, h := range heights {
		x := startX + i*(barWidth+spacing)
		y := 512 - 100 - h
		rect := image.Rect(x, y, x+barWidth, 512-100)
		draw.Draw(img, rect, &image.Uniform{white}, image.Point{}, draw.Src)
	}

	// Draw circular gauge/speedometer indicator
	centerX, centerY := 256, 150
	radius := 80
	// Draw circle outline
	drawCircle(img, centerX, centerY, radius, white, 8)

	// Draw needle pointing up-right (performance indicator)
	drawLine(img, centerX, centerY, centerX+50, centerY-50, white, 6)

	// Save to file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// drawCircle draws a circle outline
func drawCircle(img *image.RGBA, cx, cy, r int, c color.RGBA, thickness int) {
	for angle := 0.0; angle < 360.0; angle += 0.5 {
		rad := angle * 3.14159 / 180.0
		for t := -thickness / 2; t < thickness/2; t++ {
			x := cx + int(float64(r+t)*cos(rad))
			y := cy + int(float64(r+t)*sin(rad))
			if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.Set(x, y, c)
			}
		}
	}
}

// drawLine draws a thick line
func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA, thickness int) {
	dx := x2 - x1
	dy := y2 - y1
	steps := max(abs(dx), abs(dy))

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x1 + int(float64(dx)*t)
		y := y1 + int(float64(dy)*t)

		// Draw thickness
		for tx := -thickness / 2; tx < thickness/2; tx++ {
			for ty := -thickness / 2; ty < thickness/2; ty++ {
				px, py := x+tx, y+ty
				if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
	}
}

func cos(rad float64) float64 {
	// Simple cos approximation
	return float64(1.0 - rad*rad/2.0 + rad*rad*rad*rad/24.0)
}

func sin(rad float64) float64 {
	// Simple sin approximation
	return float64(rad - rad*rad*rad/6.0 + rad*rad*rad*rad*rad/120.0)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
