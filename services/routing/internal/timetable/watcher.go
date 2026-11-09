package timetable

import (
	"context"
	"log/slog"
	"time"
)

// Watcher periodically checks the static feed source and triggers hot-swaps on version updates.
type Watcher struct {
	source   Source
	builder  *Builder
	store    *Store
	interval time.Duration
	log      *slog.Logger
}

func NewWatcher(src Source, builder *Builder, store *Store, interval time.Duration, log *slog.Logger) *Watcher {
	return &Watcher{
		source:   src,
		builder:  builder,
		store:    store,
		interval: interval,
		log:      log,
	}
}

// StartInitialLoad synchronously performs the first load so readiness gates are passed.
func (w *Watcher) StartInitialLoad(ctx context.Context) error {
	raw, err := w.source.Load(ctx)
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

// Run starts the background watcher loop for automatic blue-green timetable updates.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := w.store.Current()
			currentVersion := ""
			if current != nil {
				currentVersion = current.Version
			}

			activeVersion, err := w.source.ActiveVersion(ctx)
			if err != nil {
				w.log.Warn("check active feed version failed", "err", err)
				continue
			}

			if activeVersion != "" && activeVersion != currentVersion {
				w.log.Info("new static feed version detected, building timetable",
					"current", currentVersion,
					"new", activeVersion,
				)
				raw, err := w.source.Load(ctx)
				if err != nil {
					w.log.Error("failed loading new feed version", "err", err)
					continue
				}
				newTT, err := w.builder.Build(raw)
				if err != nil {
					w.log.Error("failed compiling new timetable, keeping old version", "err", err)
					continue
				}
				w.store.Swap(newTT)
				w.log.Info("timetable hot-swapped successfully",
					"version", newTT.Version,
					"stops", len(newTT.Stops),
					"trips", len(newTT.Trips),
				)
			}
		}
	}
}

