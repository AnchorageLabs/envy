package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AnchorageLabs/envy/api/internal/server"
)

const shutdownTimeout = 5 * time.Second

type config struct {
	addr        string
	databaseURL string
	logLevel    slog.Level
}

func main() {
	cfg := loadConfig()
	logger := newLogger(cfg.logLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{
		Addr:    cfg.addr,
		Handler: server.NewRouter(),
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", cfg.addr, "db_url_set", cfg.databaseURL != "")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
		stop()
		logger.Info("shutting down", "reason", ctx.Err())
	case err := <-serverErr:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	if err := <-serverErr; err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func loadConfig() config {
	addr := strings.TrimSpace(os.Getenv("ENVY_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	return config{
		addr:        addr,
		databaseURL: os.Getenv("ENVY_DB_URL"),
		logLevel:    parseLogLevel(os.Getenv("ENVY_LOG_LEVEL")),
	}
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
