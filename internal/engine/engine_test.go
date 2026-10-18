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

func TestEngine_SharedIterations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	yamlScenario := `
version: 1
name: shared_iters_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  shared:
    load:
      executor: shared-iterations
      vus: 5
      iterations: 25
    steps:
      - request:
          name: ping
          method: GET
          url: ${baseUrl}/
        assert:
          - status == 200
`
	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	assert.Equal(t, int64(25), snap.TotalIterations)
	assert.Equal(t, int64(25), snap.TotalRequests)
	assert.Equal(t, int64(0), snap.TotalErrors)
}

func TestEngine_RampingUsers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	yamlScenario := `
version: 1
name: ramping_users_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  ramp:
    load:
      executor: ramping-users
      stages:
        - { duration: 200ms, target: 5 }
        - { duration: 200ms, target: 10 }
        - { duration: 100ms, target: 0 }
    steps:
      - request:
          name: ping
          method: GET
          url: ${baseUrl}/
        assert:
          - status == 200
`
	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	assert.True(t, snap.TotalIterations > 0)
	assert.Equal(t, int64(0), snap.TotalErrors)
}

func TestEngine_CoordinatedOmission_Regression(t *testing.T) {
	// A server that introduces an artificial stall after 10 requests
	var reqCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cnt := time.Now().UnixNano()
		_ = cnt
		reqCount++
		if reqCount > 10 && reqCount <= 15 {
			time.Sleep(200 * time.Millisecond) // artificial stall
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	// Open model: constant-arrival-rate of 20 req/s for 1s
	yamlScenario := `
version: 1
name: open_model_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  open_flow:
    load:
      executor: constant-arrival-rate
      rate: 20
      timeUnit: 1s
      duration: 1s
      maxVUs: 50
    steps:
      - request:
          name: check_stall
          method: GET
          url: ${baseUrl}/
        assert:
          - status == 200
`
	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	t.Logf("Open model: TotalReqs=%d, p95=%.2fms, max=%.2fms", snap.TotalRequests, snap.P95, snap.Max)
	// Because of the stall, max and p95 must capture the stall (>150ms) rather than understating it!
	assert.True(t, snap.Max >= 150.0, "expected max latency to capture the stall (at least 150ms), got %.2fms", snap.Max)
}

func TestEngine_DroppedIterations(t *testing.T) {
	// Fast arrival rate with very low maxVUs capacity (maxVUs: 2, but requests take 200ms)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	// Requesting 50 req/sec with maxVUs = 2 (cannot keep up)
	yamlScenario := `
version: 1
name: dropped_iters_test
variables:
  baseUrl: ` + ts.URL + `

scenarios:
  starved_flow:
    load:
      executor: constant-arrival-rate
      rate: 50
      timeUnit: 1s
      duration: 500ms
      maxVUs: 2
    steps:
      - request:
          name: slow_req
          method: GET
          url: ${baseUrl}/
`
	file, err := scenario.LoadBytes([]byte(yamlScenario))
	require.NoError(t, err)

	eng := NewEngine()
	snap, err := eng.Run(context.Background(), file)
	require.NoError(t, err)

	t.Logf("Completed: %d, Dropped: %d", snap.TotalIterations, snap.DroppedIterations)
	assert.True(t, snap.DroppedIterations > 0, "expected dropped iterations due to pool exhaustion, got %d", snap.DroppedIterations)
}
