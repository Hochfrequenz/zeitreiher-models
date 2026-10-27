package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	decimalPattern         = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	requestProfilePattern  = regexp.MustCompile(`^[0-9]{1,18}$`)
	responseProfilePattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
	maloPattern            = regexp.MustCompile(`^[0-9]{11}$`)
	customByPattern        = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	intervalPattern        = regexp.MustCompile(`^PT[1-9][0-9]*M$`)
)

// Decimal is an already-scaled decimal number in its shortest canonical string form, e.g. "160",
// "0.022", "25.25" or "-3.5".
//
// Values are strings on the wire so that no client loses precision by parsing them as floats.
// The accepted syntax is an optional minus, an integer part without leading zeros and an optional
// fractional part without trailing zeros: ^-?(0|[1-9][0-9]*)(\.[0-9]+)?$, and the fraction must not end
// in 0. Exponents, a plus sign, negative zero, trailing zeros ("160.0", "0.0220") and JSON numbers are
// rejected: after parsing, 160.0 and 160 are the same number, so the wire carries one spelling.
type Decimal string

// Validate checks the syntax of d.
func (d Decimal) Validate() error {
	s := string(d)
	if !decimalPattern.MatchString(s) {
		return fmt.Errorf("%q is not a decimal string", s)
	}
	if strings.HasPrefix(s, "-") && strings.Trim(s, "-0.") == "" {
		return fmt.Errorf("%q: negative zero is not allowed", s)
	}
	if strings.Contains(s, ".") && strings.HasSuffix(s, "0") {
		shortest := strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
		return fmt.Errorf("%q is not in shortest form (no trailing zeros after the decimal point): write %q", s, shortest)
	}
	return nil
}

// MarshalJSON writes d as a JSON string after validating it.
func (d Decimal) MarshalJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(string(d))
}

// UnmarshalJSON accepts a JSON string with a valid decimal only.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("must be a decimal string, not a JSON number")
	}
	if err := Decimal(s).Validate(); err != nil {
		return err
	}
	*d = Decimal(s)
	return nil
}

// validateRequestProfile accepts profile numbers with or without leading zeros (NUMC 18). An all-zero
// number is rejected: there is no profile 0, and its echo ("0") would not be a valid response profile.
func validateRequestProfile(p string) error {
	if !requestProfilePattern.MatchString(p) {
		return fmt.Errorf("%q is not a profile number (1 to 18 digits)", p)
	}
	if strings.Trim(p, "0") == "" {
		return fmt.Errorf("%q: profile number 0 does not exist", p)
	}
	return nil
}

// validateResponseProfile accepts profile numbers as the API emits them: without leading zeros.
func validateResponseProfile(p string) error {
	if !responseProfilePattern.MatchString(p) {
		return fmt.Errorf("%q is not a profile number without leading zeros", p)
	}
	return nil
}

// validateMaLo accepts an 11-digit market location id.
func validateMaLo(m string) error {
	if !maloPattern.MatchString(m) {
		return fmt.Errorf("%q is not a market location id (11 digits)", m)
	}
	return nil
}

// validateInterval accepts a sub-daily ISO 8601 duration in minutes, e.g. "PT15M". Hours are written in
// minutes ("PT60M"); "PT1H" is rejected so that every interval has exactly one spelling.
func validateInterval(i string) error {
	if !intervalPattern.MatchString(i) {
		return fmt.Errorf("%q is not a sub-daily interval of the form PT<minutes>M, e.g. PT15M or PT60M "+
			"(hours are written in minutes: PT60M, not PT1H)", i)
	}
	return nil
}

// Units are passed through as SAP delivers them; which T006 column (internal MSEHI such as "KWH" or an
// external form such as "kWh") the API carries is still open, so the rules are deliberately lenient.
const (
	// maxRequestUnitLength bounds a requested unit, in characters (code points).
	maxRequestUnitLength = 6
	// maxResponseUnitLength bounds the unit of a series record, in characters (code points).
	maxResponseUnitLength = 30
	// unitSymbols are the non-alphanumeric characters a requested unit may contain.
	unitSymbols = "%°/._-"
)

// validateRequestUnit accepts 1 to 6 characters, each a Unicode letter (L*), a Unicode number (N*)
// or one of % ° / . _ - — so no whitespace and no control characters. The Python models use the same
// Unicode categories.
func validateRequestUnit(u string) error {
	if err := validateText(u); err != nil {
		return err
	}
	n := utf8.RuneCountInString(u)
	ok := n >= 1 && n <= maxRequestUnitLength
	for _, r := range u {
		ok = ok && (unicode.IsLetter(r) || unicode.IsNumber(r) || strings.ContainsRune(unitSymbols, r))
	}
	if !ok {
		return fmt.Errorf("%q is not a unit (1 to %d letters, digits or %s)", u, maxRequestUnitLength, unitSymbols)
	}
	return nil
}

// validateResponseUnit accepts any non-empty unit of at most 30 characters without control characters
// (Unicode Cc); responses are read leniently so that a unit SAP delivers does not break clients.
func validateResponseUnit(u string) error {
	if err := validateText(u); err != nil {
		return err
	}
	n := utf8.RuneCountInString(u)
	if n < 1 || n > maxResponseUnitLength || strings.ContainsFunc(u, unicode.IsControl) {
		return fmt.Errorf("%q is not a unit (1 to %d characters, no control characters)", u, maxResponseUnitLength)
	}
	return nil
}

// validateText rejects strings that are not valid UTF-8. encoding/json would write them with U+FFFD
// replacement characters, silently changing the text; the Python models reject lone surrogates.
func validateText(s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%q is not valid UTF-8", s)
	}
	return nil
}

// validateTextMap checks the keys and values of a string map (selector params, series attributes)
// with validateText, in key order so that the reported error does not depend on map iteration.
func validateTextMap(m map[string]string) error {
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if err := validateText(k); err != nil {
			return fmt.Errorf("key: %w", err)
		}
		if err := validateText(m[k]); err != nil {
			return fieldError(k, err)
		}
	}
	return nil
}

// validateToken accepts a non-empty string of valid UTF-8 (cursors).
func validateToken(s string) error {
	if s == "" {
		return errors.New("must not be empty")
	}
	return validateText(s)
}

// validateStatus accepts non-empty status codes in strictly ascending order (sorted, no duplicates).
func validateStatus(status []string) error {
	for i, s := range status {
		if s == "" {
			return errors.New("status codes must not be empty")
		}
		if err := validateText(s); err != nil {
			return err
		}
		if i > 0 && status[i-1] >= s {
			return fmt.Errorf("status codes must be sorted and unique, got %q before %q", status[i-1], s)
		}
	}
	return nil
}

// validateMin checks an integer lower bound.
func validateMin(name string, v, minimum int) error {
	if v < minimum {
		return fmt.Errorf("%s: must be at least %d, got %d", name, minimum, v)
	}
	return nil
}
