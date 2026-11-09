// Package gtfs contains small, dependency-free helpers for GTFS primitives:
// time-of-day values ("25:10:00"), service dates ("20261109"), service-day
// anchoring in an agency timezone, and agency-namespaced IDs (ADR-003).
package gtfs

import (
	"fmt"
	"strings"
)

// IDSeparator separates the agency namespace from the original GTFS ID.
const IDSeparator = ":"

// Namespace returns "{agency}:{id}" (ADR-003). If agency is empty, id is
// returned unchanged (used for sources that are already namespaced, e.g. the
// static GTFS database populated by ingestion). Empty ids stay empty so that
// optional references (e.g. parent_station) remain "unset".
func Namespace(agency, id string) string {
	if agency == "" || id == "" {
		return id
	}
	return agency + IDSeparator + id
}

// SplitID splits a namespaced ID into its agency namespace and original ID.
func SplitID(id string) (agency, local string, err error) {
	agency, local, ok := strings.Cut(id, IDSeparator)
	if !ok || agency == "" || local == "" {
		return "", "", fmt.Errorf("gtfs: %q is not a namespaced id (want agency%sid)", id, IDSeparator)
	}
	return agency, local, nil
}

