package core

type DebugHTTPRequest struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

type DebugHTTPResponse struct {
	StatusCode int                 `json:"status_code"`
	Status     string              `json:"status"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       string              `json:"body,omitempty"`
}

type DebugHTTPExchange struct {
	Request               DebugHTTPRequest   `json:"request"`
	Response              *DebugHTTPResponse `json:"response,omitempty"`
	DurationMilliseconds  int64              `json:"duration_ms"`
	Error                 string             `json:"error,omitempty"`
	RequestBodyTruncated  bool               `json:"request_body_truncated,omitempty"`
	ResponseBodyTruncated bool               `json:"response_body_truncated,omitempty"`
}
