package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/expoR93/go-url-short/internal/api"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Load .env file
	if err := godotenv.Load(); err != nil {
		logger.Info("No .env file found, falling back to system environment variables")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Get the connection string from an environment variable
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		logger.Error("DATABASE_URL environment variable is not set")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		logger.Error("unable to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Initialize custom Server struct
	app := api.NewServer(pool, logger)

	// Define the standard http.Server with timeouts
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      app.Routes(),
		ReadTimeout:  5 * time.Second,   // Max time to read the entire request
		WriteTimeout: 10 * time.Second,  // Max time to write the response
		IdleTimeout:  120 * time.Second, // Max time for keep-alive connections
	}

	// Start server in a background goroutine so it doesn't block the shutdown signal
	go func() {
		logger.Info("starting server on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for the interrupt signal (Ctrl+C)
	<-ctx.Done()
	logger.Info("shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", "error", err)
	}

	// Wait for background tasks (like click increments) to finish
	app.WG.Wait()

	logger.Info("server stopped")

}
