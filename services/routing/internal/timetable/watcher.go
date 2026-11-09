package timetable

import (
	"context"
	"log/slog"
	"time"
)

// Watcher periodically checks the static feed source and triggers hot-swaps on version updates.
type Watcher struct {
	source   *GTFSZipSource
	builder  *Builder
	store    *Store
	interval time.Duration
	log      *slog.Logger
}

func NewWatcher(src *GTFSZipSource, builder *Builder, store *Store, interval time.Duration, log *slog.Logger) *Watcher {
	return &Watcher{
		source:   src,
		builder:  builder,
		store:    store,
		interval: interval,
		log:      log,
	}
}

// StartInitialLoad synchronously performs the first load so readiness gates are passed.
func (w *Watcher) StartInitialLoad() error {
	raw, err := w.source.Load()
	if err != nil {
		return err
	}
	tt, err := w.builder.Build(raw)
	if err != nil {
		return err
	}
	w.store.Swap(tt)
	w.log.Info("initial timetable loaded",
		"version", tt.Version,
		"stops", len(tt.Stops),
		"patterns", len(tt.Patterns),
		"trips", len(tt.Trips),
	)
	return nil
}

// Run starts the background watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// For GTFSZipSource, we can re-evaluate on version change or modtime
			// Future PostgresSource checks active version in feed_versions
		}
	}
}
