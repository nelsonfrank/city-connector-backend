package timetable

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/city-connector-backend/routing/internal/gtfs"
)

// GTFSZipSource reads GTFS static files directly from a .zip or an unpacked directory.
type GTFSZipSource struct {
	Path           string
	DefaultAgency  string // Prefix to prepend if not already namespaced
}

func (s *GTFSZipSource) Load(ctx context.Context) (*RawFeed, error) {
	fi, err := os.Stat(s.Path)
	if err != nil {
		return nil, fmt.Errorf("gtfs source: %w", err)
	}

	feed := &RawFeed{
		Version: filepath.Base(s.Path),
	}

	if fi.IsDir() {
		return s.loadDir(feed)
	}
	return s.loadZip(feed)
}

func (s *GTFSZipSource) ActiveVersion(ctx context.Context) (string, error) {
	fi, err := os.Stat(s.Path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%d", filepath.Base(s.Path), fi.ModTime().Unix()), nil
}

func (s *GTFSZipSource) loadZip(feed *RawFeed) (*RawFeed, error) {
	zr, err := zip.OpenReader(s.Path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	files := make(map[string]*zip.File)
	for _, f := range zr.File {
		files[filepath.Base(f.Name)] = f
	}

	openTable := func(name string) (io.ReadCloser, error) {
		f, ok := files[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return f.Open()
	}

	if err := s.parseAll(feed, openTable); err != nil {
		return nil, err
	}
	return feed, nil
}

func (s *GTFSZipSource) loadDir(feed *RawFeed) (*RawFeed, error) {
	openTable := func(name string) (io.ReadCloser, error) {
		p := filepath.Join(s.Path, name)
		return os.Open(p)
	}
	if err := s.parseAll(feed, openTable); err != nil {
		return nil, err
	}
	return feed, nil
}

func (s *GTFSZipSource) parseAll(feed *RawFeed, openTable func(string) (io.ReadCloser, error)) error {
	// Parse agency.txt (required)
	if r, err := openTable("agency.txt"); err == nil {
		defer r.Close()
		if err := s.parseAgencies(feed, r); err != nil {
			return fmt.Errorf("agency.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Parse stops.txt (required)
	if r, err := openTable("stops.txt"); err == nil {
		defer r.Close()
		if err := s.parseStops(feed, r); err != nil {
			return fmt.Errorf("stops.txt: %w", err)
		}
	} else {
		return fmt.Errorf("stops.txt is required: %w", err)
	}

	// Parse routes.txt (required)
	if r, err := openTable("routes.txt"); err == nil {
		defer r.Close()
		if err := s.parseRoutes(feed, r); err != nil {
			return fmt.Errorf("routes.txt: %w", err)
		}
	} else {
		return fmt.Errorf("routes.txt is required: %w", err)
	}

	// Parse trips.txt (required)
	if r, err := openTable("trips.txt"); err == nil {
		defer r.Close()
		if err := s.parseTrips(feed, r); err != nil {
			return fmt.Errorf("trips.txt: %w", err)
		}
	} else {
		return fmt.Errorf("trips.txt is required: %w", err)
	}

	// Parse stop_times.txt (required)
	if r, err := openTable("stop_times.txt"); err == nil {
		defer r.Close()
		if err := s.parseStopTimes(feed, r); err != nil {
			return fmt.Errorf("stop_times.txt: %w", err)
		}
	} else {
		return fmt.Errorf("stop_times.txt is required: %w", err)
	}

	// Parse calendar.txt (optional if calendar_dates is supplied)
	if r, err := openTable("calendar.txt"); err == nil {
		defer r.Close()
		if err := s.parseCalendar(feed, r); err != nil {
			return fmt.Errorf("calendar.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Parse calendar_dates.txt (optional)
	if r, err := openTable("calendar_dates.txt"); err == nil {
		defer r.Close()
		if err := s.parseCalendarDates(feed, r); err != nil {
			return fmt.Errorf("calendar_dates.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Parse transfers.txt (optional)
	if r, err := openTable("transfers.txt"); err == nil {
		defer r.Close()
		if err := s.parseTransfers(feed, r); err != nil {
			return fmt.Errorf("transfers.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Parse frequencies.txt (optional)
	if r, err := openTable("frequencies.txt"); err == nil {
		defer r.Close()
		if err := s.parseFrequencies(feed, r); err != nil {
			return fmt.Errorf("frequencies.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s *GTFSZipSource) ns(id string) string {
	if s.DefaultAgency == "" || strings.Contains(id, gtfs.IDSeparator) {
		return id
	}
	return gtfs.Namespace(s.DefaultAgency, id)
}

func readCSVHeaders(r io.Reader) (*csv.Reader, map[string]int, error) {
	// Strip UTF-8 BOM if present
	br := csv.NewReader(r)
	br.FieldsPerRecord = -1
	br.ReuseRecord = true
	br.TrimLeadingSpace = true

	header, err := br.Read()
	if err != nil {
		return nil, nil, err
	}
	if len(header) > 0 {
		header[0] = string(bytes.TrimPrefix([]byte(header[0]), []byte("\xef\xbb\xbf")))
	}
	colIdx := make(map[string]int, len(header))
	for i, h := range header {
		colIdx[strings.TrimSpace(h)] = i
	}
	return br, colIdx, nil
}

func getField(rec []string, colMap map[string]int, col string) string {
	if idx, ok := colMap[col]; ok && idx < len(rec) {
		return strings.TrimSpace(rec[idx])
	}
	return ""
}

func (s *GTFSZipSource) parseAgencies(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		id := getField(rec, colMap, "agency_id")
		if id == "" && s.DefaultAgency != "" {
			id = s.DefaultAgency
		}
		feed.Agencies = append(feed.Agencies, RawAgency{
			ID:       s.ns(id),
			Name:     getField(rec, colMap, "agency_name"),
			Timezone: getField(rec, colMap, "agency_timezone"),
		})
	}
	return nil
}

func (s *GTFSZipSource) parseStops(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		lat, _ := strconv.ParseFloat(getField(rec, colMap, "stop_lat"), 64)
		lon, _ := strconv.ParseFloat(getField(rec, colMap, "stop_lon"), 64)
		stopID := getField(rec, colMap, "stop_id")
		feed.Stops = append(feed.Stops, RawStop{
			ID:   s.ns(stopID),
			Name: getField(rec, colMap, "stop_name"),
			Lat:  lat,
			Lon:  lon,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseRoutes(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		routeType, _ := strconv.Atoi(getField(rec, colMap, "route_type"))
		feed.Routes = append(feed.Routes, RawRoute{
			ID:        s.ns(getField(rec, colMap, "route_id")),
			AgencyID:  s.ns(getField(rec, colMap, "agency_id")),
			ShortName: getField(rec, colMap, "route_short_name"),
			LongName:  getField(rec, colMap, "route_long_name"),
			Type:      routeType,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseTrips(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		dir, _ := strconv.Atoi(getField(rec, colMap, "direction_id"))
		feed.Trips = append(feed.Trips, RawTrip{
			ID:        s.ns(getField(rec, colMap, "trip_id")),
			RouteID:   s.ns(getField(rec, colMap, "route_id")),
			ServiceID: s.ns(getField(rec, colMap, "service_id")),
			Headsign:  getField(rec, colMap, "trip_headsign"),
			Direction: dir,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseStopTimes(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		seq, err := strconv.ParseUint(getField(rec, colMap, "stop_sequence"), 10, 32)
		if err != nil {
			continue
		}
		arr, _ := gtfs.ParseTime(getField(rec, colMap, "arrival_time"))
		dep, _ := gtfs.ParseTime(getField(rec, colMap, "departure_time"))

		feed.StopTimes = append(feed.StopTimes, RawStopTime{
			TripID:       s.ns(getField(rec, colMap, "trip_id")),
			StopID:       s.ns(getField(rec, colMap, "stop_id")),
			StopSequence: uint32(seq),
			ArrTime:      arr,
			DepTime:      dep,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseCalendar(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		startDate, _ := gtfs.ParseDate(getField(rec, colMap, "start_date"))
		endDate, _ := gtfs.ParseDate(getField(rec, colMap, "end_date"))

		feed.Calendars = append(feed.Calendars, RawCalendar{
			ServiceID: s.ns(getField(rec, colMap, "service_id")),
			Monday:    getField(rec, colMap, "monday") == "1",
			Tuesday:   getField(rec, colMap, "tuesday") == "1",
			Wednesday: getField(rec, colMap, "wednesday") == "1",
			Thursday:  getField(rec, colMap, "thursday") == "1",
			Friday:    getField(rec, colMap, "friday") == "1",
			Saturday:  getField(rec, colMap, "saturday") == "1",
			Sunday:    getField(rec, colMap, "sunday") == "1",
			StartDate: startDate,
			EndDate:   endDate,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseCalendarDates(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		d, err := gtfs.ParseDate(getField(rec, colMap, "date"))
		if err != nil {
			continue
		}
		exType, _ := strconv.Atoi(getField(rec, colMap, "exception_type"))
		feed.CalendarDates = append(feed.CalendarDates, RawCalendarDate{
			ServiceID:     s.ns(getField(rec, colMap, "service_id")),
			Date:          d,
			ExceptionType: exType,
		})
	}
	return nil
}

func (s *GTFSZipSource) parseTransfers(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		tt, _ := strconv.Atoi(getField(rec, colMap, "transfer_type"))
		minTime, _ := strconv.ParseUint(getField(rec, colMap, "min_transfer_time"), 10, 16)
		feed.Transfers = append(feed.Transfers, RawTransfer{
			FromStopID:      s.ns(getField(rec, colMap, "from_stop_id")),
			ToStopID:        s.ns(getField(rec, colMap, "to_stop_id")),
			TransferType:    tt,
			MinTransferTime: uint16(minTime),
		})
	}
	return nil
}

func (s *GTFSZipSource) parseFrequencies(feed *RawFeed, r io.Reader) error {
	reader, colMap, err := readCSVHeaders(r)
	if err != nil {
		return err
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		startTime, _ := gtfs.ParseTime(getField(rec, colMap, "start_time"))
		endTime, _ := gtfs.ParseTime(getField(rec, colMap, "end_time"))
		headway, _ := strconv.ParseUint(getField(rec, colMap, "headway_secs"), 10, 32)
		exactTimes, _ := strconv.Atoi(getField(rec, colMap, "exact_times"))

		feed.Frequencies = append(feed.Frequencies, RawFrequency{
			TripID:      s.ns(getField(rec, colMap, "trip_id")),
			StartTime:   startTime,
			EndTime:     endTime,
			HeadwaySecs: uint32(headway),
			ExactTimes:  exactTimes,
		})
	}
	return nil
}

