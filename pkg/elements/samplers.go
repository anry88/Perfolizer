package elements

import (
	"bytes"
	"io"
	"log"
	"math"
	"net/http"
	"perfolizer/pkg/core"
	"regexp"
	"sync"
	"sync/atomic"
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
			ExtractVars: core.GetStringSlice(props, "ExtractVars"),
			Body:        core.GetString(props, "Body", ""),
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
		"Url":         h.Url,
		"Method":      h.Method,
		"TargetRPS":   h.TargetRPS,
		"ExtractVars": h.ExtractVars,
		"Body":        h.Body,
	}
}

func (h *HttpSampler) Clone() core.TestElement {
	newH := *h
	newH.BaseElement = core.NewBaseElement(h.Name())
	if h.ExtractVars != nil {
		newH.ExtractVars = make([]string, len(h.ExtractVars))
		copy(newH.ExtractVars, h.ExtractVars)
	}
	return &newH
}

func (h *HttpSampler) Execute(ctx *core.Context) error {
	// 0. Rate Limiting (Per Sampler)

	// Determine effective RPS base
	baseRPS := h.TargetRPS
	if baseRPS == 0 {
		if val, ok := ctx.GetVar("DefaultRPS").(float64); ok {
			baseRPS = val
		}
	}

	profileScale := getProfileScale(ctx)
	targetRPS := baseRPS * profileScale

	// In RPS Thread Group blocks, profileScale can intentionally be zero.
	// If sampler has a base profile, skip execution in that case.
	if baseRPS > 0 && targetRPS <= 0 {
		return nil
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
	// Substitute variables
	url := ctx.Substitute(h.Url)
	method := ctx.Substitute(h.Method)
	body := ctx.Substitute(h.Body)

	// Debug substitution
	log.Printf("Debug: Sampler %q Request: %s %s", h.Name(), method, url)

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
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

		var respBodyBytes []byte
		// Read body for variable extraction if needed, otherwise discard
		if len(h.ExtractVars) > 0 {
			respBodyBytes, _ = io.ReadAll(resp.Body)
			result.BytesReceived = int64(len(respBodyBytes))
		} else {
			written, _ := io.Copy(io.Discard, resp.Body)
			result.BytesReceived = written
		}

		// Parameter Extraction
		if len(h.ExtractVars) > 0 {
			respBody := string(respBodyBytes)

			for _, varName := range h.ExtractVars {
				// Find parameter definition
				if param, ok := ctx.GetParameterDefinition(varName); ok {
					// Debug: Log extraction attempt
					log.Printf("Debug: Sampler %q extracting param %q (Type=%s)", h.Name(), varName, param.Type)

					if param.Type == core.ParamTypeRegexp {
						if param.Expression == "" {
							// Config Error or User mistake: Expression empty.
							log.Printf("Debug: Param %q has empty expression, using Value as default", varName)
							if param.Value != "" {
								ctx.SetVar(varName, param.Value)
							}
							continue
						}

						re, err := regexp.Compile(param.Expression)
						if err == nil {
							matches := re.FindStringSubmatch(respBody)
							if len(matches) > 1 {
								log.Printf("Debug: Extracted %s=%q", varName, matches[1])
								ctx.SetVar(varName, matches[1])
							} else if len(matches) == 1 {
								log.Printf("Debug: Extracted %s=%q", varName, matches[0])
								ctx.SetVar(varName, matches[0])
							} else {
								// No match, use default/fallback
								log.Printf("Debug: No match for %s, using default=%q", varName, param.Value)
								if param.Value != "" {
									ctx.SetVar(varName, param.Value)
								}
							}
						} else {
							log.Printf("Error: Invalid regex for %s: %v", varName, err)
						}
					} else if param.Type == core.ParamTypeJSON {
						if param.Expression == "" {
							log.Printf("Debug: Param %q has empty JSON path, using Value as default", varName)
							if param.Value != "" {
								ctx.SetVar(varName, param.Value)
							}
							continue
						}

						// Simple JSON path extraction using encoding/json
						extractedValue := ExtractJSONPathSimple(respBody, param.Expression)
						if extractedValue != "" {
							log.Printf("Debug: Extracted %s=%q from JSON path %q", varName, extractedValue, param.Expression)
							ctx.SetVar(varName, extractedValue)
						} else {
							log.Printf("Debug: No value found for JSON path %q, using default=%q", param.Expression, param.Value)
							if param.Value != "" {
								ctx.SetVar(varName, param.Value)
							}
						}
					}
				} else {
					log.Printf("Warning: Parameter definition for %q not found", varName)
				}
			}
		}
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

type profileScaleState struct {
	bits atomic.Uint64
}

func newProfileScaleState(initial float64) *profileScaleState {
	s := &profileScaleState{}
	s.set(initial)
	return s
}

func (s *profileScaleState) set(v float64) {
	if v < 0 {
		v = 0
	}
	s.bits.Store(math.Float64bits(v))
}

func (s *profileScaleState) get() float64 {
	return math.Float64frombits(s.bits.Load())
}

func getProfileScale(ctx *core.Context) float64 {
	val := ctx.GetVar("RPSProfileScale")
	switch v := val.(type) {
	case *profileScaleState:
		if v == nil {
			return 1
		}
		return v.get()
	case float64:
		if v < 0 {
			return 0
		}
		return v
	default:
		return 1
	}
}

// HttpSampler executes an HTTP request
type HttpSampler struct {
	core.BaseElement
	Url         string
	Method      string
	Body        string
	TargetRPS   float64  // 0 means unlimited/thread group default
	ExtractVars []string // Parameters to extract from response
}
