package core

import (
	"context"
	"sync"
	"time"
)

// Context holds the state for a single thread's execution.
// It wraps standard context.Context and adds Perfolizer specific state.
type Context struct {
	context.Context
	Variables map[string]interface{}
	ThreadID  int
	Iteration int
	mu        sync.RWMutex
}

func NewContext(parent context.Context, threadID int) *Context {
	return &Context{
		Context:   parent,
		ThreadID:  threadID,
		Variables: make(map[string]interface{}),
	}
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
