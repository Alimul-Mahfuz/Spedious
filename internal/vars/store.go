package vars

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store holds the variables for a single Virtual User (VU) or global context.
type Store struct {
	mu     sync.RWMutex
	values map[string]any
}

// NewStore initializes a new variable store.
func NewStore(initial map[string]any) *Store {
	m := make(map[string]any)
	for k, v := range initial {
		m[k] = v
	}
	return &Store{values: m}
}

// Clone creates a shallow copy of the store for a VU.
func (s *Store) Clone() *Store {
	s.mu.RLock()
	defer s.mu.RUnlock()

	newValues := make(map[string]any, len(s.values))
	for k, v := range s.values {
		newValues[k] = v
	}
	return &Store{values: newValues}
}

// Get retrieves a variable by name or nested dot-path (e.g. "data.users.email").
func (s *Store) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.values[key]; ok {
		return val, true
	}

	// Support dot-notation access
	parts := strings.Split(key, ".")
	if len(parts) > 1 {
		var current any = s.values
		for _, part := range parts {
			if m, ok := current.(map[string]any); ok {
				var found bool
				current, found = m[part]
				if !found {
					return nil, false
				}
			} else {
				return nil, false
			}
		}
		return current, true
	}

	return nil, false
}

// Set sets a variable in the store.
func (s *Store) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = val
}

// All returns a copy of all variables in the store.
func (s *Store) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]any, len(s.values))
	for k, v := range s.values {
		res[k] = v
	}
	return res
}

var (
	templateRegex = regexp.MustCompile(`\$\{([^}]+)\}`)
	funcCallRegex = regexp.MustCompile(`^([a-zA-Z0-9_]+)\((.*)\)$`)
)

// EvaluateString replaces all ${...} placeholders in the input string using the store and built-in functions.
func (s *Store) EvaluateString(input string) string {
	if !strings.Contains(input, "${") {
		return input
	}

	return templateRegex.ReplaceAllStringFunc(input, func(m string) string {
		expr := strings.TrimSpace(m[2 : len(m)-1])

		// 1. Check if it's a built-in function call, e.g. uuid(), random(1, 100), etc.
		if funcMatch := funcCallRegex.FindStringSubmatch(expr); len(funcMatch) == 3 {
			fnName := funcMatch[1]
			fnArgs := strings.TrimSpace(funcMatch[2])
			if res, ok := evalBuiltin(fnName, fnArgs, s); ok {
				return res
			}
		}

		// 2. Check variables in store
		if val, ok := s.Get(expr); ok {
			return fmt.Sprintf("%v", val)
		}

		// If unresolved, return the original expression
		return m
	})
}

// EvaluateAny recursively resolves template strings in maps, slices, and primitive types.
func (s *Store) EvaluateAny(input any) any {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case string:
		return s.EvaluateString(v)
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, val := range v {
			res[s.EvaluateString(k)] = s.EvaluateAny(val)
		}
		return res
	case map[string]string:
		res := make(map[string]string, len(v))
		for k, val := range v {
			res[s.EvaluateString(k)] = s.EvaluateString(val)
		}
		return res
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			res[i] = s.EvaluateAny(item)
		}
		return res
	case []string:
		res := make([]string, len(v))
		for i, item := range v {
			res[i] = s.EvaluateString(item)
		}
		return res
	default:
		return input
	}
}

func evalBuiltin(name, args string, s *Store) (string, bool) {
	switch name {
	case "uuid":
		return uuid.NewString(), true
	case "now":
		return time.Now().UTC().Format(time.RFC3339), true
	case "timestamp":
		return strconv.FormatInt(time.Now().UnixMilli(), 10), true
	case "env":
		// e.g. env("MY_VAR") or env('MY_VAR')
		arg := strings.Trim(strings.TrimSpace(args), `"'`)
		return os.Getenv(arg), true
	case "random":
		// e.g. random(min, max)
		parts := strings.Split(args, ",")
		if len(parts) == 2 {
			minVal, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			maxVal, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err1 == nil && err2 == nil && maxVal >= minVal {
				n := maxVal - minVal + 1
				val, err := rand.Int(rand.Reader, big.NewInt(n))
				if err == nil {
					return strconv.FormatInt(minVal+val.Int64(), 10), true
				}
				// fallback
				return strconv.FormatInt(minVal+mathrand.Int63n(n), 10), true
			}
		}
		return "", false
	case "randomString":
		// e.g. randomString(16)
		n, err := strconv.Atoi(strings.TrimSpace(args))
		if err == nil && n > 0 {
			bytes := make([]byte, (n+1)/2)
			_, _ = rand.Read(bytes)
			str := hex.EncodeToString(bytes)
			if len(str) > n {
				str = str[:n]
			}
			return str, true
		}
		return "", false
	default:
		return "", false
	}
}
