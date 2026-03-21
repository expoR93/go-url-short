package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/expoR93/go-url-short/internal/cache"
	"github.com/expoR93/go-url-short/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/sonyflake"
)

// URLRepository defines the behavior the Server needs from the database.
type URLRepository interface {
	CreateURL(ctx context.Context, arg db.CreateURLParams) (db.Url, error)
	GetURL(ctx context.Context, shortKey string) (db.Url, error)
	IncrementClick(ctx context.Context, id int64) error
}

type CacheRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

type Server struct {
	DB     URLRepository
	Cache  CacheRepository
	Pool   *pgxpool.Pool
	Logger *slog.Logger
	WG     sync.WaitGroup
	Flake  *sonyflake.Sonyflake
}

func NewServer(pool *pgxpool.Pool, logger *slog.Logger) *Server {
	flake, err := sonyflake.New(sonyflake.Settings{})
	if err != nil {
		logger.Error("could not initialize sonyflake ID generator")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Default fallback
		logger.Info("REDIS_ADDR not set, falling back to default", "addr", redisAddr)
	}

	redisPass := os.Getenv("REDIS_PASS")

	redisDBStr := os.Getenv("REDIS_DB")
	redisDB := 0 // Default to DB 0
	if redisDBStr != "" {
		if val, err := strconv.Atoi(redisDBStr); err == nil {
			redisDB = val
		}
	}

	cacheStore := cache.NewRedisStore(redisAddr, redisPass, redisDB, 24*time.Hour)

	return &Server{
		DB:     db.New(pool),
		Cache:  cacheStore,
		Pool:   pool,
		Logger: logger,
		Flake:  flake,
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// --- Middleware Stack ---​
	r.Use(middleware.Logger)    // Log every request to the terminal
	r.Use(middleware.Recoverer) // If a handler panics, don't crash the server
	r.Use(middleware.RealIP)    // Useful for tracking actual user IPs in stats later​
	r.Use(s.RateLimitMiddleware)

	// --- Routes ---​
	r.Post("/shorten", s.HandleShorten)
	r.Get("/{key}", s.HandleRedirect)
	r.Get("/stats/{key}", s.HandleStats)

	return r
}

// RateLimitMiddleware restricts requests based on IP address using the Redis cache.
func (s *Server) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		key := "rate_limit:" + r.RemoteAddr

		// Atomic increment with a 1-minute window
		count, err := s.Cache.Incr(ctx, key, time.Minute)
		if err != nil {
			s.Logger.Error("cache error in rate limiter", "error", err)
			next.ServeHTTP(w, r) // Fail open to not block users if Redis is down
			return
		}

		if count > 60 {
			s.Logger.Warn("rate limit exceeded", "ip", r.RemoteAddr, "count", count)
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}