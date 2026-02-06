package main

import (
	"perfolizer/pkg/ui"
)

func main() {
	// Create and run the UI application
	app := ui.NewPerfolizerApp()
	app.Run()
}
