package extract

import (
	"net/http"
	"testing"
	"time"

	"github.com/spedious/spedious/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract(t *testing.T) {
	headers := make(http.Header)
	headers.Set("X-Custom-Header", "custom-value")
	headers.Set("Content-Type", "application/json")

	body := []byte(`{
		"token": "secret-jwt-123",
		"items": [
			{"id": "item-101", "name": "Book"},
			{"id": "item-102", "name": "Pen"}
		],
		"active": true,
		"count": 42
	}`)

	resp := ResponseData{
		StatusCode: 200,
		Headers:    headers,
		Body:       body,
		Duration:   150 * time.Millisecond,
	}

	configs := map[string]scenario.ExtractorConfig{
		"st":      {Status: true},
		"lat":     {Latency: true},
		"hdr":     {Header: "X-Custom-Header"},
		"token":   {JSONPath: "$.token"},
		"firstId": {JSONPath: "$.items[0].id"},
		"cnt":     {JSONPath: "$.count"},
		"regVal":  {Regex: `secret-jwt-(\d+)`},
	}

	vars, err := Extract(configs, resp)
	require.NoError(t, err)

	assert.Equal(t, 200, vars["st"])
	assert.Equal(t, int64(150), vars["lat"])
	assert.Equal(t, "custom-value", vars["hdr"])
	assert.Equal(t, "secret-jwt-123", vars["token"])
	assert.Equal(t, "item-101", vars["firstId"])
	assert.Equal(t, float64(42), vars["cnt"])
	assert.Equal(t, "123", vars["regVal"])
}
