package extract

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/spedious/spedious/internal/scenario"
	"github.com/spyzhov/ajson"
)

// ResponseData provides the context for response extraction.
type ResponseData struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

// Extract extracts values from a response based on the extractor configurations.
// Returns a map of variable name to extracted value.
func Extract(configs map[string]scenario.ExtractorConfig, resp ResponseData) (map[string]any, error) {
	result := make(map[string]any, len(configs))

	var parsedJSON *ajson.Node
	var jsonParseErr error
	hasParsedJSON := false

	for varName, cfg := range configs {
		if cfg.Status {
			result[varName] = resp.StatusCode
			continue
		}

		if cfg.Latency {
			result[varName] = resp.Duration.Milliseconds()
			continue
		}

		if cfg.Header != "" {
			val := resp.Headers.Get(cfg.Header)
			result[varName] = val
			continue
		}

		if cfg.Regex != "" {
			re, err := regexp.Compile(cfg.Regex)
			if err != nil {
				return nil, fmt.Errorf("invalid regex %q for variable %q: %w", cfg.Regex, varName, err)
			}
			matches := re.FindSubmatch(resp.Body)
			if len(matches) > 1 {
				result[varName] = string(matches[1])
			} else if len(matches) == 1 {
				result[varName] = string(matches[0])
			} else {
				result[varName] = ""
			}
			continue
		}

		if cfg.JSONPath != "" {
			if !hasParsedJSON {
				parsedJSON, jsonParseErr = ajson.Unmarshal(resp.Body)
				hasParsedJSON = true
			}
			if jsonParseErr != nil {
				return nil, fmt.Errorf("failed to parse JSON response for JSONPath %q: %w", cfg.JSONPath, jsonParseErr)
			}

			// Ensure JSONPath starts with $
			jp := cfg.JSONPath
			if !strings.HasPrefix(jp, "$") {
				jp = "$." + jp
			}

			nodes, err := parsedJSON.JSONPath(jp)
			if err != nil {
				return nil, fmt.Errorf("JSONPath evaluation failed for %q: %w", cfg.JSONPath, err)
			}
			if len(nodes) == 0 {
				result[varName] = nil
				continue
			}

			val, err := nodes[0].Unpack()
			if err != nil {
				return nil, fmt.Errorf("failed to unpack JSONPath value for %q: %w", cfg.JSONPath, err)
			}
			result[varName] = val
			continue
		}
	}

	return result, nil
}
