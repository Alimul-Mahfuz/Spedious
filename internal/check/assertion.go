package check

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spedious/spedious/internal/vars"
	"github.com/spyzhov/ajson"
)

// ResponseContext contains all response data needed for assertion evaluation.
type ResponseContext struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
	Vars       *vars.Store
}

// Result captures the outcome of evaluating a single check rule.
type Result struct {
	Rule    string
	Passed  bool
	Message string
}

// EvaluateRule evaluates a single assertion string against the response context.
func EvaluateRule(rule string, ctx ResponseContext) Result {
	rule = strings.TrimSpace(rule)
	if ctx.Vars != nil {
		rule = ctx.Vars.EvaluateString(rule)
	}

	// 1. Status checks: status == 200, status != 500, status in [200, 201, 409]
	if strings.HasPrefix(rule, "status ") {
		return evalStatusRule(rule[7:], ctx.StatusCode)
	}

	// 2. Latency checks: latency < 500ms, latency <= 1s, duration < 500ms
	if strings.HasPrefix(rule, "latency ") || strings.HasPrefix(rule, "duration ") {
		parts := strings.SplitN(rule, " ", 2)
		return evalLatencyRule(parts[1], ctx.Duration)
	}

	// 3. Body contains: body contains "text" or body not contains "text"
	if strings.HasPrefix(rule, "body contains ") {
		expected := parseQuoted(strings.TrimPrefix(rule, "body contains "))
		passed := bytes.Contains(ctx.Body, []byte(expected))
		msg := ""
		if !passed {
			msg = fmt.Sprintf("body does not contain %q", expected)
		}
		return Result{Rule: rule, Passed: passed, Message: msg}
	}
	if strings.HasPrefix(rule, "body not contains ") {
		expected := parseQuoted(strings.TrimPrefix(rule, "body not contains "))
		passed := !bytes.Contains(ctx.Body, []byte(expected))
		msg := ""
		if !passed {
			msg = fmt.Sprintf("body unexpectedly contains %q", expected)
		}
		return Result{Rule: rule, Passed: passed, Message: msg}
	}

	// 4. Header checks: header.<Name> == "val"
	if strings.HasPrefix(rule, "header.") {
		return evalHeaderRule(rule, ctx.Headers)
	}

	// 5. JSONPath checks: jsonpath <path> <op> <val>
	if strings.HasPrefix(rule, "jsonpath ") {
		return evalJSONPathRule(rule[9:], ctx.Body)
	}

	return Result{
		Rule:    rule,
		Passed:  false,
		Message: fmt.Sprintf("unrecognized assertion rule syntax: %q", rule),
	}
}

// EvaluateAll evaluates all rules and returns overall passed status and individual results.
func EvaluateAll(rules []string, ctx ResponseContext) (bool, []Result) {
	allPassed := true
	results := make([]Result, len(rules))
	for i, rule := range rules {
		res := EvaluateRule(rule, ctx)
		results[i] = res
		if !res.Passed {
			allPassed = false
		}
	}
	return allPassed, results
}

func evalStatusRule(expr string, status int) Result {
	expr = strings.TrimSpace(expr)

	// status in [200, 201, 204]
	if strings.HasPrefix(expr, "in ") {
		listStr := strings.TrimSpace(strings.TrimPrefix(expr, "in "))
		listStr = strings.Trim(listStr, "[]")
		items := strings.Split(listStr, ",")
		passed := false
		for _, item := range items {
			if code, err := strconv.Atoi(strings.TrimSpace(item)); err == nil && code == status {
				passed = true
				break
			}
		}
		msg := ""
		if !passed {
			msg = fmt.Sprintf("status %d not in list [%s]", status, listStr)
		}
		return Result{Rule: "status " + expr, Passed: passed, Message: msg}
	}

	// status == 200, status != 500, etc.
	op, targetStr, ok := parseBinaryExpr(expr)
	if !ok {
		return Result{Rule: "status " + expr, Passed: false, Message: "invalid status rule expression"}
	}
	target, err := strconv.Atoi(targetStr)
	if err != nil {
		return Result{Rule: "status " + expr, Passed: false, Message: fmt.Sprintf("invalid status code %q", targetStr)}
	}

	passed := false
	switch op {
	case "==":
		passed = status == target
	case "!=":
		passed = status != target
	case "<":
		passed = status < target
	case "<=":
		passed = status <= target
	case ">":
		passed = status > target
	case ">=":
		passed = status >= target
	}

	msg := ""
	if !passed {
		msg = fmt.Sprintf("status was %d, expected %s %d", status, op, target)
	}
	return Result{Rule: "status " + expr, Passed: passed, Message: msg}
}

