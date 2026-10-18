package mockserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMockServer_Health(t *testing.T) {
	srv := NewServer(ServerConfig{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed to GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var data map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", data["status"])
	}
}

func TestMockServer_AuthAndItems(t *testing.T) {
	srv := NewServer(ServerConfig{
		RequireAuth: true,
		AuthToken:   "secret-token-123",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Without auth -> 401
	resp, err := http.Get(ts.URL + "/items")
	if err != nil {
		t.Fatalf("failed GET /items: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// 2. Login to get token
	loginBody := bytes.NewBufferString(`{"email":"user@example.com","password":"secret"}`)
	resp, err = http.Post(ts.URL+"/auth/login", "application/json", loginBody)
	if err != nil {
		t.Fatalf("failed POST /auth/login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from login, got %d", resp.StatusCode)
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("failed decoding login resp: %v", err)
	}
	if loginResp.Token != "secret-token-123" {
		t.Errorf("unexpected token: %s", loginResp.Token)
	}

	// 3. With auth -> 200
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/items", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed GET /items with auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMockServer_Idempotency(t *testing.T) {
	srv := NewServer(ServerConfig{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/things", nil)
	req1.Header.Set("Idempotency-Key", "key-999")
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("failed post 1: %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 on first post, got %d", resp1.StatusCode)
	}

	// Duplicate with same idempotency key should conflict (409)
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/things", nil)
	req2.Header.Set("Idempotency-Key", "key-999")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("failed post 2: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 on second post, got %d", resp2.StatusCode)
	}
}
