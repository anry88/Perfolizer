package core

// Parameter represents a key-value pair for test parameterization.
const (
	ParamTypeStatic = "Static"
	ParamTypeRegexp = "Regexp"
	ParamTypeJSON   = "JSON"
)

// IsExtractor returns true if the parameter type is Regexp or JSON
func (p Parameter) IsExtractor() bool {
	return p.Type == ParamTypeRegexp || p.Type == ParamTypeJSON
}

type Parameter struct {
	ID         string
	Name       string
	Type       string // Static, Regexp, etc.
	Value      string // For Static: value, for others: default/fallback
	Expression string // Regex for Regexp, JsonPath, etc.
}
