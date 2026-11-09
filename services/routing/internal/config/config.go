// Package config loads the routing service configuration from environment
// variables. All tunables live here so that defaults are documented in one
// place (see routing engine plan §8).
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Source selects where the timetable is loaded from.
type Source string

const (
	// SourcePostgres loads the active feed version(s) from the static GTFS DB.
	SourcePostgres Source = "postgres"
	// SourceGTFSZip loads a GTFS zip/directory from disk (local dev and tests).
	SourceGTFSZip Source = "gtfszip"
)

// Config is the fully resolved service configuration.
type Config struct {
	Port     string
	LogLevel slog.Level

	// Timetable loading.
	Source                Source
	DatabaseURL           string
	GTFSPath              string
	TimetablePollInterval time.Duration

	// Walking model (plan §4.1).
	WalkSpeedMPS      float64
	MaxTransferMeters int
	MinTransferSecs   int

	// Journey query defaults and limits (tech spec §3).
	DefaultMaxTransfers  int
	MaxMaxTransfers      int
	DefaultMaxWalkMeters int
	MaxMaxWalkMeters     int

	// Realtime ETA adjustment (Phase 3, tech spec §7).
	RealtimeURL     string
	RealtimeTimeout time.Duration

	ShutdownTimeout time.Duration
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	p := parser{getenv: getenv}
	cfg := Config{
		Port:                  p.str("PORT", "8081"),
		LogLevel:              p.level("LOG_LEVEL", slog.LevelInfo),
		Source:                Source(strings.ToLower(p.str("ROUTING_SOURCE", string(SourcePostgres)))),
		DatabaseURL:           p.str("DATABASE_URL", ""),
		GTFSPath:              p.str("ROUTING_GTFS_PATH", ""),
		TimetablePollInterval: p.duration("TIMETABLE_POLL_INTERVAL", 60*time.Second),
		WalkSpeedMPS:          p.float("WALK_SPEED_MPS", 1.33),
		MaxTransferMeters:     p.int("MAX_TRANSFER_METERS", 500),
		MinTransferSecs:       p.int("MIN_TRANSFER_SECS", 60),
		DefaultMaxTransfers:   p.int("DEFAULT_MAX_TRANSFERS", 3),
		MaxMaxTransfers:       p.int("MAX_MAX_TRANSFERS", 5),
		DefaultMaxWalkMeters:  p.int("DEFAULT_MAX_WALK_METERS", 800),
		MaxMaxWalkMeters:      p.int("MAX_MAX_WALK_METERS", 2000),
		RealtimeURL:           p.str("REALTIME_URL", ""),
		RealtimeTimeout:       p.duration("REALTIME_TIMEOUT", 150*time.Millisecond),
		ShutdownTimeout:       p.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}
	if p.err != nil {
		return Config{}, p.err
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch c.Source {
	case SourcePostgres, SourceGTFSZip:
	default:
		return fmt.Errorf("config: ROUTING_SOURCE must be %q or %q, got %q", SourcePostgres, SourceGTFSZip, c.Source)
	}
	if c.WalkSpeedMPS <= 0 {
		return fmt.Errorf("config: WALK_SPEED_MPS must be > 0")
	}
	if c.TimetablePollInterval <= 0 {
		return fmt.Errorf("config: TIMETABLE_POLL_INTERVAL must be > 0")
	}
	if c.DefaultMaxTransfers < 0 || c.DefaultMaxTransfers > c.MaxMaxTransfers {
		return fmt.Errorf("config: DEFAULT_MAX_TRANSFERS must be within [0, MAX_MAX_TRANSFERS]")
	}
	if c.DefaultMaxWalkMeters < 0 || c.DefaultMaxWalkMeters > c.MaxMaxWalkMeters {
		return fmt.Errorf("config: DEFAULT_MAX_WALK_METERS must be within [0, MAX_MAX_WALK_METERS]")
	}
	return nil
}

// parser accumulates the first parse error so Load can report it once.
type parser struct {
	getenv func(string) string
	err    error
}

func (p *parser) str(key, def string) string {
	if v := strings.TrimSpace(p.getenv(key)); v != "" {
		return v
	}
	return def
}

func (p *parser) int(key string, def int) int {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return n
}

func (p *parser) float(key string, def float64) float64 {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return f
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.fail(key, v, err)
		return def
	}
	return d
}

func (p *parser) level(key string, def slog.Level) slog.Level {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		p.fail(key, v, err)
		return def
	}
	return l
}

func (p *parser) fail(key, val string, err error) {
	if p.err == nil {
		p.err = fmt.Errorf("config: invalid %s=%q: %w", key, val, err)
	}
}

