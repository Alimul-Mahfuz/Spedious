package protocol

import (
	"context"
	"net/http"
	"time"

	"github.com/spedious/spedious/internal/scenario"
	"github.com/spedious/spedious/internal/vars"
)

// RequestTiming records detailed breakdown of request execution times.
type RequestTiming struct {
	Blocked       time.Duration // Time waiting for connection acquisition
	Connecting    time.Duration // TCP dial time
	TLSHandshake  time.Duration // TLS negotiation time
	Sending       time.Duration // Time writing request
	Waiting       time.Duration // Time to first byte (TTFB)
	Receiving     time.Duration // Time downloading response body
	TotalDuration time.Duration // End-to-end duration
}

// ExecutionResult captures the output of a protocol step execution.
type ExecutionResult struct {
	StepName       string
	Method         string
	URL            string
	StatusCode     int
	Headers        http.Header
	Body           []byte
	Timing         RequestTiming
	BytesSent      int64
	BytesReceived  int64
	Error          error
	AssertionFails int
}

// VUContext carries per-VU state into protocol execution.
type VUContext struct {
	VUID       int
	Iteration  int
	Vars       *vars.Store
	HTTPClient *http.Client
	CookieJar  http.CookieJar
}

// Protocol executes a protocol-specific request step.
type Protocol interface {
	Name() string
	Execute(ctx context.Context, vuCtx *VUContext, step *scenario.RequestStep) (*ExecutionResult, error)
}
