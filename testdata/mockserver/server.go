package mockserver

import (
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ServerConfig configures mock server behaviors.
type ServerConfig struct {
	DefaultDelay    time.Duration
	DefaultStatus   int
	ErrorRate       float64 // 0.0 to 1.0 probability of returning 500
	RequireAuth     bool
	AuthToken       string
	SimulateStall   bool
	StallDuration   time.Duration
	StallAfterCount int64
}

// Item represents a mock stateful resource.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Server provides configurable HTTP endpoints for Spedious tests.
type Server struct {
	Config ServerConfig

	mu             sync.RWMutex
	items          map[string]Item
	idempotencyMap map[string]int
	requestCount   atomic.Int64
}

// NewServer creates a new configured mock server.
func NewServer(cfg ServerConfig) *Server {
	if cfg.DefaultStatus == 0 {
		cfg.DefaultStatus = http.StatusOK
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = "test-token-spedious-xyz"
	}
	s := &Server{
		Config:         cfg,
		items:          make(map[string]Item),
		idempotencyMap: make(map[string]int),
	}

	// Pre-populate some items
	s.items["item-1"] = Item{ID: "item-1", Name: "Widget A"}
	s.items["item-2"] = Item{ID: "item-2", Name: "Widget B"}

	return s
}

// Handler returns the HTTP handler for the mock server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.HandleFunc("/items", s.handleItems)
	mux.HandleFunc("/things", s.handleThings)
	mux.HandleFunc("/echo", s.handleEcho)
	mux.HandleFunc("/stall", s.handleStall)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqNum := s.requestCount.Add(1)

		// Check for stall simulation
		if s.Config.SimulateStall && s.Config.StallAfterCount > 0 && reqNum >= s.Config.StallAfterCount {
			time.Sleep(s.Config.StallDuration)
		}

		// Check header or query parameter for delay
		delay := s.Config.DefaultDelay
		if d := r.Header.Get("X-Mock-Delay"); d != "" {
			if parsed, err := time.ParseDuration(d); err == nil {
				delay = parsed
			}
		} else if d := r.URL.Query().Get("delay"); d != "" {
			if parsed, err := time.ParseDuration(d); err == nil {
				delay = parsed
			}
		}
		if delay > 0 {
			time.Sleep(delay)
		}

		// Check for error injection
		if s.Config.ErrorRate > 0 && rand.Float64() < s.Config.ErrorRate {
			http.Error(w, `{"error":"injected error"}`, http.StatusInternalServerError)
			return
		}

		// Check for explicit status override
		if st := r.Header.Get("X-Mock-Status"); st != "" {
			if code, err := strconv.Atoi(st); err == nil {
				w.WriteHeader(code)
				return
			}
		} else if st := r.URL.Query().Get("status"); st != "" {
			if code, err := strconv.Atoi(st); err == nil {
				w.WriteHeader(code)
				return
			}
		}

		mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// Try reading JSON body if present
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      s.Config.AuthToken,
		"expires_in": 3600,
		"user":       req.Email,
	})
}

func (s *Server) checkAuth(r *http.Request) bool {
	if !s.Config.RequireAuth {
		return true
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return token == s.Config.AuthToken
	}
	return false
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		items := make([]Item, 0, len(s.items))
		for _, item := range s.items {
			items = append(items, item)
		}
		_ = json.NewEncoder(w).Encode(items)

	case http.MethodPost:
		var item Item
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		if item.ID == "" {
			item.ID = "item-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		s.items[item.ID] = item
		s.mu.Unlock()

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(item)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleThings(w http.ResponseWriter, r *http.Request) {
	if !s.checkAuth(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		s.mu.Lock()
		calls := s.idempotencyMap[idempotencyKey]
		s.idempotencyMap[idempotencyKey] = calls + 1
		s.mu.Unlock()

		if calls > 0 {
			// Duplicate call
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":           "conflict",
				"idempotency_key": idempotencyKey,
			})
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "created",
		"id":     "thing-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	})
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"method":  r.Method,
		"url":     r.URL.String(),
		"headers": r.Header,
		"body":    string(body),
	})
}

func (s *Server) handleStall(w http.ResponseWriter, r *http.Request) {
	time.Sleep(2 * time.Second)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"stalled"}`))
}
