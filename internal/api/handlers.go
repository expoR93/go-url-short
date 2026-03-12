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
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Start a transaction using pgx
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Wrap the generated queries with this specific transaction
	qtx := s.Queries.WithTx(tx)

	// Get next ID (using the database sequence)
	var nextID int64
	err = tx.QueryRow(r.Context(), "SELECT nextval('urls_id_seq')").Scan(&nextID)
	if err != nil {
		http.Error(w, "Failed to generate ID", http.StatusInternalServerError)
		return
	}

	shortKey := base62.Encode(nextID)

	_, err = qtx.CreateURL(r.Context(), db.CreateURLParams{
		OriginalUrl: req.URL,
		ShortKey:    shortKey,
	})
	if err != nil {
		http.Error(w, "Failed to save to database", http.StatusInternalServerError)
		return
	}

	// Commit the transaction to make changes permanent
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// Respond to client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ShortenResponse{ShortURL: shortKey})
}

func (s *Server) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortKey := chi.URLParam(r, "key")
	if shortKey == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	urlEntry, err := s.Queries.GetURL(r.Context(), shortKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Background click increment
	s.WG.Add(1)
	go func(id int64) {
		defer s.WG.Done()
		err := s.Queries.IncrementClick(context.Background(), id)
		if err != nil {
			s.Logger.Error("failed to increment click",
				"url_id", id,
				"error", err,
			)
		}
	}(urlEntry.ID)

	http.Redirect(w, r, urlEntry.OriginalUrl, http.StatusFound)
}

func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	shortKey := chi.URLParam(r, "key")
	if shortKey == "" {
		http.Error(w, "Key is required!", http.StatusBadRequest)
		return
	}

	urlEntry, err := s.Queries.GetURL(context.Background(), shortKey)
	if err != nil {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(urlEntry); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}
