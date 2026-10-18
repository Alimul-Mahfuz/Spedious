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
	protocol    *httpProto.HTTPProtocol
	collector   *metrics.Collector
	onProgress  ProgressCallback
	abortOnFail bool
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

// SetAbortOnFail enables early termination if any threshold fails during execution.
func (e *Engine) SetAbortOnFail(abort bool) {
	e.abortOnFail = abort
}

// Collector returns the engine's metrics collector.
func (e *Engine) Collector() *metrics.Collector {
	return e.collector
}

// Run executes the entire scenario file.
func (e *Engine) Run(ctx context.Context, f *scenario.File) (*metrics.Snapshot, error) {
	e.collector.Reset()

	baseVars := vars.NewStore(f.Variables)

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// Progress reporter and time-series sampler ticker
	progressCtx, cancelProgress := context.WithCancel(runCtx)
	defer cancelProgress()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		sampleTicker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		defer sampleTicker.Stop()

		for {
			select {
			case <-ticker.C:
				snap := e.collector.Snapshot()
				if e.onProgress != nil {
					e.onProgress(snap)
				}
				// Check abort-on-threshold-fail
				if e.abortOnFail || hasAbortThreshold(f.Thresholds) {
					passed, results := EvaluateThresholds(f.Thresholds, &snap)
					if !passed {
						for _, r := range results {
							if !r.Passed {
								fmt.Printf("\n[ALERT] Aborting run: threshold failed: %s (%s)\n", r.Rule, r.Message)
								cancelRun()
								return
							}
						}
					}
				}
			case <-sampleTicker.C:
				e.collector.RecordSample()
			case <-progressCtx.Done():
				return
			}
		}
	}()

	// Compute weighted scenario allocations if weights are provided
	totalWeight := 0.0
	for _, sc := range f.Scenarios {
		if sc.Weight > 0 {
			totalWeight += sc.Weight
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(f.Scenarios))

	for scName, sc := range f.Scenarios {
		wg.Add(1)
		go func(name string, def scenario.ScenarioDefinition) {
			defer wg.Done()
			var err error

			switch def.Load.Executor {
			case scenario.ExecutorConstantUsers:
				err = e.runConstantUsers(runCtx, def, baseVars)
			case scenario.ExecutorPerVUIterations:
				err = e.runPerVUIterations(runCtx, def, baseVars)
			case scenario.ExecutorSharedIterations:
				err = e.runSharedIterations(runCtx, def, baseVars)
			case scenario.ExecutorRampingUsers:
				err = e.runRampingUsers(runCtx, def, baseVars)
			case scenario.ExecutorConstantArrivalRate:
				err = e.runConstantArrivalRate(runCtx, def, baseVars)
			case scenario.ExecutorRampingArrivalRate:
				err = e.runRampingArrivalRate(runCtx, def, baseVars)
			default:
				err = e.runConstantUsers(runCtx, def, baseVars)
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

func hasAbortThreshold(thresholds []scenario.Threshold) bool {
	for _, t := range thresholds {
		if t.Abort {
			return true
		}
	}
	return false
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
				iterStart := time.Now()
				vuVars.Set("vu.iteration", iteration)
				vuCtx := &protocol.VUContext{
					VUID:       vuID,
					Iteration:  iteration,
					Vars:       vuVars,
					HTTPClient: client,
					CookieJar:  jar,
				}

				flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
				e.collector.RecordIterationDuration(time.Since(iterStart))
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

				iterStart := time.Now()
				vuVars.Set("vu.iteration", iter)
				vuCtx := &protocol.VUContext{
					VUID:       vuID,
					Iteration:  iter,
					Vars:       vuVars,
					HTTPClient: client,
					CookieJar:  jar,
				}

				flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
				e.collector.RecordIterationDuration(time.Since(iterStart))
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

func (e *Engine) runSharedIterations(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	vus := sc.Load.VUs
	if vus <= 0 {
		vus = 1
	}
	totalIterations := int64(sc.Load.Iterations)
	if totalIterations <= 0 {
		totalIterations = 1
	}

	var iterCounter atomic.Int64
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

			for {
				select {
				case <-runCtx.Done():
					return
				default:
				}

				iter := iterCounter.Add(1)
				if iter > totalIterations {
					return
				}

				iterStart := time.Now()
				vuVars.Set("vu.iteration", int(iter))
				vuCtx := &protocol.VUContext{
					VUID:       vuID,
					Iteration:  int(iter),
					Vars:       vuVars,
					HTTPClient: client,
					CookieJar:  jar,
				}

				flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
				e.collector.RecordIterationDuration(time.Since(iterStart))
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

func (e *Engine) runRampingUsers(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	stages := sc.Load.Stages
	if len(stages) == 0 {
		return fmt.Errorf("no stages configured for ramping-users")
	}

	stepRunner := NewStepRunner(e.protocol, e.collector)
	var activeCount atomic.Int64
	var currentVUID atomic.Int64
	stopChans := make(chan chan struct{}, 10000)

	totalDuration := time.Duration(0)
	for _, st := range stages {
		totalDuration += st.Duration.Duration()
	}

	runCtx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	var wg sync.WaitGroup

	// Stage ramp manager loop
	managerDone := make(chan struct{})
	go func() {
		defer close(managerDone)
		currentStageStart := time.Now()
		startTarget := 0
		stageIdx := 0

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for stageIdx < len(stages) {
			st := stages[stageIdx]
			stDur := st.Duration.Duration()
			elapsed := time.Since(currentStageStart)

			if elapsed >= stDur {
				startTarget = st.Target
				stageIdx++
				currentStageStart = time.Now()
				if stageIdx >= len(stages) {
					break
				}
				continue
			}

			// Linear interpolation
			progress := float64(elapsed) / float64(stDur)
			target := startTarget + int(float64(st.Target-startTarget)*progress)

			currentActive := activeCount.Load()
			if currentActive < int64(target) {
				diff := int(int64(target) - currentActive)
				for i := 0; i < diff; i++ {
					vID := int(currentVUID.Add(1))
					stopCh := make(chan struct{})
					stopChans <- stopCh
					activeCount.Add(1)
					e.collector.SetActiveVUs(activeCount.Load())
					wg.Add(1)
					go func(vuID int, stop <-chan struct{}) {
						defer wg.Done()
						defer func() {
							activeCount.Add(-1)
							e.collector.SetActiveVUs(activeCount.Load())
						}()
						vuVars := baseVars.Clone()
						vuVars.Set("vu.id", vuID)
						client, jar := e.protocol.NewVUClient()
						iter := 0
						for {
							select {
							case <-runCtx.Done():
								return
							case <-stop:
								return
							default:
							}
							iter++
							iterStart := time.Now()
							vuVars.Set("vu.iteration", iter)
							vuCtx := &protocol.VUContext{
								VUID:       vuID,
								Iteration:  iter,
								Vars:       vuVars,
								HTTPClient: client,
								CookieJar:  jar,
							}
							flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
							e.collector.RecordIterationDuration(time.Since(iterStart))
							if flow == FlowAbortTest {
								cancel()
								return
							}
							e.collector.IncIterations()
						}
					}(vID, stopCh)
				}
			} else if currentActive > int64(target) {
				diff := int(currentActive - int64(target))
				for i := 0; i < diff; i++ {
					select {
					case ch := <-stopChans:
						close(ch)
					default:
					}
				}
			}

			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	<-managerDone
	wg.Wait()
	e.collector.SetActiveVUs(0)
	return nil
}

// runConstantArrivalRate implements the Open Model to avoid coordinated omission.
func (e *Engine) runConstantArrivalRate(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	rate := sc.Load.Rate
	if rate <= 0 {
		rate = 10
	}
	timeUnit := sc.Load.TimeUnit.Duration()
	if timeUnit <= 0 {
		timeUnit = 1 * time.Second
	}
	duration := sc.Load.Duration.Duration()
	if duration <= 0 {
		duration = 10 * time.Second
	}
	maxVUs := sc.Load.MaxVUs
	if maxVUs <= 0 {
		maxVUs = 1000
	}

	interval := time.Duration(float64(timeUnit) / float64(rate))

	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	stepRunner := NewStepRunner(e.protocol, e.collector)
	var activeWorkers atomic.Int64
	var vuCounter atomic.Int64
	var wg sync.WaitGroup

	startTime := time.Now()
	iterationIndex := 0

	for {
		iterationIndex++
		intendedStart := startTime.Add(time.Duration(iterationIndex) * interval)
		waitDuration := time.Until(intendedStart)

		if waitDuration > 0 {
			select {
			case <-runCtx.Done():
				wg.Wait()
				e.collector.SetActiveVUs(0)
				return nil
			case <-time.After(waitDuration):
			}
		} else {
			// Check context even if overdue
			select {
			case <-runCtx.Done():
				wg.Wait()
				e.collector.SetActiveVUs(0)
				return nil
			default:
			}
		}

		// Check if active worker pool capacity is reached
		currentActive := activeWorkers.Load()
		if currentActive >= int64(maxVUs) {
			// Pool exhausted: iteration is dropped!
			e.collector.IncDroppedIterations()
			continue
		}

		activeWorkers.Add(1)
		e.collector.SetActiveVUs(activeWorkers.Load())
		vID := int(vuCounter.Add(1))
		iterNum := iterationIndex

		wg.Add(1)
		go func(vuID int, iter int, intendedTime time.Time) {
			defer wg.Done()
			defer func() {
				activeWorkers.Add(-1)
				e.collector.SetActiveVUs(activeWorkers.Load())
			}()

			vuVars := baseVars.Clone()
			vuVars.Set("vu.id", vuID)
			vuVars.Set("vu.iteration", iter)
			client, jar := e.protocol.NewVUClient()

			vuCtx := &protocol.VUContext{
				VUID:       vuID,
				Iteration:  iter,
				Vars:       vuVars,
				HTTPClient: client,
				CookieJar:  jar,
			}

			flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
			// Calculate iteration duration from the INTENDED start time to avoid coordinated omission!
			e.collector.RecordIterationDuration(time.Since(intendedTime))
			if flow == FlowAbortTest {
				cancel()
				return
			}
			e.collector.IncIterations()
		}(vID, iterNum, intendedStart)
	}
}

// runRampingArrivalRate implements the Open Model with changing arrival rate over time.
func (e *Engine) runRampingArrivalRate(ctx context.Context, sc scenario.ScenarioDefinition, baseVars *vars.Store) error {
	stages := sc.Load.Stages
	if len(stages) == 0 {
		return fmt.Errorf("no stages configured for ramping-arrival-rate")
	}

	timeUnit := sc.Load.TimeUnit.Duration()
	if timeUnit <= 0 {
		timeUnit = 1 * time.Second
	}
	maxVUs := sc.Load.MaxVUs
	if maxVUs <= 0 {
		maxVUs = 1000
	}

	totalDuration := time.Duration(0)
	for _, st := range stages {
		totalDuration += st.Duration.Duration()
	}

	runCtx, cancel := context.WithTimeout(ctx, totalDuration)
	defer cancel()

	stepRunner := NewStepRunner(e.protocol, e.collector)
	var activeWorkers atomic.Int64
	var vuCounter atomic.Int64
	var wg sync.WaitGroup

	startTime := time.Now()
	stageIdx := 0
	currentStageStart := startTime
	startTarget := 0
	iterCount := 0

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			wg.Wait()
			e.collector.SetActiveVUs(0)
			return nil
		case <-ticker.C:
		}

		elapsed := time.Since(currentStageStart)
		st := stages[stageIdx]
		stDur := st.Duration.Duration()

		if elapsed >= stDur {
			startTarget = st.Target
			stageIdx++
			currentStageStart = time.Now()
			if stageIdx >= len(stages) {
				break
			}
			continue
		}

		progress := float64(elapsed) / float64(stDur)
		currentRate := float64(startTarget) + float64(st.Target-startTarget)*progress
		if currentRate <= 0 {
			continue
		}

		interval := time.Duration(float64(timeUnit) / currentRate)
		intendedTime := time.Now()

		// Launch iteration for current tick interval
		iterCount++
		currentActive := activeWorkers.Load()
		if currentActive >= int64(maxVUs) {
			e.collector.IncDroppedIterations()
			continue
		}

		activeWorkers.Add(1)
		e.collector.SetActiveVUs(activeWorkers.Load())
		vID := int(vuCounter.Add(1))
		iterNum := iterCount

		wg.Add(1)
		go func(vuID, iter int, intended time.Time) {
			defer wg.Done()
			defer func() {
				activeWorkers.Add(-1)
				e.collector.SetActiveVUs(activeWorkers.Load())
			}()

			vuVars := baseVars.Clone()
			vuVars.Set("vu.id", vuID)
			vuVars.Set("vu.iteration", iter)
			client, jar := e.protocol.NewVUClient()

			vuCtx := &protocol.VUContext{
				VUID:       vuID,
				Iteration:  iter,
				Vars:       vuVars,
				HTTPClient: client,
				CookieJar:  jar,
			}

			flow, _ := stepRunner.ExecuteSteps(runCtx, vuCtx, sc.Steps)
			e.collector.RecordIterationDuration(time.Since(intended))
			if flow == FlowAbortTest {
				cancel()
				return
			}
			e.collector.IncIterations()
		}(vID, iterNum, intendedTime)

		time.Sleep(interval)
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
