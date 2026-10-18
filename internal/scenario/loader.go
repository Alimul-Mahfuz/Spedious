package scenario

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError represents a line-numbered or field-specific validation error.
type ValidationError struct {
	Field   string
	Line    int
	Message string
}

func (e *ValidationError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: [%s] %s", e.Line, e.Field, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Field, e.Message)
}

// LoadFile reads and parses a YAML scenario file from disk, expanding fragments and validating.
func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file %q: %w", path, err)
	}
	return LoadBytes(data)
}

// LoadBytes parses YAML bytes into a scenario File, expanding fragments and validating.
func LoadBytes(data []byte) (*File, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("YAML syntax error: %w", err)
	}

	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("failed to decode scenario: %w", err)
	}

	if err := Validate(&file, &node); err != nil {
		return nil, err
	}

	if err := expandFragments(&file); err != nil {
		return nil, err
	}

	return &file, nil
}

// Validate checks the semantic validity of a scenario file.
func Validate(f *File, rootNode *yaml.Node) error {
	if f.Version != 1 {
		line := findNodeLine(rootNode, "version")
		return &ValidationError{Field: "version", Line: line, Message: fmt.Sprintf("unsupported version %d (only version 1 is supported)", f.Version)}
	}

	if strings.TrimSpace(f.Name) == "" {
		line := findNodeLine(rootNode, "name")
		return &ValidationError{Field: "name", Line: line, Message: "scenario file name is required"}
	}

	if len(f.Scenarios) == 0 {
		line := findNodeLine(rootNode, "scenarios")
		return &ValidationError{Field: "scenarios", Line: line, Message: "at least one scenario must be defined"}
	}

	for scName, sc := range f.Scenarios {
		if err := validateScenario(scName, sc, rootNode); err != nil {
			return err
		}
	}

	for i, th := range f.Thresholds {
		if strings.TrimSpace(th.Metric) == "" {
			return &ValidationError{Field: fmt.Sprintf("thresholds[%d].metric", i), Message: "threshold metric cannot be empty"}
		}
		if strings.TrimSpace(th.Rule) == "" {
			return &ValidationError{Field: fmt.Sprintf("thresholds[%d].rule", i), Message: "threshold rule cannot be empty"}
		}
	}

	return nil
}

func validateScenario(name string, sc ScenarioDefinition, rootNode *yaml.Node) error {
	scField := fmt.Sprintf("scenarios.%s", name)

	if !IsValidExecutor(sc.Load.Executor) {
		line := findNodeLine(rootNode, "executor")
		return &ValidationError{
			Field:   scField + ".load.executor",
			Line:    line,
			Message: fmt.Sprintf("invalid executor %q (must be one of: constant-users, ramping-users, per-vu-iterations, shared-iterations, constant-arrival-rate, ramping-arrival-rate)", sc.Load.Executor),
		}
	}

	switch strings.ToLower(sc.Load.Executor) {
	case ExecutorConstantUsers:
		if sc.Load.VUs <= 0 {
			return &ValidationError{Field: scField + ".load.vus", Message: "vus must be greater than 0"}
		}
		if sc.Load.Duration.Duration() <= 0 {
			return &ValidationError{Field: scField + ".load.duration", Message: "duration must be greater than 0"}
		}
	case ExecutorPerVUIterations:
		if sc.Load.VUs <= 0 {
			return &ValidationError{Field: scField + ".load.vus", Message: "vus must be greater than 0"}
		}
		if sc.Load.Iterations <= 0 {
			return &ValidationError{Field: scField + ".load.iterations", Message: "iterations must be greater than 0"}
		}
	case ExecutorRampingUsers, ExecutorRampingArrivalRate:
		if len(sc.Load.Stages) == 0 {
			return &ValidationError{Field: scField + ".load.stages", Message: "at least one stage must be defined for ramping executor"}
		}
	}

	if len(sc.Steps) == 0 {
		return &ValidationError{Field: scField + ".steps", Message: "scenario must have at least one step"}
	}

	for i, step := range sc.Steps {
		stepField := fmt.Sprintf("%s.steps[%d]", scField, i)
		if err := validateStep(stepField, step); err != nil {
			return err
		}
	}

	return nil
}

