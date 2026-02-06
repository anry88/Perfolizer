package elements

import (
	"context"
	"perfolizer/pkg/core"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// --- Simple Thread Group ---

type SimpleThreadGroup struct {
	core.BaseElement
	Users      int
	Iterations int // -1 for infinite
	RampUp     time.Duration
}

func NewSimpleThreadGroup(name string, users, iterations int) *SimpleThreadGroup {
	return &SimpleThreadGroup{
		BaseElement: core.NewBaseElement(name),
		Users:       users,
		Iterations:  iterations,
	}
}

func (tg *SimpleThreadGroup) Clone() core.TestElement {
	newTG := *tg
	newTG.BaseElement = core.NewBaseElement(tg.Name())
	return &newTG
}

func (tg *SimpleThreadGroup) Start(ctx context.Context, runner core.Runner) {
	var wg sync.WaitGroup
	wg.Add(tg.Users)

	// Ramp up calculation
	rampStep := time.Duration(0)
	if tg.Users > 1 && tg.RampUp > 0 {
		rampStep = tg.RampUp / time.Duration(tg.Users-1)
	}

	for i := 0; i < tg.Users; i++ {
		// Delay for rampup
		if i > 0 && rampStep > 0 {
			time.Sleep(rampStep)
		}

		go func(threadID int) {
			defer wg.Done()

			// Thread Context
			tCtx := core.NewContext(ctx, threadID)
			tCtx.SetVar("Reporter", runner)

			for iter := 0; tg.Iterations == -1 || iter < tg.Iterations; iter++ {
				// Check for stop
				select {
				case <-ctx.Done():
					return
				default:
				}

				tCtx.Iteration = iter

				// Execute all children
				for _, child := range tg.GetChildren() {
					if exec, ok := child.(core.Executable); ok {
						_ = exec.Execute(tCtx) // Errors are logged by individual samplers/runner?
						// Or should we stop the thread on error?
						// Usually load tests continue unless configured otherwise.
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

// --- RPS Thread Group ---

type RPSThreadGroup struct {
	core.BaseElement
	Users    int     // Max concurrent workers
	RPS      float64 // Requests (Transactions) per second
	Duration time.Duration
}

func NewRPSThreadGroup(name string, rps float64, duration time.Duration) *RPSThreadGroup {
	return &RPSThreadGroup{
		BaseElement: core.NewBaseElement(name),
		Users:       10, // Default, maybe auto-scale in future
		RPS:         rps,
		Duration:    duration,
	}
}

func (tg *RPSThreadGroup) Clone() core.TestElement {
	newTG := *tg
	newTG.BaseElement = core.NewBaseElement(tg.Name())
	return &newTG
}

func (tg *RPSThreadGroup) Start(ctx context.Context, runner core.Runner) {
	limiter := rate.NewLimiter(rate.Limit(tg.RPS), 1) // Burst 1 for smooth pacing

	// Create a worker pool
	jobs := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(tg.Users)

	// Start workers
	for i := 0; i < tg.Users; i++ {
		go func(threadID int) {
			defer wg.Done()
			tCtx := core.NewContext(ctx, threadID)
			tCtx.SetVar("Reporter", runner)

			for range jobs {
				// Execute children once per job
				for _, child := range tg.GetChildren() {
					if exec, ok := child.(core.Executable); ok {
						_ = exec.Execute(tCtx)
					}
				}
			}
		}(i)
	}

	// Generator loop

	timeout := time.After(tg.Duration)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-timeout:
			break loop
		default:
			// Wait for rate limiter
			if err := limiter.Wait(ctx); err != nil {
				break loop
			}

			// Non-blocking send to ensure we don't pile up if workers are slow
			select {
			case jobs <- struct{}{}:
			default:
				// If we can't send, it means all workers are busy.
				// We recorded a "miss" in accurate RPS, or we could block?
				// For now, let's block to maintain pressure, effectively queuing.
				// Or skip? JMeter has options. Blocking is safer for constant load.
				jobs <- struct{}{}
			}
		}
	}

	close(jobs)
	wg.Wait()
}
