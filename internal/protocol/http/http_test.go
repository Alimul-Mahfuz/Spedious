package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spedious/spedious/internal/protocol"
	"github.com/spedious/spedious/internal/scenario"
	"github.com/spedious/spedious/internal/vars"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPProtocol_Execute(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"token":"jwt-abc-999"}`))
			return
		}
		if r.URL.Path == "/secure" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer jwt-abc-999" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result":"welcome"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	proto := NewHTTPProtocol()
	client, jar := proto.NewVUClient()
	vStore := vars.NewStore(map[string]any{
		"baseUrl": ts.URL,
	})

	vuCtx := &protocol.VUContext{
		VUID:       1,
		Iteration:  1,
		Vars:       vStore,
		HTTPClient: client,
		CookieJar:  jar,
	}

	// 1. Execute login step with token extraction
	loginStep := &scenario.RequestStep{
		Name:   "login",
		Method: "POST",
		URL:    "${baseUrl}/login",
		Extract: map[string]scenario.ExtractorConfig{
			"tok": {JSONPath: "$.token"},
		},
		Assert: []string{
			"status == 200",
			`body contains "jwt-abc-999"`,
		},
	}

	res, err := proto.Execute(context.Background(), vuCtx, loginStep)
	require.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.True(t, res.Timing.TotalDuration > 0)

	// Verify extracted variable
	extractedTok, ok := vStore.Get("tok")
	require.True(t, ok)
	assert.Equal(t, "jwt-abc-999", extractedTok)

	// 2. Execute secure step using extracted token
	secureStep := &scenario.RequestStep{
		Name:   "secure_call",
		Method: "GET",
		URL:    "${baseUrl}/secure",
		Headers: map[string]string{
			"Authorization": "Bearer ${tok}",
		},
		Assert: []string{
			"status == 200",
			`jsonpath $.result == "welcome"`,
		},
	}

	res2, err := proto.Execute(context.Background(), vuCtx, secureStep)
	require.NoError(t, err)
	assert.Equal(t, 200, res2.StatusCode)
	assert.Equal(t, 0, res2.AssertionFails)
}
