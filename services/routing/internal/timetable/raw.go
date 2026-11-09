package timetable

import "github.com/city-connector-backend/routing/internal/gtfs"

// RawFeed holds intermediate unindexed GTFS records before patternization.
type RawFeed struct {
	Version      string
	Agencies     []RawAgency
	Stops        []RawStop
	Routes       []RawRoute
	Trips        []RawTrip
	StopTimes    []RawStopTime
	Calendars    []RawCalendar
	CalendarDates []RawCalendarDate
	Transfers    []RawTransfer
	Frequencies  []RawFrequency
}

type RawAgency struct {
	ID       string
	Name     string
	Timezone string
}

type RawStop struct {
	ID   string
	Name string
	Lat  float64
	Lon  float64
}

type RawRoute struct {
	ID        string
	AgencyID  string
	ShortName string
	LongName  string
	Type      int
}

type RawTrip struct {
	ID        string
	RouteID   string
	ServiceID string
	Headsign  string
	Direction int
}

type RawStopTime struct {
	TripID       string
	StopID       string
	StopSequence uint32
	ArrTime      gtfs.Seconds
	DepTime      gtfs.Seconds
}

type RawCalendar struct {
	ServiceID string
	Monday    bool
	Tuesday   bool
	Wednesday bool
	Thursday  bool
	Friday    bool
	Saturday  bool
	Sunday    bool
	StartDate gtfs.Date
	EndDate   gtfs.Date
}

type RawCalendarDate struct {
	ServiceID     string
	Date          gtfs.Date
	ExceptionType int // 1 = added, 2 = removed
}

type RawTransfer struct {
	FromStopID      string
	ToStopID        string
	TransferType    int // 0: recommended, 1: timed, 2: min time, 3: not possible
	MinTransferTime uint16
}

type RawFrequency struct {
	TripID      string
	StartTime   gtfs.Seconds
	EndTime     gtfs.Seconds
	HeadwaySecs uint32
	ExactTimes  int // 0 = frequency based, 1 = schedule based
}
