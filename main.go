package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

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

	ctx := context.Background()

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

	srv := api.NewServer(pool, logger)

	logger.Info("starting server on :8080")
	
	if err := http.ListenAndServe(":8080", srv.Routes()); err != nil {
		logger.Error("server failed", "error", err)
	}

}