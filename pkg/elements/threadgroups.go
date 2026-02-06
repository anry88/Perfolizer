package elements

import (
	"context"
	"perfolizer/pkg/core"
	"runtime"
	"sync"
	"time"
)

func init() {
	core.RegisterFactory("SimpleThreadGroup", func(name string, props map[string]interface{}) core.TestElement {
		return &SimpleThreadGroup{
			BaseElement: core.NewBaseElement(name),
			Users:       core.GetInt(props, "Users", 1),
			Iterations:  core.GetInt(props, "Iterations", 1),
		}
	})
	core.RegisterFactory("RPSThreadGroup", func(name string, props map[string]interface{}) core.TestElement {
		return &RPSThreadGroup{
			BaseElement: core.NewBaseElement(name),
			Users:       core.GetInt(props, "Users", 10),
			RPS:         core.GetFloat(props, "RPS", 10.0),
			Duration:    time.Duration(core.GetInt(props, "DurationMS", 60000)) * time.Millisecond,
		}
	})
}

// --- Simple Thread Group ---

type SimpleThreadGroup struct {
	core.BaseElement
	Users      int
	Iterations int // -1 for infinite
	RampUp     time.Duration
}

func (tg *SimpleThreadGroup) GetType() string {
	return "SimpleThreadGroup"
}

func (tg *SimpleThreadGroup) GetProps() map[string]interface{} {
	return map[string]interface{}{
		"Users":      tg.Users,
		"Iterations": tg.Iterations,
	}
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
		// Delay for rampup (Cancellable)
		if i > 0 && rampStep > 0 {
			select {
			case <-time.After(rampStep):
			case <-ctx.Done():
				// If canceled during rampup, we still need to account for the added WG count
				// But we shouldn't start the worker
				wg.Done()
				continue
			}
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
						err := exec.Execute(tCtx)
						if err != nil {
							// If error is due to cancellation, stop the thread
							// We might also want to stop on other critical errors if configured
							if ctx.Err() != nil {
								return
							}
						}
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

func (tg *RPSThreadGroup) GetType() string {
	return "RPSThreadGroup"
}

func (tg *RPSThreadGroup) GetProps() map[string]interface{} {
	return map[string]interface{}{
		"Users":      tg.Users,
		"RPS":        tg.RPS,
		"DurationMS": tg.Duration.Milliseconds(),
	}
}

func (tg *RPSThreadGroup) Clone() core.TestElement {
	newTG := *tg
	newTG.BaseElement = core.NewBaseElement(tg.Name())
	return &newTG
}

func (tg *RPSThreadGroup) Start(ctx context.Context, runner core.Runner) {
	// Create a context that expires after Duration
	groupCtx, cancel := context.WithTimeout(ctx, tg.Duration)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(tg.Users)

	// Start workers
	for i := 0; i < tg.Users; i++ {
		go func(threadID int) {
			defer wg.Done()

			// Thread Context
			tCtx := core.NewContext(groupCtx, threadID)
			tCtx.SetVar("Reporter", runner)
			// Inject DefaultRPS for children to inherit if they don't have one
			tCtx.SetVar("DefaultRPS", tg.RPS)

			// Loop until timeout or cancellation
			for {
				select {
				case <-groupCtx.Done():
					return
				default:
					runtime.Gosched()
					// Execute children
					for _, child := range tg.GetChildren() {
						if exec, ok := child.(core.Executable); ok {
							if err := exec.Execute(tCtx); err != nil {
								if groupCtx.Err() != nil {
									return
								}
								// Log other errors?
							}
						}
					}
				}
			}
		}(i)
	}

	wg.Wait()
}
