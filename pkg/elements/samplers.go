package elements

import (
	"bytes"
	"io"
	"net/http"
	"perfolizer/pkg/core"
	"time"
)

type HttpSampler struct {
	core.BaseElement
	Url    string
	Method string
	Body   string
}

func NewHttpSampler(name, method, url string) *HttpSampler {
	return &HttpSampler{
		BaseElement: core.NewBaseElement(name),
		Method:      method,
		Url:         url,
	}
}

func (h *HttpSampler) Clone() core.TestElement {
	newH := *h
	newH.BaseElement = core.NewBaseElement(h.Name())
	return &newH
}

func (h *HttpSampler) Execute(ctx *core.Context) error {
	// 1. Prepare Request
	var bodyReader io.Reader
	if h.Body != "" {
		bodyReader = bytes.NewBufferString(h.Body)
	}

	req, err := http.NewRequest(h.Method, h.Url, bodyReader)
	if err != nil {
		return err // Or report error sample?
	}
	req = req.WithContext(ctx)

	// 2. Execute
	start := time.Now()
	resp, err := http.DefaultClient.Do(req) // TODO: Use custom client
	end := time.Now()

	// 3. Report Result
	result := &core.SampleResult{
		SamplerName: h.Name(),
		StartTime:   start,
		EndTime:     end,
		Latency:     end.Sub(start),
	}

	if err != nil {
		result.Error = err
		result.Success = false
	} else {
		defer resp.Body.Close()
		result.ResponseCode = resp.Status // "200 OK"
		result.Success = resp.StatusCode >= 200 && resp.StatusCode < 400
		
		// Read body size (limited)
		// We might want to drain body to reuse connection
		written, _ := io.Copy(io.Discard, resp.Body)
		result.BytesReceived = written
	}

	// Used mechanism to report up? 
	// The ctx doesn't specify a generic "Reporter". 
	// Maybe context should hold the Runner or ResultCollector?
	// For now let's assume we print or we passed a channel in Context variables (hacky but works for MVP).
	// Ideally ThreadGroup passing a 'Reporter' callback to Execute?
	// Let's add Reporter interface to Context or passed in Execute signature? 
	// For MVP, lets just log if no reporter is found.
	if reporter, ok := ctx.GetVar("Reporter").(core.Runner); ok {
		reporter.ReportResult(result)
	}

	return nil
}
