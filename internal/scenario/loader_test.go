package scenario

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBytes_Valid(t *testing.T) {
	yamlData := `
version: 1
name: test_api
variables:
  baseUrl: http://localhost:8080

fragments:
  login_frag:
    - request:
        name: login
        method: POST
        url: ${baseUrl}/auth/login

scenarios:
  main:
    load:
      executor: constant-users
      vus: 10
      duration: 30s
    steps:
      - include: login_frag
      - request:
          name: get_items
          method: GET
          url: ${baseUrl}/items
          thinkTime: 100ms-500ms
          onFailure: abort_iteration
      - sleep: 1s
`
	f, err := LoadBytes([]byte(yamlData))
	require.NoError(t, err)
	assert.Equal(t, 1, f.Version)
	assert.Equal(t, "test_api", f.Name)
	assert.Equal(t, "http://localhost:8080", f.Variables["baseUrl"])

	sc, ok := f.Scenarios["main"]
	require.True(t, ok)
	assert.Equal(t, ExecutorConstantUsers, sc.Load.Executor)
	assert.Equal(t, 10, sc.Load.VUs)
	assert.Equal(t, 30*time.Second, sc.Load.Duration.Duration())

	// Steps should have expanded fragment: login, get_items, sleep
	require.Len(t, sc.Steps, 3)
	assert.Equal(t, "login", sc.Steps[0].Request.Name)
	assert.Equal(t, "get_items", sc.Steps[1].Request.Name)
	assert.Equal(t, 1*time.Second, sc.Steps[2].Sleep.Duration())
}

func TestLoadBytes_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		expectedError string
	}{
		{
			name: "invalid version",
			yaml: `
version: 2
name: test
scenarios:
  main:
    load:
      executor: constant-users
      vus: 1
      duration: 1s
    steps:
      - request: { name: "r", method: "GET", url: "http://example.com" }
`,
			expectedError: "unsupported version 2",
		},
		{
			name: "missing name",
			yaml: `
version: 1
scenarios:
  main:
    load:
      executor: constant-users
      vus: 1
      duration: 1s
    steps:
      - request: { name: "r", method: "GET", url: "http://example.com" }
`,
			expectedError: "name is required",
		},
		{
			name: "invalid executor",
			yaml: `
version: 1
name: test
scenarios:
  main:
    load:
      executor: unknown-executor
    steps:
      - request: { name: "r", method: "GET", url: "http://example.com" }
`,
			expectedError: "invalid executor",
		},
		{
			name: "circular fragment include",
			yaml: `
version: 1
name: test
fragments:
  a:
    - include: b
  b:
    - include: a
scenarios:
  main:
    load:
      executor: constant-users
      vus: 1
      duration: 1s
    steps:
      - include: a
`,
			expectedError: "circular fragment include detected",
		},
		{
			name: "missing fragment reference",
			yaml: `
version: 1
name: test
scenarios:
  main:
    load:
      executor: constant-users
      vus: 1
      duration: 1s
    steps:
      - include: nonexistent
`,
			expectedError: "not found in fragments",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadBytes([]byte(tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}
