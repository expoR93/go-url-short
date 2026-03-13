package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/expoR93/go-url-short/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// URLRepository defines the behavior the Server needs from the database.
type URLRepository interface {
	CreateURL(ctx context.Context, arg db.CreateURLParams) (db.Url, error)
	GetURL(ctx context.Context, shortKey string) (db.Url, error)
	IncrementClick(ctx context.Context, id int64) error
}

type Server struct {
	DB     URLRepository
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	WG     sync.WaitGroup
}

func NewServer(pool *pgxpool.Pool, logger *slog.Logger) *Server {
	return &Server{
		DB:     db.New(pool),
		Pool:   pool,
		Logger: logger,
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// --- Middleware Stack ---​
	r.Use(middleware.Logger)    // Log every request to the terminal
	r.Use(middleware.Recoverer) // If a handler panics, don't crash the server
	r.Use(middleware.RealIP)    // Useful for tracking actual user IPs in stats later​

	// --- Routes ---​
	r.Post("/shorten", s.HandleShorten)
	r.Get("/{key}", s.HandleRedirect)
	r.Get("/stats/{key}", s.HandleStats)

	return r
}
