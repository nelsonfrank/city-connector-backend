package gtfs

import (
	"fmt"
	"time"
)

// Seconds is a GTFS time of day: seconds since the start of the service day
// ("noon minus 12h", see ServiceDayStart). Values may exceed 24h for trips
// that run past midnight (e.g. "25:10:00").
type Seconds = int32

// NoTime marks a missing arrival/departure (non-timepoint stop_times).
const NoTime Seconds = -1

// ParseTime parses "H:MM:SS" or "HH:MM:SS" (hours may be >= 24).
// An empty string returns NoTime and no error.
func ParseTime(s string) (Seconds, error) {
	if s == "" {
		return NoTime, nil
	}
	var h, m, sec int32
	i := 0
	readExactDigits := func(digits int) (int32, bool) {
		start := i
		var n int32
		for i < len(s) && s[i] >= '0' && s[i] <= '9' && i-start < digits {
			n = n*10 + int32(s[i]-'0')
			i++
		}
		return n, i-start == digits
	}
	readHours := func() (int32, bool) {
		start := i
		var n int32
		for i < len(s) && s[i] >= '0' && s[i] <= '9' && i-start < 3 {
			n = n*10 + int32(s[i]-'0')
			i++
		}
		digits := i - start
		return n, digits == 1 || digits == 2 || digits == 3
	}
	// Some producers pad with a leading space.
	for i < len(s) && s[i] == ' ' {
		i++
	}
	var ok bool
	if h, ok = readHours(); !ok || i >= len(s) || s[i] != ':' {
		return NoTime, fmt.Errorf("gtfs: invalid time %q", s)
	}
	i++
	if m, ok = readExactDigits(2); !ok || i >= len(s) || s[i] != ':' || m > 59 {
		return NoTime, fmt.Errorf("gtfs: invalid time %q", s)
	}
	i++
	if sec, ok = readExactDigits(2); !ok || i != len(s) || sec > 59 {
		return NoTime, fmt.Errorf("gtfs: invalid time %q", s)
	}
	return h*3600 + m*60 + sec, nil
}

// FormatTime renders seconds as "HH:MM:SS" (hours may be >= 24).
func FormatTime(t Seconds) string {
	if t < 0 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d:%02d", t/3600, t/60%60, t%60)
}

// Date is a civil service date, stored as days since 1970-01-01.
type Date int32

// ParseDate parses a GTFS "YYYYMMDD" date.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("20060102", s)
	if err != nil {
		return 0, fmt.Errorf("gtfs: invalid date %q", s)
	}
	return DateOf(t), nil
}

// DateOf returns the civil date of t in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// AddDays returns d shifted by n days.
func (d Date) AddDays(n int) Date { return d + Date(n) }

// Weekday returns the day of the week (1970-01-01 was a Thursday).
func (d Date) Weekday() time.Weekday {
	return time.Weekday(((int(d)+4)%7 + 7) % 7)
}

// civil returns the date at UTC midnight (for formatting/components).
func (d Date) civil() time.Time { return time.Unix(int64(d)*86400, 0).UTC() }

// String renders "YYYYMMDD".
func (d Date) String() string { return d.civil().Format("20060102") }

// ServiceDayStart returns the instant that GTFS times on service date d are
// relative to: "noon minus 12h" in loc. This equals local midnight except on
// DST transition days, which is exactly how the GTFS spec defines it.
func ServiceDayStart(d Date, loc *time.Location) time.Time {
	c := d.civil()
	return time.Date(c.Year(), c.Month(), c.Day(), 12, 0, 0, 0, loc).Add(-12 * time.Hour)
}

// Instant converts (service date, seconds) into an absolute time.
func Instant(d Date, t Seconds, loc *time.Location) time.Time {
	return ServiceDayStart(d, loc).Add(time.Duration(t) * time.Second)
}

