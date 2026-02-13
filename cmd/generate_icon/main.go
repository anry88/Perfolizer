package main

import (
	"fmt"
	"log"

	"perfolizer/pkg/ui"
)

func main() {
	err := ui.GenerateIcon("Icon.png")
	if err != nil {
		log.Fatal("Failed to generate icon:", err)
	}
	fmt.Println("Icon generated successfully: Icon.png")
}
