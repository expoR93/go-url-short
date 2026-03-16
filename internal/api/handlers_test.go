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

	"github.com/expoR93/go-url-short/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/sony/sonyflake"
)

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
		expectedStatus int
	}{
		{
			name:           "Successful Redirect",
			urlKey:         "found",
			expectedStatus: http.StatusFound, // 302
		},
		{
			name:           "Key Not Found",
			urlKey:         "missing-key",
			expectedStatus: http.StatusNotFound, // 404
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Mock
			mock := &MockDB{
				OnGet: func(key string) (db.Url, error) {
					if key == "found" {
						return db.Url{ID: 1, OriginalUrl: "https://google.com"}, nil
					}
					return db.Url{}, errors.New("not found")
				},
			}

			srv := &Server{
				DB:     mock,
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

			// 3. Assert
			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected %d, got %d", tt.name, tt.expectedStatus, w.Code)
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
