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
var instantPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$`)

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
	if strings.HasPrefix(s, "0000") {
		return time.Time{}, fmt.Errorf("%q: year 0000 is not allowed", s)
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
