package engine

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/spedious/spedious/internal/metrics"
	"github.com/spedious/spedious/internal/protocol"
	"github.com/spedious/spedious/internal/scenario"
)

// FlowControl signals loop and iteration termination.
type FlowControl int

const (
	FlowContinue FlowControl = iota
	FlowAbortIteration
	FlowAbortTest
)

// StepRunner executes individual scenario steps for a virtual user.
type StepRunner struct {
	protocol  protocol.Protocol
	collector *metrics.Collector
}

// NewStepRunner creates a StepRunner.
func NewStepRunner(p protocol.Protocol, c *metrics.Collector) *StepRunner {
	return &StepRunner{
		protocol:  p,
		collector: c,
	}
}

// ExecuteSteps runs a sequence of steps in order.
func (r *StepRunner) ExecuteSteps(ctx context.Context, vuCtx *protocol.VUContext, steps []scenario.Step) (FlowControl, error) {
	for _, step := range steps {
		select {
		case <-ctx.Done():
			return FlowAbortTest, ctx.Err()
		default:
		}

		flow, err := r.ExecuteStep(ctx, vuCtx, step)
		if flow != FlowContinue {
			return flow, err
		}
	}
	return FlowContinue, nil
}

// ExecuteStep runs a single step (request, sleep, if, loop, group).
func (r *StepRunner) ExecuteStep(ctx context.Context, vuCtx *protocol.VUContext, step scenario.Step) (FlowControl, error) {
	// 1. Sleep step
	if step.Sleep.Duration() > 0 {
		if err := sleepWithContext(ctx, step.Sleep.Duration()); err != nil {
			return FlowAbortTest, err
		}
		return FlowContinue, nil
	}

	// 2. Request step
	if step.Request != nil {
		reqStep := step.Request
		res, err := r.protocol.Execute(ctx, vuCtx, reqStep)
		if ctx.Err() != nil {
			return FlowAbortTest, ctx.Err()
		}

		isError := err != nil || (res != nil && res.AssertionFails > 0)
		if res != nil {
			// Check if status is classified as expectedStatus
			if len(reqStep.ExpectedStatus) > 0 {
				for _, exp := range reqStep.ExpectedStatus {
					if res.StatusCode == exp {
						isError = false
						break
					}
				}
			}

			r.collector.RecordRequestWithTiming(reqStep.Name, res.Timing, isError, res.BytesSent, res.BytesReceived)
		}

		// Handle think time
		if reqStep.ThinkTime != "" {
			tt, parseErr := parseThinkTime(reqStep.ThinkTime)
			if parseErr == nil && tt > 0 {
				if err := sleepWithContext(ctx, tt); err != nil {
					return FlowAbortTest, err
				}
			}
		}

		// Handle failure policy
		if isError {
			policy := reqStep.OnFailure
			if policy == "" {
				policy = scenario.OnFailureAbortIteration
			}
			switch policy {
			case scenario.OnFailureAbortIteration:
				return FlowAbortIteration, err
			case scenario.OnFailureAbortTest:
				return FlowAbortTest, err
			case scenario.OnFailureContinue:
				return FlowContinue, nil
			}
		}

		return FlowContinue, nil
	}

	// 3. Conditional (if) step
	if step.If != nil {
		cond := vuCtx.Vars.EvaluateString(step.If.Condition)
		if evalCondition(cond) {
			return r.ExecuteSteps(ctx, vuCtx, step.If.Then)
		} else if len(step.If.Else) > 0 {
			return r.ExecuteSteps(ctx, vuCtx, step.If.Else)
		}
		return FlowContinue, nil
	}

	// 4. Loop step
	if step.Loop != nil {
		if step.Loop.Count > 0 {
			for i := 0; i < step.Loop.Count; i++ {
				flow, err := r.ExecuteSteps(ctx, vuCtx, step.Loop.Steps)
				if flow != FlowContinue {
					return flow, err
				}
			}
		} else if step.Loop.Condition != "" {
			maxIters := 10000 // safeguard against infinite loops
			iters := 0
			for {
				iters++
				if iters > maxIters {
					return FlowAbortIteration, fmt.Errorf("loop exceeded safety limit of %d iterations", maxIters)
				}
				cond := vuCtx.Vars.EvaluateString(step.Loop.Condition)
				if !evalCondition(cond) {
					break
				}
				flow, err := r.ExecuteSteps(ctx, vuCtx, step.Loop.Steps)
				if flow != FlowContinue {
					return flow, err
				}
			}
		}
		return FlowContinue, nil
	}

	// 5. Group step
	if step.Group != nil {
		start := time.Now()
		flow, err := r.ExecuteSteps(ctx, vuCtx, step.Group.Steps)
		groupDuration := time.Since(start)
		isErr := flow != FlowContinue || err != nil
		r.collector.RecordRequest("group:"+step.Group.Name, groupDuration, isErr, 0, 0)
		return flow, err
	}

	return FlowContinue, nil
}

func parseThinkTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		minD, err1 := time.ParseDuration(strings.TrimSpace(parts[0]))
		maxD, err2 := time.ParseDuration(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid think time range %q", s)
		}
		if maxD <= minD {
			return minD, nil
		}
		delta := maxD - minD
		randomOffset := time.Duration(rand.Int63n(int64(delta)))
		return minD + randomOffset, nil
	}
	return time.ParseDuration(s)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func evalCondition(cond string) bool {
	cond = strings.TrimSpace(cond)
	if cond == "true" {
		return true
	}
	if cond == "false" || cond == "" {
		return false
	}

	ops := []string{"==", "!=", "<=", ">=", "<", ">"}
	for _, op := range ops {
		idx := strings.Index(cond, op)
		if idx != -1 {
			lhs := strings.TrimSpace(cond[:idx])
			rhs := strings.TrimSpace(cond[idx+len(op):])

			// Try numeric
			lNum, errL := strconv.ParseFloat(lhs, 64)
			rNum, errR := strconv.ParseFloat(rhs, 64)
			if errL == nil && errR == nil {
				switch op {
				case "==":
					return lNum == rNum
				case "!=":
					return lNum != rNum
				case "<":
					return lNum < rNum
				case "<=":
					return lNum <= rNum
				case ">":
					return lNum > rNum
				case ">=":
					return lNum >= rNum
				}
			}

			// String comparison
			lhs = strings.Trim(lhs, `"'`)
			rhs = strings.Trim(rhs, `"'`)
			switch op {
			case "==":
				return lhs == rhs
			case "!=":
				return lhs != rhs
			}
		}
	}

	return false
}
