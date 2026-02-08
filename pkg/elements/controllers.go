package elements

import (
	"perfolizer/pkg/core"
	"time"
)

func init() {
	core.RegisterFactory("LoopController", func(name string, props map[string]interface{}) core.TestElement {
		return &LoopController{
			BaseElement: core.NewBaseElement(name),
			Loops:       core.GetInt(props, "Loops", 1),
		}
	})
	core.RegisterFactory("IfController", func(name string, props map[string]interface{}) core.TestElement {
		return &IfController{
			BaseElement: core.NewBaseElement(name),
			Condition:   func(ctx *core.Context) bool { return true }, // Scripting not supported in JSON yet
		}
	})
	core.RegisterFactory("PauseController", func(name string, props map[string]interface{}) core.TestElement {
		return &PauseController{
			BaseElement: core.NewBaseElement(name),
			Duration:    time.Duration(core.GetInt(props, "DurationMS", 1000)) * time.Millisecond,
		}
	})
}

// ... LoopController methods ...

func (l *LoopController) GetType() string {
	return "LoopController"
}

func (l *LoopController) GetProps() map[string]interface{} {
	return map[string]interface{}{
		"Loops": l.Loops,
	}
}

// ... IfController methods ...

func (c *IfController) GetType() string {
	return "IfController"
}

func (c *IfController) GetProps() map[string]interface{} {
	return map[string]interface{}{}
}

// ... PauseController methods ...

func (p *PauseController) GetType() string {
	return "PauseController"
}

func (p *PauseController) GetProps() map[string]interface{} {
	return map[string]interface{}{
		"DurationMS": p.Duration.Milliseconds(),
	}
}

type LoopController struct {
	core.BaseElement
	Loops int // -1 for infinite
	Count int // Runtime counter
}

func NewLoopController(name string, loops int) *LoopController {
	l := &LoopController{
		BaseElement: core.NewBaseElement(name),
		Loops:       loops,
	}
	return l
}

func (l *LoopController) Clone() core.TestElement {
	// Deep copy if needed, for now shallow copy of struct
	newL := *l
	newL.BaseElement = core.NewBaseElement(l.Name()) // New ID
	// TODO: Clone children
	return &newL
}

func (l *LoopController) Next(ctx *core.Context) core.TestElement {
	// In a real implementation, 'Next' logic is complex stateless or stateful?
	// The JMeter model is: Controller returns the next Sampler.
	// But our Execute() model might be simpler for Go:
	// Just Execute children in a loop.
	return nil
}

// Since we defined Executable for Samplers, we might want Controllers to be Executable too if they just run children?
// The interface `Controller` had `Next()`. Let's refine the core engine.
// JMeter Style: Tree traversal.
// Simpler Go Style: Recursive Create/Execute.
// Let's make Controllers implement `Executable` and just run their logic.

func (l *LoopController) Execute(ctx *core.Context) error {
	for i := 0; l.Loops == -1 || i < l.Loops; i++ {
		// Checks context for stop signal
		if ctx.Err() != nil {
			return ctx.Err()
		}

		for _, child := range l.GetChildren() {
			if !child.Enabled() {
				continue
			}
			if exec, ok := child.(core.Executable); ok {
				if err := exec.Execute(ctx); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// --- If Controller ---

type IfController struct {
	core.BaseElement
	Condition func(ctx *core.Context) bool
}

func NewIfController(name string, condition func(ctx *core.Context) bool) *IfController {
	return &IfController{
		BaseElement: core.NewBaseElement(name),
		Condition:   condition,
	}
}

func (c *IfController) Clone() core.TestElement {
	newC := *c
	newC.BaseElement = core.NewBaseElement(c.Name())
	return &newC
}

func (c *IfController) Execute(ctx *core.Context) error {
	if c.Condition(ctx) {
		for _, child := range c.GetChildren() {
			if !child.Enabled() {
				continue
			}
			if exec, ok := child.(core.Executable); ok {
				if err := exec.Execute(ctx); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// --- Pause Controller ---

type PauseController struct {
	core.BaseElement
	Duration time.Duration
}

func NewPauseController(name string, d time.Duration) *PauseController {
	return &PauseController{
		BaseElement: core.NewBaseElement(name),
		Duration:    d,
	}
}

func (p *PauseController) Clone() core.TestElement {
	newP := *p
	newP.BaseElement = core.NewBaseElement(p.Name())
	return &newP
}

func (p *PauseController) Execute(ctx *core.Context) error {
	select {
	case <-time.After(p.Duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
