package zeitreihermodels

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

// presence says whether a JSON key must exist and whether it may be null.
type presence uint8

const (
	// required keys must exist and must not be null.
	required presence = iota
	// requiredNullable keys must exist and may be null.
	requiredNullable
	// optional keys may be missing; an explicit null counts as missing.
	optional
)

// rules maps every known key of a JSON object to its presence.
type rules map[string]presence

// unknownKeys says what to do with keys that have no rule.
type unknownKeys bool

const (
	// rejectUnknown is used for requests: a typo must fail loudly.
	rejectUnknown unknownKeys = false
	// ignoreUnknown is used for responses: additive server changes must not break clients.
	ignoreUnknown unknownKeys = true
)

var jsonNull = []byte("null")

// isNull reports whether raw is the JSON literal null.
func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), jsonNull)
}

// checkObject decodes b as a JSON object and enforces r. The returned map holds the known keys only;
// optional keys that are null are removed, required-nullable keys keep their null.
func checkObject(b []byte, r rules, unknown unknownKeys) (map[string]json.RawMessage, error) {
	if isNull(b) {
		return nil, errors.New("must be a JSON object, not null")
	}
	if err := checkText(b); err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errors.New("must be a JSON object")
	}
	if unknown == rejectUnknown {
		var extra []string
		for k := range m {
			if _, ok := r[k]; !ok {
				extra = append(extra, k)
			}
		}
		if len(extra) > 0 {
			sort.Strings(extra)
			return nil, fmt.Errorf("unknown key(s) %q", extra)
		}
	}
	out := make(map[string]json.RawMessage, len(r))
	for key, p := range r {
		raw, ok := m[key]
		switch {
		case !ok && p != optional:
			return nil, fieldError(key, errors.New("is required"))
		case !ok:
			continue
		case isNull(raw) && p == required:
			return nil, fieldError(key, errors.New("must not be null"))
		case isNull(raw) && p == optional:
			continue
		}
		out[key] = raw
	}
	return out, nil
}

// wrapField is fieldError that passes nil through.
func wrapField(key string, err error) error {
	if err == nil {
		return nil
	}
	return fieldError(key, err)
}

// fieldError prefixes err with the JSON key it belongs to.
func fieldError(key string, err error) error {
	return fmt.Errorf("%s: %w", key, err)
}

// decodeField unmarshals the value of key into v, if present, and prefixes errors with the key.
func decodeField(m map[string]json.RawMessage, key string, v any) error {
	raw, ok := m[key]
	if !ok || isNull(raw) {
		return nil
	}
	if sm, isStringMap := v.(*map[string]string); isStringMap {
		return wrapField(key, decodeStringMap(raw, sm))
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fieldError(key, err)
	}
	return nil
}

// decodeStringMap decodes a JSON object of strings. encoding/json would turn a null member into "";
// it is rejected instead, as the Python models do.
func decodeStringMap(raw json.RawMessage, dst *map[string]string) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	out := make(map[string]string, len(members))
	for k, member := range members {
		if isNull(member) {
			return fieldError(k, errors.New("must be a string, not null"))
		}
		var s string
		if err := json.Unmarshal(member, &s); err != nil {
			return fieldError(k, errors.New("must be a string"))
		}
		out[k] = s
	}
	*dst = out
	return nil
}

// decodeInstant parses the value of key as an instant. A missing or null key gives nil.
func decodeInstant(m map[string]json.RawMessage, key string, kind side) (*time.Time, error) {
	raw, ok := m[key]
	if !ok || isNull(raw) {
		return nil, nil //nolint:nilnil // absence is not an error; presence rules are checked by checkObject
	}
	t, err := parseInstant(raw, kind)
	if err != nil {
		return nil, fieldError(key, err)
	}
	return &t, nil
}

// decodeRequiredInstant parses the value of key, which checkObject has already proven present.
func decodeRequiredInstant(m map[string]json.RawMessage, key string, kind side) (time.Time, error) {
	t, err := parseInstant(m[key], kind)
	if err != nil {
		return time.Time{}, fieldError(key, err)
	}
	return t, nil
}

// instantPtr renders an optional instant for the wire; nil stays nil (null or omitted).
func instantPtr(t *time.Time, kind side) *string {
	if t == nil {
		return nil
	}
	s := formatInstant(*t, kind)
	return &s
}

// backslash starts a JSON escape sequence.
const backslash = 0x5C

var errUnpairedSurrogate = errors.New(`string contains an unpaired UTF-16 surrogate escape (\uD800-\uDFFF)`)

// checkText rejects input that encoding/json would silently repair to U+FFFD: invalid UTF-8 and
// \u escapes of unpaired UTF-16 surrogates. The Python models reject both.
func checkText(b []byte) error {
	if !utf8.Valid(b) {
		return errors.New("input is not valid UTF-8")
	}
	for i := 0; i < len(b); i++ {
		if b[i] != backslash {
			continue
		}
		if i+6 > len(b) || b[i+1] != 'u' {
			i++ // skip the escaped character
			continue
		}
		switch r := hex4(b[i+2 : i+6]); {
		case r >= 0xD800 && r <= 0xDBFF:
			if i+12 > len(b) || b[i+6] != backslash || b[i+7] != 'u' {
				return errUnpairedSurrogate
			}
			if lo := hex4(b[i+8 : i+12]); lo < 0xDC00 || lo > 0xDFFF {
				return errUnpairedSurrogate
			}
			i += 11
		case r >= 0xDC00 && r <= 0xDFFF:
			return errUnpairedSurrogate
		default:
			i += 5
		}
	}
	return nil
}

// hex4 decodes four hex digits, or returns -1; malformed escapes are left to encoding/json.
func hex4(h []byte) int {
	v, err := strconv.ParseUint(string(h), 16, 32)
	if err != nil {
		return -1
	}
	return int(v)
}
