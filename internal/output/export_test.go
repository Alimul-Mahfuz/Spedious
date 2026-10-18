package output

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSONSummary(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "summary.json")

	snap := &metrics.Snapshot{
		TotalRequests:   50,
		TotalErrors:     1,
		TotalIterations: 50,
		Duration:        2 * time.Second,
		P95:             25.4,
		WaitingDuration: metrics.MetricStats{P95: 15.0},
	}

	thresholds := []check.Result{
		{Rule: "http_req_duration: p(95) < 100ms", Passed: true},
	}

	err := WriteJSONSummary(jsonPath, snap, true, thresholds)
	require.NoError(t, err)

	data, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"total_requests": 50`)
	assert.Contains(t, string(data), `"passed": true`)
	assert.Contains(t, string(data), `http_req_duration: p(95) < 100ms`)
}

func TestWriteCSVTimeSeries(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "timeseries.csv")

	samples := []metrics.TimeSeriesSample{
		{
			Timestamp:   time.Now(),
			ElapsedSec:  1.0,
			ActiveVUs:   10,
			RPS:         100.0,
			Errors:      0,
			ErrorRate:   0.0,
			P95Duration: 12.5,
		},
		{
			Timestamp:   time.Now().Add(time.Second),
			ElapsedSec:  2.0,
			ActiveVUs:   10,
			RPS:         120.0,
			Errors:      1,
			ErrorRate:   0.005,
			P95Duration: 14.2,
		},
	}

	err := WriteCSVTimeSeries(csvPath, samples)
	require.NoError(t, err)

	data, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "timestamp,elapsed_sec,active_vus,rps,errors,error_rate,p95_duration_ms")
	assert.Contains(t, content, "1.00,10,100.0,0,0.0000,12.50")
	assert.Contains(t, content, "2.00,10,120.0,1,0.0050,14.20")
}
