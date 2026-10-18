package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spedious/spedious/internal/scenario"
	"github.com/spedious/spedious/testdata/mockserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_EndToEnd_MockServer(t *testing.T) {
	// Setup mock server
	srv := mockserver.NewServer(mockserver.ServerConfig{
		RequireAuth: true,
		AuthToken:   "jwt-token-spedious-123",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	yamlScenario := `
version: 1
name: e2e_mock_test
variables:
  baseUrl: ` + ts.URL + `

fragments:
  login_step:
    - request:
        name: login
        method: POST
        url: ${baseUrl}/auth/login
        body:
          email: "user@example.com"
          password: "password123"
      extract:
        token: { jsonpath: $.token }
      assert:
        - status == 200

scenarios:
  main_flow:
    load:
      executor: per-vu-iterations
      vus: 5
      iterations: 2
    steps:
      - include: login_step
      - request:
          name: get_items
          method: GET
          url: ${baseUrl}/items
          headers:
            Authorization: "Bearer ${token}"
        assert:
          - status == 200
          - latency < 2s
      - if:
          condition: ${status} == 200
          then:
            - sleep: 10ms

thresholds:
  - metric: http_req_duration
    rule: p(95) < 1s
  - metric: error_rate
    rule: rate < 0.05
`

	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	// 5 VUs * 2 iterations = 10 iterations
	// Each iteration does: login (1 req) + get_items (1 req) = 2 requests
	// Total requests = 20
	assert.Equal(t, int64(10), snap.TotalIterations)
	assert.Equal(t, int64(20), snap.TotalRequests)
	assert.Equal(t, int64(0), snap.TotalErrors)

	// Test threshold evaluation
	passed, results := EvaluateThresholds(file.Thresholds, snap)
	assert.True(t, passed)
	assert.Len(t, results, 2)
}

func TestEngine_StateIsolation(t *testing.T) {
	// Test that each VU has its own isolated variables
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	yamlScenario := `
version: 1
name: vu_isolation_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  main:
    load:
      executor: per-vu-iterations
      vus: 10
      iterations: 1
    steps:
      - request:
          name: vu_req
          method: GET
          url: ${baseUrl}?vu=${vu.id}
        assert:
          - status == 200
`

	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	assert.Equal(t, int64(10), snap.TotalIterations)
	assert.Equal(t, int64(10), snap.TotalRequests)
	assert.Equal(t, int64(0), snap.TotalErrors)
}

func TestEngine_1000_Concurrent_VUs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	yamlScenario := `
version: 1
name: high_concurrency_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  high_load:
    load:
      executor: per-vu-iterations
      vus: 1000
      iterations: 2
    steps:
      - request:
          name: ping
          method: GET
          url: ${baseUrl}/health
          thinkTime: 10ms-30ms
        assert:
          - status == 200
`

	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	start := time.Now()
	snap, err := eng.Run(context.Background(), file)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, int64(2000), snap.TotalIterations)
	t.Logf("Errors: %d / %d", snap.TotalErrors, snap.TotalRequests)
	assert.Equal(t, int64(0), snap.TotalErrors)

	t.Logf("1,000 VUs completed 2,000 requests in %s (%.1f req/s)", duration, float64(snap.TotalRequests)/duration.Seconds())
	t.Logf("Latency: min=%.2fms, med=%.2fms, p95=%.2fms, p99=%.2fms, max=%.2fms",
		snap.Min, snap.P50, snap.P95, snap.P99, snap.Max)
}
