package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spedious/spedious/internal/check"
	"github.com/spedious/spedious/internal/metrics"
	"github.com/spedious/spedious/internal/protocol"
	httpProto "github.com/spedious/spedious/internal/protocol/http"
	"github.com/spedious/spedious/internal/scenario"
	"github.com/spedious/spedious/internal/vars"
)

// ProgressCallback is invoked periodically with live metrics.
type ProgressCallback func(snap metrics.Snapshot)

// Engine runs scenario workloads using configured executors.
type Engine struct {
	protocol   *httpProto.HTTPProtocol
	collector  *metrics.Collector
	onProgress ProgressCallback
}

// NewEngine creates an initialized load testing engine.
func NewEngine() *Engine {
	proto := httpProto.NewHTTPProtocol()
	col := metrics.NewCollector()
	return &Engine{
		protocol:  proto,
		collector: col,
	}
}

// SetProgressCallback registers a function to receive periodic progress snapshots.
func (e *Engine) SetProgressCallback(cb ProgressCallback) {
	e.onProgress = cb
}

// Collector returns the engine's metrics collector.
func (e *Engine) Collector() *metrics.Collector {
	return e.collector
}

// Run executes the entire scenario file.
func (e *Engine) Run(ctx context.Context, f *scenario.File) (*metrics.Snapshot, error) {
	e.collector.Reset()

	baseVars := vars.NewStore(f.Variables)

	// Start progress reporter goroutine if callback is set
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	if e.onProgress != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					e.onProgress(e.collector.Snapshot())
				case <-progressCtx.Done():
					return
				}
			}
		}()
	}

	// Run all defined scenarios
	var wg sync.WaitGroup
	errCh := make(chan error, len(f.Scenarios))

	for scName, sc := range f.Scenarios {
		wg.Add(1)
		go func(name string, def scenario.ScenarioDefinition) {
			defer wg.Done()
			var err error
			switch def.Load.Executor {
			case scenario.ExecutorConstantUsers:
				err = e.runConstantUsers(ctx, def, baseVars)
			case scenario.ExecutorPerVUIterations:
				err = e.runPerVUIterations(ctx, def, baseVars)
			default:
				// Fallback to constant-users if not specified
				err = e.runConstantUsers(ctx, def, baseVars)
			}
			if err != nil && err != context.Canceled {
				errCh <- fmt.Errorf("scenario %q error: %w", name, err)
			}
		}(scName, sc)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return nil, <-errCh
	}

	snap := e.collector.Snapshot()
	return &snap, nil
}

