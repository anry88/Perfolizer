package elements_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"perfolizer/pkg/core"
	"perfolizer/pkg/elements"
)

type runtimeProbe struct {
	timeout           time.Duration
	disableKeepAlives bool
}

type runtimeProbeElement struct {
	core.BaseElement
	seen chan runtimeProbe
}

func newRuntimeProbeElement(name string) *runtimeProbeElement {
	return &runtimeProbeElement{
		BaseElement: core.NewBaseElement(name),
		seen:        make(chan runtimeProbe, 1),
	}
}

func (e *runtimeProbeElement) Clone() core.TestElement {
	clone := *e
	clone.BaseElement = core.NewBaseElement(e.Name())
	clone.seen = make(chan runtimeProbe, 1)
	return &clone
}

func (e *runtimeProbeElement) Execute(ctx *core.Context) error {
	runtime := ctx.HTTPRuntime()
	if runtime == nil || runtime.Client == nil {
		return nil
	}

	transport, _ := runtime.Client.Transport.(*http.Transport)
	probe := runtimeProbe{timeout: runtime.RequestTimeout}
	if transport != nil {
		probe.disableKeepAlives = transport.DisableKeepAlives
	}

	select {
	case e.seen <- probe:
	default:
	}
	return nil
}

func TestSimpleThreadGroupInjectsHTTPRuntimeIntoChildren(t *testing.T) {
	tg := elements.NewSimpleThreadGroup("Simple", 1, 1)
	tg.HTTPRequestTimeout = 2300 * time.Millisecond
	tg.HTTPKeepAlive = false

	probe := newRuntimeProbeElement("Probe")
	tg.AddChild(probe)

	tg.Start(context.Background(), noopRunner{})

	select {
	case got := <-probe.seen:
		if got.timeout != 2300*time.Millisecond {
			t.Fatalf("expected timeout %v, got %v", 2300*time.Millisecond, got.timeout)
		}
		if !got.disableKeepAlives {
			t.Fatal("expected keep-alive=false to disable transport keep-alives")
		}
	case <-time.After(time.Second):
		t.Fatal("expected child execution to observe thread-group runtime")
	}
}

func TestRPSThreadGroupInjectsHTTPRuntimeIntoChildren(t *testing.T) {
	tg := elements.NewRPSThreadGroup("RPS", 10)
	tg.Users = 1
	tg.HTTPRequestTimeout = 4100 * time.Millisecond
	tg.HTTPKeepAlive = true
	tg.ProfileBlocks = []elements.RPSProfileBlock{
		{RampUp: 0, StepDuration: 25 * time.Millisecond, ProfilePercent: 100},
	}

	probe := newRuntimeProbeElement("Probe")
	tg.AddChild(probe)

	tg.Start(context.Background(), noopRunner{})

	select {
	case got := <-probe.seen:
		if got.timeout != 4100*time.Millisecond {
			t.Fatalf("expected timeout %v, got %v", 4100*time.Millisecond, got.timeout)
		}
		if got.disableKeepAlives {
			t.Fatal("expected keep-alive=true to preserve transport keep-alives")
		}
	case <-time.After(time.Second):
		t.Fatal("expected child execution to observe RPS thread-group runtime")
	}
}
