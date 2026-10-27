package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	decimalPattern         = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	requestProfilePattern  = regexp.MustCompile(`^[0-9]{1,18}$`)
	responseProfilePattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
	maloPattern            = regexp.MustCompile(`^[0-9]{11}$`)
	customByPattern        = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	intervalPattern        = regexp.MustCompile(`^PT[1-9][0-9]*M$`)
	// unitPattern: an SAP internal unit (T006-MSEHI, CHAR 3), e.g. "KWH", "%" or "°C".
	unitPattern = regexp.MustCompile(`^[0-9A-Za-z%°/._-]{1,3}$`)
)

// Decimal is an already-scaled decimal number in its string form, e.g. "160" or "0.022".
//
// Values are strings on the wire so that no client loses precision by parsing them as floats.
// The accepted syntax is an optional minus, an integer part without leading zeros and an optional
// fractional part: ^-?(0|[1-9][0-9]*)(\.[0-9]+)?$. Exponents, a plus sign, negative zero and JSON numbers
// are rejected.
type Decimal string

// Validate checks the syntax of d.
func (d Decimal) Validate() error {
	if !decimalPattern.MatchString(string(d)) {
		return fmt.Errorf("%q is not a decimal string", string(d))
	}
	if strings.HasPrefix(string(d), "-") && strings.Trim(string(d), "-0.") == "" {
		return fmt.Errorf("%q: negative zero is not allowed", string(d))
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

// validateInterval accepts a sub-daily ISO 8601 duration in minutes, e.g. "PT15M".
func validateInterval(i string) error {
	if !intervalPattern.MatchString(i) {
		return fmt.Errorf("%q is not a sub-daily interval of the form PT<minutes>M", i)
	}
	return nil
}

// validateUnit accepts an SAP unit: 1 to 3 characters out of A-Z, a-z, 0-9 and % ° / . _ -.
func validateUnit(u string) error {
	if err := validateText(u); err != nil {
		return err
	}
	if !unitPattern.MatchString(u) {
		return fmt.Errorf("%q is not a unit (1 to 3 characters out of A-Z, a-z, 0-9, %%°/._-)", u)
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
