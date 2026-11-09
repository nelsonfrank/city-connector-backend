package timetable

import (
	"context"
	"fmt"
	"strings"

	"github.com/city-connector-backend/routing/internal/gtfs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSource reads active static GTFS feeds from PostgreSQL using pgx.
type PostgresSource struct {
	pool *pgxpool.Pool
}

func NewPostgresSource(pool *pgxpool.Pool) *PostgresSource {
	return &PostgresSource{pool: pool}
}

// ActiveVersion queries feed_versions for all currently active versions.
func (s *PostgresSource) ActiveVersion(ctx context.Context) (string, error) {
	rows, err := s.pool.Query(ctx, "SELECT id FROM feed_versions WHERE is_active = TRUE ORDER BY id ASC")
	if err != nil {
		return "", fmt.Errorf("active versions query: %w", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var vid string
		if err := rows.Scan(&vid); err != nil {
			return "", err
		}
		versions = append(versions, vid)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no active feed version found in feed_versions")
	}
	return strings.Join(versions, "+"), nil
}

// Load loads all records for active feed versions from Postgres into RawFeed.
func (s *PostgresSource) Load(ctx context.Context) (*RawFeed, error) {
	activeVersionStr, err := s.ActiveVersion(ctx)
	if err != nil {
		return nil, err
	}
	vids := strings.Split(activeVersionStr, "+")

	feed := &RawFeed{
		Version: activeVersionStr,
	}

	// 1. Agencies
	agencyQuery := `
		SELECT agency_id, agency_name, agency_timezone 
		FROM gtfs_agencies 
		WHERE feed_version_id = ANY($1)
	`
	aRows, err := s.pool.Query(ctx, agencyQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load agencies: %w", err)
	}
	defer aRows.Close()
	for aRows.Next() {
		var a RawAgency
		if err := aRows.Scan(&a.ID, &a.Name, &a.Timezone); err != nil {
			return nil, err
		}
		feed.Agencies = append(feed.Agencies, a)
	}

	// 2. Stops
	stopQuery := `
		SELECT stop_id, stop_name, stop_lat, stop_lon 
		FROM gtfs_stops 
		WHERE feed_version_id = ANY($1)
	`
	sRows, err := s.pool.Query(ctx, stopQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load stops: %w", err)
	}
	defer sRows.Close()
	for sRows.Next() {
		var st RawStop
		if err := sRows.Scan(&st.ID, &st.Name, &st.Lat, &st.Lon); err != nil {
			return nil, err
		}
		feed.Stops = append(feed.Stops, st)
	}

	// 3. Routes
	routeQuery := `
		SELECT route_id, agency_id, COALESCE(route_short_name, ''), COALESCE(route_long_name, ''), route_type 
		FROM gtfs_routes 
		WHERE feed_version_id = ANY($1)
	`
	rRows, err := s.pool.Query(ctx, routeQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load routes: %w", err)
	}
	defer rRows.Close()
	for rRows.Next() {
		var r RawRoute
		if err := rRows.Scan(&r.ID, &r.AgencyID, &r.ShortName, &r.LongName, &r.Type); err != nil {
			return nil, err
		}
		feed.Routes = append(feed.Routes, r)
	}

	// 4. Calendars
	calQuery := `
		SELECT service_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday, start_date, end_date
		FROM gtfs_calendar
		WHERE feed_version_id = ANY($1)
	`
	cRows, err := s.pool.Query(ctx, calQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load calendars: %w", err)
	}
	defer cRows.Close()
	for cRows.Next() {
		var c RawCalendar
		var sDate, eDate string
		if err := cRows.Scan(&c.ServiceID, &c.Monday, &c.Tuesday, &c.Wednesday, &c.Thursday, &c.Friday, &c.Saturday, &c.Sunday, &sDate, &eDate); err != nil {
			return nil, err
		}
		c.StartDate, _ = gtfs.ParseDate(sDate)
		c.EndDate, _ = gtfs.ParseDate(eDate)
		feed.Calendars = append(feed.Calendars, c)
	}

	// 5. Calendar Dates
	cdQuery := `
		SELECT service_id, date, exception_type
		FROM gtfs_calendar_dates
		WHERE feed_version_id = ANY($1)
	`
	cdRows, err := s.pool.Query(ctx, cdQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load calendar dates: %w", err)
	}
	defer cdRows.Close()
	for cdRows.Next() {
		var cd RawCalendarDate
		var dStr string
		if err := cdRows.Scan(&cd.ServiceID, &dStr, &cd.ExceptionType); err != nil {
			return nil, err
		}
		cd.Date, _ = gtfs.ParseDate(dStr)
		feed.CalendarDates = append(feed.CalendarDates, cd)
	}

	// 6. Trips
	tripQuery := `
		SELECT trip_id, route_id, service_id, COALESCE(trip_headsign, ''), COALESCE(direction_id, 0)
		FROM gtfs_trips
		WHERE feed_version_id = ANY($1)
	`
	tRows, err := s.pool.Query(ctx, tripQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load trips: %w", err)
	}
	defer tRows.Close()
	for tRows.Next() {
		var tr RawTrip
		if err := tRows.Scan(&tr.ID, &tr.RouteID, &tr.ServiceID, &tr.Headsign, &tr.Direction); err != nil {
			return nil, err
		}
		feed.Trips = append(feed.Trips, tr)
	}

	// 7. Stop Times
	stQuery := `
		SELECT trip_id, stop_id, stop_sequence, arrival_time, departure_time
		FROM gtfs_stop_times
		WHERE feed_version_id = ANY($1)
	`
	stRows, err := s.pool.Query(ctx, stQuery, vids)
	if err != nil {
		return nil, fmt.Errorf("load stop times: %w", err)
	}
	defer stRows.Close()
	for stRows.Next() {
		var st RawStopTime
		var arrStr, depStr string
		if err := stRows.Scan(&st.TripID, &st.StopID, &st.StopSequence, &arrStr, &depStr); err != nil {
			return nil, err
		}
		st.ArrTime, _ = gtfs.ParseTime(arrStr)
		st.DepTime, _ = gtfs.ParseTime(depStr)
		feed.StopTimes = append(feed.StopTimes, st)
	}

	// 8. Transfers
	trQuery := `
		SELECT from_stop_id, to_stop_id, transfer_type, COALESCE(min_transfer_time, 0)
		FROM gtfs_transfers
		WHERE feed_version_id = ANY($1)
	`
	trRows, err := s.pool.Query(ctx, trQuery, vids)
	if err == nil {
		defer trRows.Close()
		for trRows.Next() {
			var tr RawTransfer
			if err := trRows.Scan(&tr.FromStopID, &tr.ToStopID, &tr.TransferType, &tr.MinTransferTime); err == nil {
				feed.Transfers = append(feed.Transfers, tr)
			}
		}
	}

	// 9. Frequencies
	fQuery := `
		SELECT trip_id, start_time, end_time, headway_secs, COALESCE(exact_times, 0)
		FROM gtfs_frequencies
		WHERE feed_version_id = ANY($1)
	`
	fRows, err := s.pool.Query(ctx, fQuery, vids)
	if err == nil {
		defer fRows.Close()
		for fRows.Next() {
			var f RawFrequency
			var startStr, endStr string
			if err := fRows.Scan(&f.TripID, &startStr, &endStr, &f.HeadwaySecs, &f.ExactTimes); err == nil {
				f.StartTime, _ = gtfs.ParseTime(startStr)
				f.EndTime, _ = gtfs.ParseTime(endStr)
				feed.Frequencies = append(feed.Frequencies, f)
			}
		}
	}

	return feed, nil
}
