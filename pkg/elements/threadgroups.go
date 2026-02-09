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
			BaseElement:      core.NewBaseElement(name),
			Users:            core.GetInt(props, "Users", 10),
			RPS:              core.GetFloat(props, "RPS", 10.0),
			ProfileBlocks:    parseRPSProfileBlocks(props),
			GracefulShutdown: time.Duration(core.GetInt(props, "GracefulShutdownMS", 0)) * time.Millisecond,
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

				// Execute all children (skip disabled)
				for _, child := range tg.GetChildren() {
					if !child.Enabled() {
						continue
					}
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
	Users            int     // Max concurrent workers
	RPS              float64 // Base Requests (Transactions) per second for samplers with TargetRPS=0
	ProfileBlocks    []RPSProfileBlock
	GracefulShutdown time.Duration
}

type RPSProfileBlock struct {
	RampUp         time.Duration
	StepDuration   time.Duration
	ProfilePercent float64
}

func NewRPSThreadGroup(name string, rps float64) *RPSThreadGroup {
	return &RPSThreadGroup{
		BaseElement:      core.NewBaseElement(name),
		Users:            10, // Default, maybe auto-scale in future
		RPS:              rps,
		ProfileBlocks:    []RPSProfileBlock{{RampUp: 0, StepDuration: 60 * time.Second, ProfilePercent: 100}},
		GracefulShutdown: 0,
	}
}

func (tg *RPSThreadGroup) GetType() string {
	return "RPSThreadGroup"
}

func (tg *RPSThreadGroup) GetProps() map[string]interface{} {
	blocks := make([]map[string]interface{}, 0, len(tg.ProfileBlocks))
	for _, block := range tg.ProfileBlocks {
		blocks = append(blocks, map[string]interface{}{
			"RampUpMS":       block.RampUp.Milliseconds(),
			"StepDurationMS": block.StepDuration.Milliseconds(),
			"ProfilePercent": block.ProfilePercent,
		})
	}

	return map[string]interface{}{
		"Users":              tg.Users,
		"RPS":                tg.RPS,
		"ProfileBlocks":      blocks,
		"GracefulShutdownMS": tg.GracefulShutdown.Milliseconds(),
	}
}

func (tg *RPSThreadGroup) Clone() core.TestElement {
	newTG := *tg
	newTG.BaseElement = core.NewBaseElement(tg.Name())
	newTG.ProfileBlocks = append([]RPSProfileBlock(nil), tg.ProfileBlocks...)
	return &newTG
}

func (tg *RPSThreadGroup) Start(ctx context.Context, runner core.Runner) {
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sharedLimiters := newLimiterStore()
	profileScale := newProfileScaleState(1)
	if len(tg.ProfileBlocks) > 0 {
		profileScale.set(0)
	}

	stopRequested := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() {
		stopOnce.Do(func() {
			close(stopRequested)
		})
	}

	go func() {
		defer cancel()

		if len(tg.ProfileBlocks) == 0 {
			requestStop()
			return
		}

		runRPSProfileBlocks(groupCtx, tg.ProfileBlocks, profileScale)
		requestStop()
		if tg.GracefulShutdown > 0 {
			_ = waitForDuration(groupCtx, tg.GracefulShutdown)
		}
	}()

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
			// RPS Thread Group uses shared, non-blocking limiter checks so each sampler
			// can run at its own rate without being stalled by slower siblings.
			tCtx.SetVar("SharedLimiterStore", sharedLimiters)
			tCtx.SetVar("RPSNonBlocking", true)
			tCtx.SetVar("RPSProfileScale", profileScale)

			// Loop until timeout or cancellation
			for {
				select {
				case <-groupCtx.Done():
					return
				case <-stopRequested:
					return
				default:
					runtime.Gosched()
					// Execute children (skip disabled)
					for _, child := range tg.GetChildren() {
						if !child.Enabled() {
							continue
						}
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

func parseRPSProfileBlocks(props map[string]interface{}) []RPSProfileBlock {
	raw := props["ProfileBlocks"]
	if raw == nil {
		legacyDuration := time.Duration(core.GetInt(props, "DurationMS", 0)) * time.Millisecond
		if legacyDuration > 0 {
			return []RPSProfileBlock{{RampUp: 0, StepDuration: legacyDuration, ProfilePercent: 100}}
		}
		return nil
	}

	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	blocks := make([]RPSProfileBlock, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		blocks = append(blocks, RPSProfileBlock{
			RampUp:         time.Duration(core.GetInt(m, "RampUpMS", 0)) * time.Millisecond,
			StepDuration:   time.Duration(core.GetInt(m, "StepDurationMS", 0)) * time.Millisecond,
			ProfilePercent: core.GetFloat(m, "ProfilePercent", 100),
		})
	}

	return blocks
}

func runRPSProfileBlocks(ctx context.Context, blocks []RPSProfileBlock, profileScale *profileScaleState) {
	currentScale := 0.0
	profileScale.set(currentScale)

	for _, block := range blocks {
		targetScale := normalizeProfilePercent(block.ProfilePercent)

		if block.RampUp > 0 {
			start := time.Now()

			for {
				elapsed := time.Since(start)
				if elapsed >= block.RampUp {
					break
				}

				progress := float64(elapsed) / float64(block.RampUp)
				profileScale.set(currentScale + (targetScale-currentScale)*progress)

				waitStep := 100 * time.Millisecond
				remaining := block.RampUp - elapsed
				if remaining < waitStep {
					waitStep = remaining
				}

				if !waitForDuration(ctx, waitStep) {
					return
				}
			}
		}

		profileScale.set(targetScale)
		if !waitForDuration(ctx, block.StepDuration) {
			return
		}

		currentScale = targetScale
	}
}

func normalizeProfilePercent(percent float64) float64 {
	if percent < 0 {
		return 0
	}
	if percent > 1 {
		return percent / 100.0
	}
	return percent
}

func waitForDuration(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
