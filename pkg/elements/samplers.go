package elements

import (
	"bytes"
	"io"
	"net/http"
	"perfolizer/pkg/core"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

func init() {
	core.RegisterFactory("HttpSampler", func(name string, props map[string]interface{}) core.TestElement {
		return &HttpSampler{
			BaseElement: core.NewBaseElement(name),
			Url:         core.GetString(props, "Url", "http://localhost"),
			Method:      core.GetString(props, "Method", "GET"),
			TargetRPS:   core.GetFloat(props, "TargetRPS", 0),
		}
	})
}

func NewHttpSampler(name, method, url string) *HttpSampler {
	return &HttpSampler{
		BaseElement: core.NewBaseElement(name),
		Method:      method,
		Url:         url,
	}
}

func (h *HttpSampler) GetType() string {
	return "HttpSampler"
}

func (h *HttpSampler) GetProps() map[string]interface{} {
	return map[string]interface{}{
		"Url":       h.Url,
		"Method":    h.Method,
		"TargetRPS": h.TargetRPS,
	}
}

func (h *HttpSampler) Clone() core.TestElement {
	newH := *h
	newH.BaseElement = core.NewBaseElement(h.Name())
	newH.BaseElement = core.NewBaseElement(h.Name())
	return &newH
}

func (h *HttpSampler) Execute(ctx *core.Context) error {
	// 0. Rate Limiting (Per Sampler)

	// Determine effective RPS
	targetRPS := h.TargetRPS
	if targetRPS == 0 {
		if val, ok := ctx.GetVar("DefaultRPS").(float64); ok {
			targetRPS = val
		}
	}

	if targetRPS > 0 {
		key := "Limiter_" + h.ID()
		limiter := getOrCreateLimiter(ctx, key, targetRPS)

		// Check if target changed (dynamic update support)
		if float64(limiter.Limit()) != targetRPS {
			limiter.SetLimit(rate.Limit(targetRPS))
		}

		if nonBlocking, ok := ctx.GetVar("RPSNonBlocking").(bool); ok && nonBlocking {
			if !limiter.Allow() {
				return nil
			}
		} else {
			if err := limiter.Wait(ctx); err != nil {
				return err
			}
		}
	}

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
	if reporter, ok := ctx.GetVar("Reporter").(core.Runner); ok {
		reporter.ReportResult(result)
	}

	return nil
}

type limiterStore struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newLimiterStore() *limiterStore {
	return &limiterStore{
		limiters: make(map[string]*rate.Limiter),
	}
}

func (s *limiterStore) getOrCreate(key string, targetRPS float64) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limiter, ok := s.limiters[key]; ok {
		return limiter
	}

	limiter := rate.NewLimiter(rate.Limit(targetRPS), 1)
	s.limiters[key] = limiter
	return limiter
}

func getOrCreateLimiter(ctx *core.Context, key string, targetRPS float64) *rate.Limiter {
	if shared, ok := ctx.GetVar("SharedLimiterStore").(*limiterStore); ok && shared != nil {
		return shared.getOrCreate(key, targetRPS)
	}

	if val := ctx.GetVar(key); val != nil {
		return val.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rate.Limit(targetRPS), 1)
	ctx.SetVar(key, limiter)
	return limiter
}

// HttpSampler executes an HTTP request
type HttpSampler struct {
	core.BaseElement
	Url       string
	Method    string
	Body      string
	TargetRPS float64 // 0 means unlimited/thread group default
}
