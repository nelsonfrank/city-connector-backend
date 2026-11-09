// Command routing runs the Routing Service: journey search over an in-memory
// RAPTOR timetable (ADR-002). main only composes components and manages the
// process lifecycle; domain logic lives under internal/.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/city-connector-backend/routing/internal/api"
	"github.com/city-connector-backend/routing/internal/config"
	"github.com/city-connector-backend/routing/internal/obs"
	"github.com/city-connector-backend/routing/internal/timetable"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "routing:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := obs.NewLogger(os.Stdout, cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg := obs.NewRegistry()
	store := timetable.NewStore()
	// TODO(M1/M5): start the timetable Watcher here; it loads the active feed
	// version from cfg.Source and publishes it via store.Swap. Until then the
	// service stays not-ready (/readyz=503, /journey=503) by design.

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           api.NewServer(log, store, reg).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("routing service listening", "port", cfg.Port, "source", cfg.Source)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", cfg.ShutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
