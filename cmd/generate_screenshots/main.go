package main

import (
	"log"
	"path/filepath"

	"fyne.io/fyne/v2/test"

	"perfolizer/pkg/ui"
)

func main() {
	app := test.NewApp()
	outputDir := filepath.Join("docs", "screenshots")

	if err := ui.GenerateShowcaseScreenshots(app, outputDir); err != nil {
		log.Fatalf("generate screenshots: %v", err)
	}

	log.Printf("Showcase screenshots written to %s", outputDir)
}
