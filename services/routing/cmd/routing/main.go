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
	"github.com/city-connector-backend/routing/internal/eta"
	"github.com/city-connector-backend/routing/internal/obs"
	"github.com/city-connector-backend/routing/internal/planner"
	"github.com/city-connector-backend/routing/internal/raptor"
	"github.com/city-connector-backend/routing/internal/timetable"
	"github.com/jackc/pgx/v5/pgxpool"
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

	engine := raptor.NewEngine()
	adjuster := eta.NewNoopAdjuster()
	pl := planner.NewPlanner(cfg, engine, adjuster)

	// Configure static data source: Postgres vs GTFSZip
	var src timetable.Source
	if cfg.Source == config.SourcePostgres && cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Error("failed connecting to postgres", "err", err)
		} else {
			defer pool.Close()
			src = timetable.NewPostgresSource(pool)
		}
	} else if cfg.Source == config.SourceGTFSZip && cfg.GTFSPath != "" {
		src = &timetable.GTFSZipSource{Path: cfg.GTFSPath}
	}

	if src != nil {
		bldr := timetable.NewBuilder(timetable.BuilderConfig{
			WalkSpeedMPS:      cfg.WalkSpeedMPS,
			MaxTransferMeters: cfg.MaxTransferMeters,
			MinTransferSecs:   cfg.MinTransferSecs,
		})
		watcher := timetable.NewWatcher(src, bldr, store, cfg.TimetablePollInterval, log)
		if err := watcher.StartInitialLoad(ctx); err != nil {
			log.Warn("initial static feed load failed (will retry in background)", "err", err)
		}
		go watcher.Run(ctx)
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           api.NewServer(cfg, log, store, pl, reg).Handler(),
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