func validateStep(field string, s Step) error {
	hasAction := false

	if s.Include != "" {
		hasAction = true
	}

	if s.Request != nil {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
		if strings.TrimSpace(s.Request.Name) == "" {
			return &ValidationError{Field: field + ".request.name", Message: "request name is required"}
		}
		if strings.TrimSpace(s.Request.Method) == "" {
			return &ValidationError{Field: field + ".request.method", Message: "request method is required"}
		}
		if strings.TrimSpace(s.Request.URL) == "" {
			return &ValidationError{Field: field + ".request.url", Message: "request url is required"}
		}
		if s.Request.OnFailure != "" {
			switch s.Request.OnFailure {
			case OnFailureAbortIteration, OnFailureContinue, OnFailureAbortTest:
			default:
				return &ValidationError{
					Field:   field + ".request.onFailure",
					Message: fmt.Sprintf("invalid onFailure policy %q (must be abort_iteration, continue, or abort_test)", s.Request.OnFailure),
				}
			}
		}
	}

	if s.Sleep.Duration() > 0 {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
	}

	if s.If != nil {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
		if strings.TrimSpace(s.If.Condition) == "" {
			return &ValidationError{Field: field + ".if.condition", Message: "condition cannot be empty"}
		}
		if len(s.If.Then) == 0 {
			return &ValidationError{Field: field + ".if.then", Message: "then block cannot be empty"}
		}
	}

	if s.Loop != nil {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
		if s.Loop.Count <= 0 && strings.TrimSpace(s.Loop.Condition) == "" {
			return &ValidationError{Field: field + ".loop", Message: "loop requires either a positive count or a condition"}
		}
		if len(s.Loop.Steps) == 0 {
			return &ValidationError{Field: field + ".loop.steps", Message: "loop steps cannot be empty"}
		}
	}

	if s.Group != nil {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
		if strings.TrimSpace(s.Group.Name) == "" {
			return &ValidationError{Field: field + ".group.name", Message: "group name cannot be empty"}
		}
		if len(s.Group.Steps) == 0 {
			return &ValidationError{Field: field + ".group.steps", Message: "group steps cannot be empty"}
		}
	}

	if s.Sync != nil {
		if hasAction {
			return &ValidationError{Field: field, Message: "step cannot combine multiple actions"}
		}
		hasAction = true
		if s.Sync.WaitFor <= 0 {
			return &ValidationError{Field: field + ".sync.waitFor", Message: "waitFor must be greater than 0"}
		}
		if s.Sync.Timeout.Duration() <= 0 {
			return &ValidationError{Field: field + ".sync.timeout", Message: "sync timeout must be greater than 0"}
		}
	}

	if !hasAction {
		return &ValidationError{Field: field, Message: "empty step with no action specified"}
	}

	return nil
}

func expandFragments(f *File) error {
	for scName, sc := range f.Scenarios {
		visited := make(map[string]bool)
		expandedSteps, err := expandStepList(sc.Steps, f.Fragments, visited)
		if err != nil {
			return fmt.Errorf("in scenario %q: %w", scName, err)
		}
		sc.Steps = expandedSteps
		f.Scenarios[scName] = sc
	}
	return nil
}

func expandStepList(steps []Step, fragments map[string][]Step, visited map[string]bool) ([]Step, error) {
	result := make([]Step, 0, len(steps))

	for _, step := range steps {
		if step.Include != "" {
			fragName := step.Include
			if visited[fragName] {
				return nil, fmt.Errorf("circular fragment include detected: %q", fragName)
			}
			fragSteps, ok := fragments[fragName]
			if !ok {
				return nil, fmt.Errorf("referenced fragment %q not found in fragments", fragName)
			}
			visited[fragName] = true
			expanded, err := expandStepList(fragSteps, fragments, visited)
			delete(visited, fragName)
			if err != nil {
				return nil, err
			}
			result = append(result, expanded...)
		} else if step.If != nil {
			thenExp, err := expandStepList(step.If.Then, fragments, visited)
			if err != nil {
				return nil, err
			}
			elseExp, err := expandStepList(step.If.Else, fragments, visited)
			if err != nil {
				return nil, err
			}
			step.If.Then = thenExp
			step.If.Else = elseExp
			result = append(result, step)
		} else if step.Loop != nil {
			loopExp, err := expandStepList(step.Loop.Steps, fragments, visited)
			if err != nil {
				return nil, err
			}
			step.Loop.Steps = loopExp
			result = append(result, step)
		} else if step.Group != nil {
			groupExp, err := expandStepList(step.Group.Steps, fragments, visited)
			if err != nil {
				return nil, err
			}
			step.Group.Steps = groupExp
			result = append(result, step)
		} else {
			result = append(result, step)
		}
	}

	return result, nil
}

func findNodeLine(node *yaml.Node, key string) int {
	if node == nil {
		return 0
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return findNodeLine(node.Content[0], key)
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i]
			if k.Value == key {
				return k.Line
			}
			if line := findNodeLine(node.Content[i+1], key); line > 0 {
				return line
			}
		}
	}
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if line := findNodeLine(child, key); line > 0 {
				return line
			}
		}
	}
	return 0
}
