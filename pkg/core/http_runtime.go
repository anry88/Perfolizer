package core

import (
	"context"
	"net"
	"net/http"
	"time"
)

const (
	DefaultHTTPRequestTimeout      = 5 * time.Second
	defaultHTTPDialTimeout         = 10 * time.Second
	defaultHTTPKeepAlive           = 30 * time.Second
	defaultHTTPIdleConnTimeout     = 90 * time.Second
	defaultHTTPTLSHandshakeTimeout = 10 * time.Second
	defaultHTTPResponseHeaderWait  = 30 * time.Second
	defaultHTTPExpectContinueWait  = 1 * time.Second
	defaultHTTPMaxIdleConns        = 100
	defaultHTTPMaxIdleConnsPerHost = 100
)

type HTTPRuntime struct {
	Client         *http.Client
	RequestTimeout time.Duration
}

type HTTPRuntimeOptions struct {
	RequestTimeout        time.Duration
	DisableKeepAlives     bool
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	ExpectContinueTimeout time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

func DefaultHTTPRuntimeOptions() HTTPRuntimeOptions {
	return HTTPRuntimeOptions{
		RequestTimeout:        DefaultHTTPRequestTimeout,
		DialTimeout:           defaultHTTPDialTimeout,
		KeepAlive:             defaultHTTPKeepAlive,
		IdleConnTimeout:       defaultHTTPIdleConnTimeout,
		TLSHandshakeTimeout:   defaultHTTPTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultHTTPResponseHeaderWait,
		ExpectContinueTimeout: defaultHTTPExpectContinueWait,
		MaxIdleConns:          defaultHTTPMaxIdleConns,
		MaxIdleConnsPerHost:   defaultHTTPMaxIdleConnsPerHost,
	}
}

func NewHTTPRuntime(options HTTPRuntimeOptions) *HTTPRuntime {
	options = options.withDefaults()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   options.DialTimeout,
			KeepAlive: options.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     options.DisableKeepAlives,
		MaxIdleConns:          options.MaxIdleConns,
		MaxIdleConnsPerHost:   options.MaxIdleConnsPerHost,
		IdleConnTimeout:       options.IdleConnTimeout,
		TLSHandshakeTimeout:   options.TLSHandshakeTimeout,
		ResponseHeaderTimeout: options.ResponseHeaderTimeout,
		ExpectContinueTimeout: options.ExpectContinueTimeout,
	}

	return &HTTPRuntime{
		Client: &http.Client{
			Transport: transport,
		},
		RequestTimeout: options.RequestTimeout,
	}
}

func (o HTTPRuntimeOptions) withDefaults() HTTPRuntimeOptions {
	defaults := DefaultHTTPRuntimeOptions()

	if o.RequestTimeout <= 0 {
		o.RequestTimeout = defaults.RequestTimeout
	}
	if o.DialTimeout <= 0 {
		o.DialTimeout = defaults.DialTimeout
	}
	if o.KeepAlive <= 0 {
		o.KeepAlive = defaults.KeepAlive
	}
	if o.IdleConnTimeout <= 0 {
		o.IdleConnTimeout = defaults.IdleConnTimeout
	}
	if o.TLSHandshakeTimeout <= 0 {
		o.TLSHandshakeTimeout = defaults.TLSHandshakeTimeout
	}
	if o.ResponseHeaderTimeout <= 0 {
		o.ResponseHeaderTimeout = o.RequestTimeout
	}
	if o.ExpectContinueTimeout <= 0 {
		o.ExpectContinueTimeout = defaults.ExpectContinueTimeout
	}
	if o.MaxIdleConns <= 0 {
		o.MaxIdleConns = defaults.MaxIdleConns
	}
	if o.MaxIdleConnsPerHost <= 0 {
		o.MaxIdleConnsPerHost = defaults.MaxIdleConnsPerHost
	}

	return o
}

func (r *HTTPRuntime) ClientOrDefault() *http.Client {
	if r != nil && r.Client != nil {
		return r.Client
	}
	return http.DefaultClient
}

func (r *HTTPRuntime) EffectiveTimeout(override time.Duration) time.Duration {
	if override > 0 {
		return override
	}
	if r != nil && r.RequestTimeout > 0 {
		return r.RequestTimeout
	}
	return DefaultHTTPRequestTimeout
}

type httpRuntimeContextKey struct{}

func WithHTTPRuntime(ctx context.Context, runtime *HTTPRuntime) context.Context {
	if ctx == nil || runtime == nil {
		return ctx
	}
	return context.WithValue(ctx, httpRuntimeContextKey{}, runtime)
}

func HTTPRuntimeFromContext(ctx context.Context) *HTTPRuntime {
	if ctx == nil {
		return nil
	}
	runtime, _ := ctx.Value(httpRuntimeContextKey{}).(*HTTPRuntime)
	return runtime
}

func (c *Context) SetHTTPRuntime(runtime *HTTPRuntime) {
	if c == nil || runtime == nil {
		return
	}
	c.Context = WithHTTPRuntime(c.Context, runtime)
}

func (c *Context) HTTPRuntime() *HTTPRuntime {
	if c == nil {
		return nil
	}
	return HTTPRuntimeFromContext(c.Context)
}

func (c *Context) HTTPClient() *http.Client {
	return c.HTTPRuntime().ClientOrDefault()
}

func (c *Context) EffectiveHTTPRequestTimeout(override time.Duration) time.Duration {
	return c.HTTPRuntime().EffectiveTimeout(override)
}
