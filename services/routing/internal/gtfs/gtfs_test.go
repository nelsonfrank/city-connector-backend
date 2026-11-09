package gtfs

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	cases := map[string]Seconds{
		"":         NoTime,
		"00:00:00": 0,
		"8:05:09":  8*3600 + 5*60 + 9,
		"08:05:09": 8*3600 + 5*60 + 9,
		"25:10:00": 25*3600 + 10*60,
		" 7:00:00": 7 * 3600,
		"100:00:00": 100 * 3600,
	}
	for in, want := range cases {
		got, err := ParseTime(in)
		if err != nil || got != want {
			t.Errorf("ParseTime(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"8:5:00", "08:60:00", "08:00:60", "08:00", "aa:00:00", "08:00:00x", "1000:00:00"} {
		if _, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%q) should fail", bad)
		}
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(25*3600 + 61); got != "25:01:01" {
		t.Errorf("FormatTime = %q", got)
	}
	if FormatTime(NoTime) != "" {
		t.Error("NoTime should format as empty")
	}
}

func TestDate(t *testing.T) {
	d, err := ParseDate("20261109")
	if err != nil {
		t.Fatal(err)
	}
	if d.String() != "20261109" {
		t.Errorf("String = %s", d)
	}
	if d.Weekday() != time.Monday {
		t.Errorf("2026-11-09 weekday = %v, want Monday", d.Weekday())
	}
	if d.AddDays(1).String() != "20261110" {
		t.Error("AddDays")
	}
	if Date(0).Weekday() != time.Thursday {
		t.Error("epoch weekday")
	}
	if _, err := ParseDate("2026-11-09"); err == nil {
		t.Error("expected error for dashed date")
	}
	loc, _ := time.LoadLocation("Africa/Nairobi")
	if got := DateOf(time.Date(2026, 11, 9, 23, 30, 0, 0, loc)); got != d {
		t.Errorf("DateOf = %s", got)
	}
}

func TestServiceDayStartDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata not available:", err)
	}
	// 2026-03-08: US DST starts (02:00 → 03:00). Noon-12h is 23:00 the day before.
	d, _ := ParseDate("20260308")
	start := ServiceDayStart(d, ny)
	if want := time.Date(2026, 3, 7, 23, 0, 0, 0, ny); !start.Equal(want) {
		t.Errorf("DST start day: got %v want %v", start, want)
	}
	// A normal day anchors at local midnight.
	d2, _ := ParseDate("20261109")
	if want := time.Date(2026, 11, 9, 0, 0, 0, 0, ny); !ServiceDayStart(d2, ny).Equal(want) {
		t.Errorf("normal day: got %v", ServiceDayStart(d2, ny))
	}
	// 12:00:00 is always local noon.
	if got := Instant(d, 12*3600, ny); got.Hour() != 12 {
		t.Errorf("noon on DST day = %v", got)
	}
}

func TestNamespace(t *testing.T) {
	if Namespace("MBTA", "place-sstat") != "MBTA:place-sstat" {
		t.Error("Namespace")
	}
	if Namespace("", "x") != "x" || Namespace("A", "") != "" {
		t.Error("Namespace passthrough")
	}
	a, l, err := SplitID("MBTA:70061")
	if err != nil || a != "MBTA" || l != "70061" {
		t.Errorf("SplitID = %q %q %v", a, l, err)
	}
	if _, _, err := SplitID("70061"); err == nil {
		t.Error("SplitID should fail without namespace")
	}
}

