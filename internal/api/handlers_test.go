package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/expoR93/go-url-short/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/sony/sonyflake"
)

type MockCache struct {
	OnGet  func(key string) (string, error)
	OnSet  func(key string, value string) error
	OnIncr func(key string, ttl time.Duration) (int64, error)
}

func (m *MockCache) Get(ctx context.Context, key string) (string, error) {
	if m.OnGet != nil {
		return m.OnGet(key)
	}

	return "", errors.New("cache miss")
}

func (m *MockCache) Set(ctx context.Context, key string, value string) error {
	if m.OnSet != nil {
		return m.OnSet(key, value)
	}

	return nil
}

func (m *MockCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if m.OnIncr != nil {
		return m.OnIncr(key, ttl)
	}
	return 1, nil
}

type MockDB struct {
	OnCreate    func(arg db.CreateURLParams) (db.Url, error)
	OnGet       func(key string) (db.Url, error)
	OnIncrement func(id int64) error
}

func (m *MockDB) CreateURL(ctx context.Context, arg db.CreateURLParams) (db.Url, error) {
	return m.OnCreate(arg)
}
func (m *MockDB) GetURL(ctx context.Context, key string) (db.Url, error) {
	return m.OnGet(key)
}
func (m *MockDB) IncrementClick(ctx context.Context, id int64) error {
	return m.OnIncrement(id)
}

func TestHandleShorten_Unified(t *testing.T) {
	tests := []struct {
		name           string
		inputBody      interface{}
		mockErr        error
		expectedStatus int
		expectedErr    string
	}{
		{name: "Valid HTTPS", inputBody: ShortenRequest{URL: "https://google.com"}, expectedStatus: http.StatusCreated},
		{name: "IP Address", inputBody: ShortenRequest{URL: "http://192.168.1.1"}, expectedStatus: http.StatusCreated},
		{name: "Malformed JSON", inputBody: "{ bad }", expectedStatus: http.StatusBadRequest, expectedErr: "Invalid request payload"},
		{name: "DB Error", inputBody: ShortenRequest{URL: "https://x.com"}, mockErr: errors.New("fail"), expectedStatus: http.StatusInternalServerError, expectedErr: "Failed to save URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheCalled := false
			var capturedKey, capturedValue string

			mockCache := &MockCache{
				OnSet: func(key string, value string) error {
					capturedKey = key
					capturedValue = value
					cacheCalled = true
					return nil
				},
			}

			mock := &MockDB{
				OnCreate: func(arg db.CreateURLParams) (db.Url, error) {
					return db.Url{ShortKey: arg.ShortKey}, tt.mockErr
				},
			}

			// Initialize Sonyflake to prevent a nil pointer panic
			flake, err := sonyflake.New(sonyflake.Settings{})
			if err != nil {
				t.Fatal("could not initialize sonyflake for test")
			}

			srv := &Server{
				DB:     mock,
				Cache:  mockCache,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Flake:  flake, // Inject the generator
			}

			var body []byte
			if s, ok := tt.inputBody.(string); ok {
				body = []byte(s)
			} else {
				body, _ = json.Marshal(tt.inputBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBuffer(body))
			w := httptest.NewRecorder()
			srv.HandleShorten(w, req)

			if tt.expectedStatus == http.StatusCreated {
				if !cacheCalled {
					t.Errorf("%s: expected cache to be updated on success, but it wasn't", tt.name)
				}
				// Check if the key exists (is not empty)
				if capturedKey == "" {
					t.Error("expected shortKey to be sent to cache, but it was empty")
				}
				// Check if the value matches the original URL from the input
				input := tt.inputBody.(ShortenRequest)
				if capturedValue != input.URL {
					t.Errorf("cache received wrong URL: got %s, want %s", capturedValue, input.URL)
				}
			}

			if w.Code != tt.expectedStatus {
				t.Errorf("got %d, want %d", w.Code, tt.expectedStatus)
			}

			// Verify JSON error message if status is not OK
			if tt.expectedStatus != http.StatusOK && tt.expectedStatus != http.StatusCreated {
				var resp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if resp["error"] != tt.expectedErr {
					t.Errorf("got error message %q, expected %q", resp["error"], tt.expectedErr)
				}
			}
		})
	}
}

func TestHandleRedirect(t *testing.T) {
	tests := []struct {
		name           string
		urlKey         string
		cacheHit       bool
		expectedStatus int
	}{
		{
			name:           "Cache Hit Redirect",
			urlKey:         "cached-key",
			cacheHit:       true,
			expectedStatus: http.StatusFound,
		},
		{
			name:           "Cache Miss - DB Hit",
			urlKey:         "cached-key",
			cacheHit:       false,
			expectedStatus: http.StatusFound,
		},
		{
			name:           "Successful Redirect",
			urlKey:         "found",
			cacheHit:       false,
			expectedStatus: http.StatusFound, // 302
		},
		{
			name:           "Key Not Found",
			urlKey:         "missing-key",
			cacheHit:       false,
			expectedStatus: http.StatusNotFound, // 404
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var setKey, setVal string

			// Setup MockCache
			mockCache := &MockCache{
				OnGet: func(key string) (string, error) {
					if tt.cacheHit {
						return "https://google.com", nil
					}
					return "", errors.New("redis: nil")
				},
				OnSet: func(key string, value string) error {
					setKey = key
					setVal = value
					return nil
				},
			}

			// Setup MockDB
			mockDB := &MockDB{
				OnGet: func(key string) (db.Url, error) {
					// If the key is "missing-key", explicitly return an error
					if key == "missing-key" {
						return db.Url{}, errors.New("not found")
					}

					return db.Url{
						ID:          1,
						OriginalUrl: "https://google.com",
						ShortKey:    key,
					}, nil
				},
				OnIncrement: func(id int64) error {
					return nil
				},
			}

			srv := &Server{
				DB:     mockDB,
				Cache:  mockCache,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			// 1. Create request with the specific key from the table
			req := httptest.NewRequest(http.MethodGet, "/"+tt.urlKey, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("key", tt.urlKey)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()

			// 2. Execute
			srv.HandleRedirect(w, req)

			srv.WG.Wait()

			// 3. Assert
			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected %d, got %d", tt.name, tt.expectedStatus, w.Code)
			}
			if !tt.cacheHit && tt.expectedStatus == http.StatusFound {
				if setKey != tt.urlKey {
					t.Errorf("expected cache to be filled with %s, but got %s", tt.urlKey, setKey)
				}
				expectedURL := "https://google.com"
				if setVal != expectedURL {
					t.Errorf("%s: expected cache value %s, got %s", tt.name, expectedURL, setVal)
				}
			}
		})
	}
}

func TestHandleStats(t *testing.T) {
	mock := &MockDB{
		OnGet: func(key string) (db.Url, error) {
			return db.Url{ShortKey: key, Clicks: 10}, nil
		},
	}
	srv := &Server{DB: mock, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	req := httptest.NewRequest(http.MethodGet, "/stats/abc", nil)
	// Inject chi context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	srv.HandleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		mockCount      int64
		expectedStatus int
	}{
		{
			name:           "Under Limit",
			mockCount:      10,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Over Limit",
			mockCount:      61,
			expectedStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache := &MockCache{
				OnIncr: func(key string, ttl time.Duration) (int64, error) {
					return tt.mockCount, nil
				},
			}

			srv := &Server{
				Cache:  mockCache,
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			// Dummy handler to wrap with middleware
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()

			// Execute the middleware
			srv.RateLimitMiddleware(nextHandler).ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.name, tt.expectedStatus, w.Code)
			}
		})
	}
}
