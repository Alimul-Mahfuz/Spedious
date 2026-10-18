package output

import (
	"bytes"
	"testing"
	"time"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestConsoleWriter_PrintSummary(t *testing.T) {
	buf := new(bytes.Buffer)
	cw := NewConsoleWriter(buf)

	snap := &metrics.Snapshot{
		TotalRequests:   100,
		TotalErrors:     2,
		TotalIterations: 100,
		Duration:        5 * time.Second,
		P50:             15.2,
		P95:             28.4,
		P99:             35.1,
		Mean:            16.0,
		Min:             5.0,
		Max:             45.0,
		BytesSent:       1024 * 10,
		BytesReceived:   1024 * 50,
	}

	thresholds := []check.Result{
		{Rule: "http_req_duration: p95 < 50ms", Passed: true},
		{Rule: "error_rate: rate < 0.01", Passed: false, Message: "error rate was 0.02"},
	}

	cw.PrintSummary(snap, false, thresholds)

	output := buf.String()
	assert.Contains(t, output, "TEST RESULTS SUMMARY")
	assert.Contains(t, output, "STATUS: FAILED")
	assert.Contains(t, output, "http_req_duration: p95 < 50ms")
	assert.Contains(t, output, "error_rate: rate < 0.01")
}
