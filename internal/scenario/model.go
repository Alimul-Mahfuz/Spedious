package scenario

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Duration is a wrapper around time.Duration that supports YAML/JSON unmarshaling from strings (e.g. "500ms", "30s", "1m").
type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var i int64
	if err := unmarshal(&i); err == nil {
		*d = Duration(time.Duration(i) * time.Millisecond)
		return nil
	}
	return fmt.Errorf("duration must be a string like '500ms' or an integer milliseconds")
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var i int64
	if err := json.Unmarshal(data, &i); err == nil {
		*d = Duration(time.Duration(i) * time.Millisecond)
		return nil
	}
	return fmt.Errorf("duration must be a string or number")
}

// File represents the top-level scenario file format (v1).
type File struct {
	Version    int                           `yaml:"version" json:"version"`
	Name       string                        `yaml:"name" json:"name"`
	Variables  map[string]any                `yaml:"variables" json:"variables"`
	Data       map[string]DataSourceConfig   `yaml:"data" json:"data"`
	Fragments  map[string][]Step             `yaml:"fragments" json:"fragments"`
	Scenarios  map[string]ScenarioDefinition `yaml:"scenarios" json:"scenarios"`
	Thresholds []Threshold                   `yaml:"thresholds" json:"thresholds"`
}

// DataSourceConfig configures a feeder data source.
type DataSourceConfig struct {
	Type     string `yaml:"type" json:"type"`         // csv, json
	File     string `yaml:"file" json:"file"`         // path to file
	Strategy string `yaml:"strategy" json:"strategy"` // unique, sequential, random
}

// ScenarioDefinition defines a workload workflow and its execution load profile.
type ScenarioDefinition struct {
	Weight float64     `yaml:"weight" json:"weight"`
	Load   LoadProfile `yaml:"load" json:"load"`
	Steps  []Step      `yaml:"steps" json:"steps"`
}

// LoadProfile specifies the executor and its parameters.
type LoadProfile struct {
	Executor   string        `yaml:"executor" json:"executor"` // constant-users, per-vu-iterations, ramping-users, etc.
	VUs        int           `yaml:"vus" json:"vus"`
	Duration   Duration      `yaml:"duration" json:"duration"`
	Iterations int           `yaml:"iterations" json:"iterations"`
	Stages     []Stage       `yaml:"stages" json:"stages"`
	Rate       int           `yaml:"rate" json:"rate"`
	TimeUnit   Duration      `yaml:"timeUnit" json:"timeUnit"`
	MaxVUs     int           `yaml:"maxVUs" json:"maxVUs"`
}

// Stage defines a target VU or rate over a duration for ramping executors.
type Stage struct {
	Duration Duration `yaml:"duration" json:"duration"`
	Target   int      `yaml:"target" json:"target"`
}

// Step represents a single unit of execution.
type Step struct {
	Include string       `yaml:"include,omitempty" json:"include,omitempty"`
	Request *RequestStep `yaml:"request,omitempty" json:"request,omitempty"`
	Sleep   Duration     `yaml:"sleep,omitempty" json:"sleep,omitempty"`
	If      *IfStep      `yaml:"if,omitempty" json:"if,omitempty"`
	Loop    *LoopStep    `yaml:"loop,omitempty" json:"loop,omitempty"`
	Group   *GroupStep   `yaml:"group,omitempty" json:"group,omitempty"`
	Sync    *SyncStep    `yaml:"sync,omitempty" json:"sync,omitempty"`
}

// AuthConfig provides helper authentication configurations.
type AuthConfig struct {
	Type     string `yaml:"type" json:"type"` // bearer, basic
	Token    string `yaml:"token,omitempty" json:"token,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

// ExtractorConfig defines an extraction rule.
type ExtractorConfig struct {
	JSONPath string `yaml:"jsonpath,omitempty" json:"jsonpath,omitempty"`
	Regex    string `yaml:"regex,omitempty" json:"regex,omitempty"`
	Header   string `yaml:"header,omitempty" json:"header,omitempty"`
	Status   bool   `yaml:"status,omitempty" json:"status,omitempty"`
	Latency  bool   `yaml:"latency,omitempty" json:"latency,omitempty"`
}

// RequestStep configures an HTTP/protocol request step.
type RequestStep struct {
	Name            string                     `yaml:"name" json:"name"`
	Method          string                     `yaml:"method" json:"method"`
	URL             string                     `yaml:"url" json:"url"`
	Headers         map[string]string          `yaml:"headers,omitempty" json:"headers,omitempty"`
	Query           map[string]string          `yaml:"query,omitempty" json:"query,omitempty"`
	Body            any                        `yaml:"body,omitempty" json:"body,omitempty"`
	Timeout         Duration                   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Auth            *AuthConfig                `yaml:"auth,omitempty" json:"auth,omitempty"`
	Extract         map[string]ExtractorConfig `yaml:"extract,omitempty" json:"extract,omitempty"`
	Assert          []string                   `yaml:"assert,omitempty" json:"assert,omitempty"`
	ThinkTime       string                     `yaml:"thinkTime,omitempty" json:"thinkTime,omitempty"` // e.g. "1s-3s" or "500ms"
	OnFailure       string                     `yaml:"onFailure,omitempty" json:"onFailure,omitempty"` // abort_iteration, continue, abort_test
	ExpectedStatus  []int                      `yaml:"expectedStatus,omitempty" json:"expectedStatus,omitempty"`
}

// IfStep represents a conditional execution branch.
type IfStep struct {
	Condition string `yaml:"condition" json:"condition"`
	Then      []Step `yaml:"then" json:"then"`
	Else      []Step `yaml:"else,omitempty" json:"else,omitempty"`
}

// LoopStep represents count or conditional looping.
type LoopStep struct {
	Count     int    `yaml:"count,omitempty" json:"count,omitempty"`
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`
	Steps     []Step `yaml:"steps" json:"steps"`
}

// GroupStep groups steps for transaction measurements.
type GroupStep struct {
	Name  string `yaml:"name" json:"name"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// SyncStep represents a barrier sync point.
type SyncStep struct {
	Name    string   `yaml:"name" json:"name"`
	WaitFor int      `yaml:"waitFor" json:"waitFor"`
	Timeout Duration `yaml:"timeout" json:"timeout"`
}

// Threshold represents a pass/fail metric rule.
type Threshold struct {
	Metric string `yaml:"metric" json:"metric"`
	Rule   string `yaml:"rule" json:"rule"`
	Abort  bool   `yaml:"abort,omitempty" json:"abort,omitempty"`
}

// Valid executors
const (
	ExecutorConstantUsers       = "constant-users"
	ExecutorRampingUsers        = "ramping-users"
	ExecutorPerVUIterations     = "per-vu-iterations"
	ExecutorSharedIterations    = "shared-iterations"
	ExecutorConstantArrivalRate = "constant-arrival-rate"
	ExecutorRampingArrivalRate  = "ramping-arrival-rate"
)

// Valid failure policies
const (
	OnFailureAbortIteration = "abort_iteration"
	OnFailureContinue       = "continue"
	OnFailureAbortTest      = "abort_test"
)

// IsValidExecutor checks if the executor string is one of the supported executors.
func IsValidExecutor(name string) bool {
	switch strings.ToLower(name) {
	case ExecutorConstantUsers,
		ExecutorRampingUsers,
		ExecutorPerVUIterations,
		ExecutorSharedIterations,
		ExecutorConstantArrivalRate,
		ExecutorRampingArrivalRate:
		return true
	default:
		return false
	}
}