func (e *Engine) runConstantUsers(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	vus := sc.Load.VUs
	if vus <= 0 {
		vus = 1
	}

	duration := sc.Load.Duration.Duration()
	if duration <= 0 {
		duration = 10 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	stepRunner := NewStepRunner(e.protocol, e.collector)
	var activeCount atomic.Int64
	var wg sync.WaitGroup

	for vu := 1; vu <= vus; vu++ {
		wg.Add(1)
		go func(vuID int) {
			defer wg.Done()
			defer activeCount.Add(-1)
			activeCount.Add(1)
			e.collector.SetActiveVUs(activeCount.Load())

			// Isolated state per VU
			vuVars := baseVars.Clone()
			vuVars.Set("vu.id", vuID)
			client, jar := e.protocol.NewVUClient()

			iteration := 0
			for {
				select {
				case <-runCtx.Done():
					return
				default:
				}

				iteration++
				vuVars.Set("vu.iteration", iteration)
				vuCtx := &protocol.VUContext{
					VUID:       vuID,
					Iteration:  iteration,
					Vars:       vuVars,
					HTTPClient: client,
					CookieJar:  jar,
				}

				flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
				if flow == FlowAbortTest {
					cancel()
					return
				}

				e.collector.IncIterations()
			}
		}(vu)
	}

	wg.Wait()
	e.collector.SetActiveVUs(0)
	return nil
}

func (e *Engine) runPerVUIterations(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	vus := sc.Load.VUs
	if vus <= 0 {
		vus = 1
	}

	iterations := sc.Load.Iterations
	if iterations <= 0 {
		iterations = 1
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	stepRunner := NewStepRunner(e.protocol, e.collector)
	var activeCount atomic.Int64
	var wg sync.WaitGroup

	for vu := 1; vu <= vus; vu++ {
		wg.Add(1)
		go func(vuID int) {
			defer wg.Done()
			defer activeCount.Add(-1)
			activeCount.Add(1)
			e.collector.SetActiveVUs(activeCount.Load())

			vuVars := baseVars.Clone()
			vuVars.Set("vu.id", vuID)
			client, jar := e.protocol.NewVUClient()

			for iter := 1; iter <= iterations; iter++ {
				select {
				case <-runCtx.Done():
					return
				default:
				}

				vuVars.Set("vu.iteration", iter)
				vuCtx := &protocol.VUContext{
					VUID:       vuID,
					Iteration:  iter,
					Vars:       vuVars,
					HTTPClient: client,
					CookieJar:  jar,
				}

				flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
				if flow == FlowAbortTest {
					cancel()
					return
				}

				e.collector.IncIterations()
			}
		}(vu)
	}

	wg.Wait()
	e.collector.SetActiveVUs(0)
	return nil
}

// EvaluateThresholds evaluates scenario thresholds against a metrics snapshot.
func EvaluateThresholds(thresholds []scenario.Threshold, snap *metrics.Snapshot) (bool, []check.Result) {
	allPassed := true
	results := make([]check.Result, 0, len(thresholds))

	for _, th := range thresholds {
		res := check.Result{
			Rule:   fmt.Sprintf("%s: %s", th.Metric, th.Rule),
			Passed: true,
		}

		switch th.Metric {
		case "http_req_duration":
			res = evalDurationThreshold(th.Rule, snap)
		case "error_rate":
			res = evalErrorRateThreshold(th.Rule, snap)
		case "http_reqs":
			res = evalCountThreshold(th.Rule, snap.TotalRequests)
		}

		if !res.Passed {
			allPassed = false
		}
		results = append(results, res)
	}

	return allPassed, results
}

func evalDurationThreshold(rule string, snap *metrics.Snapshot) check.Result {
	// e.g. "p(95) < 500ms" or "p95 < 500ms" or "max < 1s"
	rule = strings.TrimSpace(rule)
	var actual float64

	switch {
	case strings.HasPrefix(rule, "p(50)") || strings.HasPrefix(rule, "p50"):
		actual = snap.P50
	case strings.HasPrefix(rule, "p(90)") || strings.HasPrefix(rule, "p90"):
		actual = snap.P90
	case strings.HasPrefix(rule, "p(95)") || strings.HasPrefix(rule, "p95"):
		actual = snap.P95
	case strings.HasPrefix(rule, "p(99)") || strings.HasPrefix(rule, "p99"):
		actual = snap.P99
	case strings.HasPrefix(rule, "max"):
		actual = snap.Max
	case strings.HasPrefix(rule, "min"):
		actual = snap.Min
	case strings.HasPrefix(rule, "mean") || strings.HasPrefix(rule, "avg"):
		actual = snap.Mean
	default:
		actual = snap.P95
	}

	// Parse operator and threshold target
	ops := []string{"<=", ">=", "<", ">", "=="}
	for _, op := range ops {
		idx := strings.Index(rule, op)
		if idx != -1 {
			targetStr := strings.TrimSpace(rule[idx+len(op):])
			d, err := time.ParseDuration(targetStr)
			if err != nil {
				return check.Result{Rule: rule, Passed: false, Message: fmt.Sprintf("invalid duration %q in threshold", targetStr)}
			}
			targetMs := float64(d.Milliseconds())
			passed := false
			switch op {
			case "<":
				passed = actual < targetMs
			case "<=":
				passed = actual <= targetMs
			case ">":
				passed = actual > targetMs
			case ">=":
				passed = actual >= targetMs
			case "==":
				passed = actual == targetMs
			}
			msg := ""
			if !passed {
				msg = fmt.Sprintf("actual was %.2fms, threshold was %s %s", actual, op, targetStr)
			}
			return check.Result{Rule: rule, Passed: passed, Message: msg}
		}
	}

	return check.Result{Rule: rule, Passed: false, Message: "failed to parse threshold expression"}
}

func evalErrorRateThreshold(rule string, snap *metrics.Snapshot) check.Result {
	// e.g. "rate < 0.01" or "< 0.05"
	rule = strings.TrimSpace(rule)
	var rate float64
	if snap.TotalRequests > 0 {
		rate = float64(snap.TotalErrors) / float64(snap.TotalRequests)
	}

	ops := []string{"<=", ">=", "<", ">", "=="}
	for _, op := range ops {
		idx := strings.Index(rule, op)
		if idx != -1 {
			targetStr := strings.TrimSpace(rule[idx+len(op):])
			var target float64
			_, err := fmt.Sscanf(targetStr, "%f", &target)
			if err != nil {
				return check.Result{Rule: rule, Passed: false, Message: fmt.Sprintf("invalid rate %q", targetStr)}
			}
			passed := false
			switch op {
			case "<":
				passed = rate < target
			case "<=":
				passed = rate <= target
			case ">":
				passed = rate > target
			case ">=":
				passed = rate >= target
			}
			msg := ""
			if !passed {
				msg = fmt.Sprintf("actual error rate was %.4f, threshold was %s %.4f", rate, op, target)
			}
			return check.Result{Rule: rule, Passed: passed, Message: msg}
		}
	}

	return check.Result{Rule: rule, Passed: false, Message: "failed to parse error rate rule"}
}

func evalCountThreshold(rule string, count int64) check.Result {
	rule = strings.TrimSpace(rule)
	ops := []string{"<=", ">=", "<", ">", "=="}
	for _, op := range ops {
		idx := strings.Index(rule, op)
		if idx != -1 {
			targetStr := strings.TrimSpace(rule[idx+len(op):])
			var target int64
			_, err := fmt.Sscanf(targetStr, "%d", &target)
			if err != nil {
				return check.Result{Rule: rule, Passed: false, Message: fmt.Sprintf("invalid count %q", targetStr)}
			}
			passed := false
			switch op {
			case ">":
				passed = count > target
			case ">=":
				passed = count >= target
			case "<":
				passed = count < target
			case "<=":
				passed = count <= target
			case "==":
				passed = count == target
			}
			msg := ""
			if !passed {
				msg = fmt.Sprintf("actual count was %d, threshold was %s %d", count, op, target)
			}
			return check.Result{Rule: rule, Passed: passed, Message: msg}
		}
	}
	return check.Result{Rule: rule, Passed: false, Message: "failed to parse count threshold rule"}
}
