package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/expoR93/go-url-short/internal/base62"
	"github.com/expoR93/go-url-short/internal/db"
	"github.com/go-chi/chi/v5"
)

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	ShortURL string `json:"short_url"`
}

func (s *Server) HandleShorten(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.Logger.Error("failed to decode request", "error", err)
		s.respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	id, err := s.Flake.NextID()
	if err != nil {
		s.Logger.Error("failed to generate ID", "error", err)
		s.respondWithError(w, http.StatusInternalServerError, "Internal error")
		return
	}

	shortKey := base62.Encode(id)

	url, err := s.DB.CreateURL(r.Context(), db.CreateURLParams{
		ID:          int64(id),
		OriginalUrl: req.URL,
		ShortKey:    shortKey,
	})
	if err != nil {
		s.Logger.Error("database insertion failed", "error", err, "url", req.URL)
		s.respondWithError(w, http.StatusInternalServerError, "Failed to save URL")
		return
	}

	s.respondWithJSON(w, http.StatusCreated, url)
}

func (s *Server) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortKey := chi.URLParam(r, "key")
	ctx := r.Context()
	if shortKey == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	// Check Redis cache
	originalUrl, err := s.Cache.Get(ctx, shortKey)
	if err == nil {
		http.Redirect(w, r, originalUrl, http.StatusFound)
		return
	}

	// Cache Miss: Check PostgreSQL​
	urlEntry, err := s.DB.GetURL(r.Context(), shortKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Background click increment
	s.WG.Add(1)
	go func(id int64) {
		defer s.WG.Done()
		_ = s.DB.IncrementClick(context.Background(), id)
	}(urlEntry.ID)

	http.Redirect(w, r, urlEntry.OriginalUrl, http.StatusFound)
}

func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	shortKey := chi.URLParam(r, "key")
	if shortKey == "" {
		http.Error(w, "Key is required!", http.StatusBadRequest)
		return
	}

	urlEntry, err := s.DB.GetURL(r.Context(), shortKey)
	if err != nil {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(urlEntry); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

// respondWithJSON is a helper to send consistent JSON responses.
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.Logger.Error("failed to encode json response", "error", err)
	}
}

// respondWithError is a helper to send standardized error messages.
func (s *Server) respondWithError(w http.ResponseWriter, code int, message string) {
	s.respondWithJSON(w, code, map[string]string{"error": message})
}
