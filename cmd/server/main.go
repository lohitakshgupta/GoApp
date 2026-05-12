package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lohitakshgupta/GoApp/internal/api"
	"github.com/lohitakshgupta/GoApp/internal/job"
	"github.com/lohitakshgupta/GoApp/internal/observability"
	"github.com/lohitakshgupta/GoApp/internal/storage"
	"github.com/lohitakshgupta/GoApp/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := configFromEnv()

	repo := storage.NewMemoryStore(time.Now)
	metrics := observability.NewMetrics()
	processor := job.NewDemoProcessor(logger)
	service := job.NewService(repo, processor, metrics, logger)
	pool := worker.NewPool(cfg.Workers, service, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool.Start(ctx)

	router := api.NewRouter(api.Config{
		Service: service,
		Pool:    pool,
		Metrics: metrics,
		Logger:  logger,
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "workers", cfg.Workers)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	pool.Stop(shutdownCtx)
	logger.Info("shutdown complete")
	return nil
}

type config struct {
	Addr            string
	Workers         int
	ShutdownTimeout time.Duration
}

func configFromEnv() config {
	return config{
		Addr:            getenv("ADDR", ":8080"),
		Workers:         getenvInt("WORKERS", 4),
		ShutdownTimeout: time.Duration(getenvInt("SHUTDOWN_TIMEOUT_SECONDS", 10)) * time.Second,
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
