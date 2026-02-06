package core

import "fmt"

type ConsoleRunner struct {
}

func NewConsoleRunner() *ConsoleRunner {
	return &ConsoleRunner{}
}

func (r *ConsoleRunner) ReportResult(result *SampleResult) {
	fmt.Printf("Sample: %s, Duration: %v, Success: %t\n", result.SamplerName, result.Duration(), result.Success)
}