func evalLatencyRule(expr string, latency time.Duration) Result {
	op, targetStr, ok := parseBinaryExpr(expr)
	if !ok {
		return Result{Rule: "latency " + expr, Passed: false, Message: "invalid latency rule"}
	}
	target, err := time.ParseDuration(targetStr)
	if err != nil {
		// try parsing as ms number
		if ms, err2 := strconv.ParseInt(targetStr, 10, 64); err2 == nil {
			target = time.Duration(ms) * time.Millisecond
		} else {
			return Result{Rule: "latency " + expr, Passed: false, Message: fmt.Sprintf("invalid duration %q", targetStr)}
		}
	}

	passed := false
	switch op {
	case "<":
		passed = latency < target
	case "<=":
		passed = latency <= target
	case ">":
		passed = latency > target
	case ">=":
		passed = latency >= target
	case "==":
		passed = latency == target
	case "!=":
		passed = latency != target
	}

	msg := ""
	if !passed {
		msg = fmt.Sprintf("latency was %s, expected %s %s", latency, op, target)
	}
	return Result{Rule: "latency " + expr, Passed: passed, Message: msg}
}

func evalHeaderRule(rule string, headers http.Header) Result {
	// header.Content-Type == "application/json"
	parts := strings.SplitN(rule, " ", 2)
	hdrName := strings.TrimPrefix(parts[0], "header.")
	actual := headers.Get(hdrName)

	if len(parts) < 2 {
		return Result{Rule: rule, Passed: actual != "", Message: fmt.Sprintf("header %s is missing", hdrName)}
	}

	op, targetStr, ok := parseBinaryExpr(parts[1])
	if !ok {
		return Result{Rule: rule, Passed: false, Message: "invalid header rule expression"}
	}
	targetStr = parseQuoted(targetStr)

	passed := false
	switch op {
	case "==":
		passed = strings.EqualFold(actual, targetStr)
	case "!=":
		passed = !strings.EqualFold(actual, targetStr)
	case "contains":
		passed = strings.Contains(strings.ToLower(actual), strings.ToLower(targetStr))
	}

	msg := ""
	if !passed {
		msg = fmt.Sprintf("header %q was %q, expected %s %q", hdrName, actual, op, targetStr)
	}
	return Result{Rule: rule, Passed: passed, Message: msg}
}

func evalJSONPathRule(expr string, body []byte) Result {
	// e.g. "$.id == 'item-101'" or "$.count > 0"
	op, targetStr, ok := parseBinaryExpr(expr)
	if !ok {
		return Result{Rule: "jsonpath " + expr, Passed: false, Message: "invalid jsonpath rule"}
	}

	// The LHS is the path
	path := strings.TrimSpace(expr[:strings.Index(expr, op)])
	targetStr = parseQuoted(targetStr)

	root, err := ajson.Unmarshal(body)
	if err != nil {
		return Result{Rule: "jsonpath " + expr, Passed: false, Message: fmt.Sprintf("JSON parse error: %v", err)}
	}

	if !strings.HasPrefix(path, "$") {
		path = "$." + path
	}

	nodes, err := root.JSONPath(path)
	if err != nil || len(nodes) == 0 {
		return Result{Rule: "jsonpath " + expr, Passed: false, Message: fmt.Sprintf("path %q not found in body", path)}
	}

	actualVal, _ := nodes[0].Unpack()
	actualStr := fmt.Sprintf("%v", actualVal)

	passed := false
	// Attempt numeric comparison if possible
	targetNum, errTarget := strconv.ParseFloat(targetStr, 64)
	actualNum, errActual := strconv.ParseFloat(actualStr, 64)
	if errTarget == nil && errActual == nil {
		switch op {
		case "==":
			passed = actualNum == targetNum
		case "!=":
			passed = actualNum != targetNum
		case "<":
			passed = actualNum < targetNum
		case "<=":
			passed = actualNum <= targetNum
		case ">":
			passed = actualNum > targetNum
		case ">=":
			passed = actualNum >= targetNum
		}
	} else {
		switch op {
		case "==":
			passed = actualStr == targetStr
		case "!=":
			passed = actualStr != targetStr
		}
	}

	msg := ""
	if !passed {
		msg = fmt.Sprintf("JSONPath %q value %q did not satisfy %s %s", path, actualStr, op, targetStr)
	}
	return Result{Rule: "jsonpath " + expr, Passed: passed, Message: msg}
}

func parseBinaryExpr(expr string) (op string, target string, ok bool) {
	operators := []string{"==", "!=", "<=", ">=", "<", ">", "contains"}
	for _, candidate := range operators {
		idx := strings.Index(expr, candidate)
		if idx != -1 {
			target := strings.TrimSpace(expr[idx+len(candidate):])
			return candidate, target, true
		}
	}
	return "", "", false
}

func parseQuoted(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}
	return s
}
