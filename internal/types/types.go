// Package types defines the shared contracts used across config parsing,
// the JS parser boundary, persistence, and the HTTP API.
package types

// UsageStatus categorizes the outcome of a single usage query.
type UsageStatus string

const (
	StatusOK      UsageStatus = "ok"
	StatusExpired UsageStatus = "expired"
	StatusError   UsageStatus = "error"
)

// UsageTier is a single usage window / limit. zai only fills Utilization;
// future services with absolute quotas may fill Used/Limit/Unit instead.
type UsageTier struct {
	Name        string  `json:"name"`
	Utilization float64 `json:"utilization"`
	Used        float64 `json:"used,omitempty"`
	Limit       float64 `json:"limit,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	ResetsAt    string  `json:"resetsAt,omitempty"`
}

// ParseContext is passed into a parser alongside the parsed JSON body.
// It carries everything a parser needs that is not in the body itself.
type ParseContext struct {
	HTTPStatus int            `json:"httpStatus"`
	RawBody    string         `json:"rawBody"`
	Endpoint   EndpointPublic `json:"endpoint"`
}

// UsageResult is what a parser produces and what is persisted as the DB payload.
type UsageResult struct {
	Status    UsageStatus `json:"status"`
	Message   string      `json:"message,omitempty"`
	Tiers     []UsageTier `json:"tiers"`
	Error     string      `json:"error,omitempty"`
	QueriedAt int64       `json:"queriedAt"`
}

// UsageRecord is a stored / returned record: UsageResult plus identity and
// the time of the most recent refresh attempt.
type UsageRecord struct {
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updatedAt"`
	UsageResult
}

// EndpointConfig is a single entry in endpoints.yaml. It is server-side only
// and contains the raw headers (including Authorization secrets).
type EndpointConfig struct {
	Name      string            `yaml:"name"`
	URL       string            `yaml:"url"`
	Methods   string            `yaml:"methods"`
	Headers   map[string]string `yaml:"headers"`
	Parser    string            `yaml:"parser,omitempty"`
	TimeoutMs int               `yaml:"timeoutMs,omitempty"`
}

// Public returns a sanitized view (no headers) suitable for the parser context.
func (e EndpointConfig) Public() EndpointPublic {
	return EndpointPublic{Name: e.Name, URL: e.URL, Methods: e.Methods}
}

// ParserName returns the parser identifier: explicit Parser, else Name.
func (e EndpointConfig) ParserName() string {
	if e.Parser != "" {
		return e.Parser
	}
	return e.Name
}

// EndpointPublic is the desensitized endpoint view exposed to parsers.
type EndpointPublic struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Methods string `json:"methods"`
}

// EndpointsFile is the top-level shape of endpoints.yaml.
type EndpointsFile struct {
	Endpoints []EndpointConfig `yaml:"endpoints"`
}
