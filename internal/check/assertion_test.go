package check

import (
	"net/http"
	"testing"
	"time"

	"github.com/spedious/spedious/internal/vars"
	"github.com/stretchr/testify/assert"
)

func TestEvaluateRule(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set("X-RateLimit-Remaining", "99")

	body := []byte(`{"id": 101, "name": "Spedious", "status": "active"}`)

	vStore := vars.NewStore(map[string]any{
		"expectedStatus": 200,
	})

	ctx := ResponseContext{
		StatusCode: 200,
		Headers:    headers,
		Body:       body,
		Duration:   45 * time.Millisecond,
		Vars:       vStore,
	}

	tests := []struct {
		rule   string
		passed bool
	}{
		{"status == 200", true},
		{"status == ${expectedStatus}", true},
		{"status != 500", true},
		{"status in [200, 201, 204]", true},
		{"status in [400, 404, 500]", false},
		{"latency < 100ms", true},
		{"latency > 1s", false},
		{`body contains "Spedious"`, true},
		{`body contains "NonExistent"`, false},
		{`body not contains "Error"`, true},
		{`header.Content-Type contains "json"`, true},
		{`header.X-RateLimit-Remaining == "99"`, true},
		{`jsonpath $.id == 101`, true},
		{`jsonpath $.id > 100`, true},
		{`jsonpath $.name == "Spedious"`, true},
		{`jsonpath $.name == "Other"`, false},
	}

	for _, tc := range tests {
		res := EvaluateRule(tc.rule, ctx)
		assert.Equal(t, tc.passed, res.Passed, "Rule: %s, Message: %s", tc.rule, res.Message)
	}
}
