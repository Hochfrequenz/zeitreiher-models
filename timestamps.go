package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// instantPattern is the only accepted spelling of an instant: RFC 3339 with an upper-case "T", whole
// seconds and an explicit offset. The Python models use the same expression.
var instantPattern = regexp.MustCompile(`^[0-9]{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// minYear is the earliest year an instant may be written with (as four digits); the Python models
// use the same.
const minYear = "1900"

// minUTCYear and maxUTCYear bound the year of the instant itself (in UTC), so that every request
// instant can also be written as a response instant ("Z"). The Python models use the same.
const (
	minUTCYear = 1900
	maxUTCYear = 9999
)

// side says whether a value travels in a request or in a response; some rules differ.
type side uint8

const (
	// requestSide: timestamps accept any explicit offset and keep it, profile numbers may have
	// leading zeros, unknown keys are rejected.
	requestSide side = iota
	// responseSide: timestamps are UTC ("Z"), profile numbers have no leading zeros, unknown keys
	// are ignored.
	responseSide
)

// unknownKeyPolicy returns how objects on this side treat unknown keys.
func (s side) unknownKeyPolicy() unknownKeys {
	if s == responseSide {
		return ignoreUnknown
	}
	return rejectUnknown
}

// parseInstantString checks s against the rules of kind and returns the instant.
func parseInstantString(s string, kind side) (time.Time, error) {
	if !instantPattern.MatchString(s) {
		return time.Time{}, fmt.Errorf("%q is not an RFC 3339 timestamp with whole seconds and an explicit offset", s)
	}
	if s[:4] < minYear {
		return time.Time{}, fmt.Errorf("%q: years before %s are not allowed", s, minYear)
	}
	if strings.HasSuffix(s, "-00:00") {
		return time.Time{}, fmt.Errorf("%q: the offset -00:00 means 'unknown offset' and is not allowed", s)
	}
	if kind == responseSide && !strings.HasSuffix(s, "Z") {
		return time.Time{}, fmt.Errorf("%q: response timestamps must be in UTC (Z)", s)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a valid timestamp: %w", s, err)
	}
	if y := t.UTC().Year(); y < minUTCYear || y > maxUTCYear {
		return time.Time{}, fmt.Errorf("%q: the instant is outside the years %d to %d in UTC", s, minUTCYear, maxUTCYear)
	}
	return t, nil
}

// parseInstant decodes a JSON string holding an instant.
func parseInstant(raw json.RawMessage, kind side) (time.Time, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, errors.New("must be an RFC 3339 timestamp string")
	}
	return parseInstantString(s, kind)
}

// validateInstant checks a time.Time that is about to be written or was built by a caller.
func validateInstant(t time.Time, kind side) error {
	if t.IsZero() {
		return errors.New("is required")
	}
	if t.Nanosecond() != 0 {
		return fmt.Errorf("%s: only whole seconds are allowed", t.Format(time.RFC3339Nano))
	}
	if t.Year() < 1 {
		return fmt.Errorf("%s: year must be at least 1", t.Format(time.RFC3339))
	}
	// The wire form must parse back to the same instant: RFC 3339 drops offset seconds (an LMT-style
	// zone would move the instant) and a five-digit year does not match the grammar.
	wire := formatInstant(t, kind)
	back, err := parseInstantString(wire, kind)
	if err != nil {
		return err
	}
	if !back.Equal(t) {
		return fmt.Errorf("%s: the offset cannot be written without changing the instant", wire)
	}
	return nil
}

// formatInstant renders t in the wire form. Request instants keep their offset (a zero offset is
// written as "Z"); response instants are converted to UTC first.
func formatInstant(t time.Time, kind side) string {
	if kind == responseSide {
		t = t.UTC()
	}
	return t.Format(time.RFC3339)
}

// validateWindow checks a half-open interval [from, to).
func validateWindow(from, to time.Time, kind side) error {
	if err := validateInstant(from, kind); err != nil {
		return fieldError("from", err)
	}
	if err := validateInstant(to, kind); err != nil {
		return fieldError("to", err)
	}
	if !from.Before(to) {
		return fmt.Errorf("from (%s) must be before to (%s)", formatInstant(from, kind), formatInstant(to, kind))
	}
	return nil
}
