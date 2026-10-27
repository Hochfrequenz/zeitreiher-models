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
var instantPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})$`)

// instantKind says which timestamp rules apply.
type instantKind uint8

const (
	// requestInstant accepts any explicit offset and keeps it.
	requestInstant instantKind = iota
	// responseInstant accepts UTC ("Z") only.
	responseInstant
)

// parseInstantString checks s against the rules of kind and returns the instant.
func parseInstantString(s string, kind instantKind) (time.Time, error) {
	if !instantPattern.MatchString(s) {
		return time.Time{}, fmt.Errorf("%q is not an RFC 3339 timestamp with whole seconds and an explicit offset", s)
	}
	if strings.HasSuffix(s, "-00:00") {
		return time.Time{}, fmt.Errorf("%q: the offset -00:00 means 'unknown offset' and is not allowed", s)
	}
	if kind == responseInstant && !strings.HasSuffix(s, "Z") {
		return time.Time{}, fmt.Errorf("%q: response timestamps must be in UTC (Z)", s)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a valid timestamp: %w", s, err)
	}
	return t, nil
}

// parseInstant decodes a JSON string holding an instant.
func parseInstant(raw json.RawMessage, kind instantKind) (time.Time, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, errors.New("must be an RFC 3339 timestamp string")
	}
	return parseInstantString(s, kind)
}

// validateInstant checks a time.Time that is about to be written or was built by a caller.
func validateInstant(t time.Time, kind instantKind) error {
	if t.IsZero() {
		return errors.New("is required")
	}
	if t.Nanosecond() != 0 {
		return fmt.Errorf("%s: only whole seconds are allowed", t.Format(time.RFC3339Nano))
	}
	if kind == responseInstant {
		if _, offset := t.Zone(); offset != 0 {
			return fmt.Errorf("%s: response timestamps must be in UTC (Z)", t.Format(time.RFC3339))
		}
	}
	return nil
}

// formatInstant renders t in the wire form. A zero offset is written as "Z".
func formatInstant(t time.Time) string {
	return t.Format(time.RFC3339)
}

// validateWindow checks a half-open interval [from, to).
func validateWindow(from, to time.Time, kind instantKind) error {
	if err := validateInstant(from, kind); err != nil {
		return fieldError("from", err)
	}
	if err := validateInstant(to, kind); err != nil {
		return fieldError("to", err)
	}
	if !from.Before(to) {
		return fmt.Errorf("from (%s) must be before to (%s)", formatInstant(from), formatInstant(to))
	}
	return nil
}
