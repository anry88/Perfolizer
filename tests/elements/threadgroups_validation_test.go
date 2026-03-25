package elements_test

import (
	"context"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

type noopRunner struct{}

func (noopRunner) ReportResult(*core.SampleResult) {}

func TestSimpleThreadGroupStartWithInvalidUsersDoesNotPanic(t *testing.T) {
	tg := elements.NewSimpleThreadGroup("Broken", -1, 1)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected no panic, got %v", recovered)
		}
	}()

	tg.Start(context.Background(), noopRunner{})
}

func TestRPSThreadGroupStartWithInvalidUsersDoesNotPanic(t *testing.T) {
	tg := elements.NewRPSThreadGroup("Broken RPS", 1)
	tg.Users = -1
	tg.ProfileBlocks = []elements.RPSProfileBlock{
		{RampUp: 0, StepDuration: time.Second, ProfilePercent: 100},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected no panic, got %v", recovered)
		}
	}()

	tg.Start(context.Background(), noopRunner{})
}
