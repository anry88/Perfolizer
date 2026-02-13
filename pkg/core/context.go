package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Context holds the state for a single thread's execution.
// It wraps standard context.Context and adds Perfolizer specific state.
type Context struct {
	context.Context
	Variables            map[string]interface{}
	ParameterDefinitions map[string]Parameter
	ThreadID             int
	Iteration            int
	mu                   sync.RWMutex
}

func NewContext(parent context.Context, threadID int) *Context {
	c := &Context{
		Context:              parent,
		ThreadID:             threadID,
		Variables:            make(map[string]interface{}),
		ParameterDefinitions: make(map[string]Parameter),
	}

	// If parent is also a *Context, copy definitions
	// (Note: stdlib context wrapping might hide it, so we check)
	// For now, assuming NewContext is called with a parent that might contain info or we inject it later.
	// Actually, usually we create a root context for the thread group.
	// If parent is *Context, we should probably inherit?
	if pCtx, ok := parent.(*Context); ok {
		pCtx.mu.RLock()
		for k, v := range pCtx.ParameterDefinitions {
			c.ParameterDefinitions[k] = v
		}
		// Also inherit variables? Usually yes for global vars.
		for k, v := range pCtx.Variables {
			c.Variables[k] = v
		}
		pCtx.mu.RUnlock()
	}

	return c
}

func (c *Context) SetVar(key string, val interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Variables[key] = val
}

func (c *Context) GetVar(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Variables[key]
}

func (c *Context) GetParameterDefinition(name string) (Parameter, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.ParameterDefinitions[name]
	return p, ok
}

// Substitute replaces ${var} with values from the context.
func (c *Context) Substitute(text string) string {
	if text == "" {
		return ""
	}

	// Fast path: no variables
	if !containsVar(text) {
		return text
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return expandVariables(text, c.Variables)
}

func containsVar(s string) bool {
	for i := 0; i < len(s)-3; i++ {
		if s[i] == '$' && s[i+1] == '{' {
			return true
		}
	}
	return false
}

func expandVariables(s string, vars map[string]interface{}) string {
	// Simple implementation, can be optimized
	// TODO: Use better parser if needed
	var result []byte
	i := 0
	for i < len(s) {
		if i < len(s)-3 && s[i] == '$' && s[i+1] == '{' {
			end := -1
			for j := i + 2; j < len(s); j++ {
				if s[j] == '}' {
					end = j
					break
				}
			}
			if end != -1 {
				key := s[i+2 : end]
				if val, ok := vars[key]; ok {
					strVal := fmt.Sprintf("%v", val)
					result = append(result, strVal...)
					i = end + 1
					continue
				}
			}
		}
		result = append(result, s[i])
		i++
	}
	return string(result)
}

// SampleResult holds the result of a sampler execution.
type SampleResult struct {
	SamplerName   string
	StartTime     time.Time
	EndTime       time.Time
	Latency       time.Duration
	ResponseCode  string
	Success       bool
	Error         error
	BytesReceived int64
}

func (s *SampleResult) Duration() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}
