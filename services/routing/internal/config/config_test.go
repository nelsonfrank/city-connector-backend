package config

import (
	"log/slog"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != "8081" || cfg.Source != SourcePostgres || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.DefaultMaxTransfers != 3 || cfg.DefaultMaxWalkMeters != 800 {
		t.Errorf("tech-spec defaults not applied: %+v", cfg)
	}
	if cfg.RealtimeTimeout != 150*time.Millisecond {
		t.Errorf("realtime timeout = %v, want 150ms", cfg.RealtimeTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"PORT":                    "9000",
		"LOG_LEVEL":               "debug",
		"ROUTING_SOURCE":          "GTFSZIP",
		"ROUTING_GTFS_PATH":       "/data/mbta.zip",
		"TIMETABLE_POLL_INTERVAL": "5s",
		"WALK_SPEED_MPS":          "1.1",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != "9000" || cfg.LogLevel != slog.LevelDebug || cfg.Source != SourceGTFSZip ||
		cfg.GTFSPath != "/data/mbta.zip" || cfg.TimetablePollInterval != 5*time.Second || cfg.WalkSpeedMPS != 1.1 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"bad int":       {"MAX_TRANSFER_METERS": "far"},
		"bad duration":  {"TIMETABLE_POLL_INTERVAL": "soon"},
		"bad level":     {"LOG_LEVEL": "loud"},
		"bad source":    {"ROUTING_SOURCE": "s3"},
		"bad walk":      {"WALK_SPEED_MPS": "0"},
		"bad transfers": {"DEFAULT_MAX_TRANSFERS": "9"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := load(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

