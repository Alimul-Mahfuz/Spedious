package vars

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_GetSet(t *testing.T) {
	s := NewStore(map[string]any{
		"baseUrl": "https://api.example.com",
		"data": map[string]any{
			"users": map[string]any{
				"email": "user@test.com",
			},
		},
	})

	val, ok := s.Get("baseUrl")
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", val)

	nested, ok := s.Get("data.users.email")
	require.True(t, ok)
	assert.Equal(t, "user@test.com", nested)

	s.Set("token", "abc-123")
	tok, ok := s.Get("token")
	require.True(t, ok)
	assert.Equal(t, "abc-123", tok)
}

func TestStore_EvaluateString(t *testing.T) {
	_ = os.Setenv("TEST_SPEDIOUS_ENV", "spedious-rocks")
	defer func() { _ = os.Unsetenv("TEST_SPEDIOUS_ENV") }()

	s := NewStore(map[string]any{
		"baseUrl": "https://api.example.com",
		"token":   "tok123",
	})

	assert.Equal(t, "https://api.example.com/items", s.EvaluateString("${baseUrl}/items"))
	assert.Equal(t, "Bearer tok123", s.EvaluateString("Bearer ${token}"))
	assert.Equal(t, "spedious-rocks", s.EvaluateString(`${env("TEST_SPEDIOUS_ENV")}`))

	// uuid()
	uuidStr := s.EvaluateString("${uuid()}")
	assert.Len(t, uuidStr, 36)

	// random(min, max)
	randNum := s.EvaluateString("${random(10, 20)}")
	n, err := strconv.Atoi(randNum)
	require.NoError(t, err)
	assert.True(t, n >= 10 && n <= 20)

	// randomString(8)
	randStr := s.EvaluateString("${randomString(8)}")
	assert.Len(t, randStr, 8)
}

func TestStore_EvaluateAny(t *testing.T) {
	s := NewStore(map[string]any{
		"name": "Widget",
		"id":   "101",
	})

	data := map[string]any{
		"title": "Item: ${name}",
		"meta": map[string]any{
			"id": "${id}",
		},
	}

	evaluated := s.EvaluateAny(data).(map[string]any)
	assert.Equal(t, "Item: Widget", evaluated["title"])
	assert.Equal(t, "101", evaluated["meta"].(map[string]any)["id"])
}
