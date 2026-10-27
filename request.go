package zeitreihermodels

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// VersionSelector chooses which values a query returns (spec §5). Exactly one form is set:
//
//	{"current":{}}                          – the current values (the default when omitted)
//	{"number":3}                            – the values of version 3, only within its covered range
//	{"as_of":"2026-02-01T00:00:00+01:00"}   – the values as they were known at that instant
//
// Build it with CurrentVersion, VersionNumber or VersionAsOf.
type VersionSelector struct {
	Current bool
	Number  *int
	AsOf    *time.Time
}

// CurrentVersion selects the current values.
func CurrentVersion() VersionSelector { return VersionSelector{Current: true} }

// VersionNumber selects the values of version n.
func VersionNumber(n int) VersionSelector { return VersionSelector{Number: &n} }

// VersionAsOf selects the values as they were known at t.
func VersionAsOf(t time.Time) VersionSelector { return VersionSelector{AsOf: &t} }

// Validate checks that exactly one form is set and that it is valid.
func (v VersionSelector) Validate() error {
	set := 0
	if v.Current {
		set++
	}
	if v.Number != nil {
		set++
		if err := validateMin("number", *v.Number, 1); err != nil {
			return err
		}
	}
	if v.AsOf != nil {
		set++
		if err := validateInstant(*v.AsOf, requestSide); err != nil {
			return fieldError("as_of", err)
		}
	}
	if set != 1 {
		return errors.New("exactly one of current, number or as_of must be set")
	}
	return nil
}

// MarshalJSON writes the single set form.
func (v VersionSelector) MarshalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("version: %w", err)
	}
	switch {
	case v.Current:
		return []byte(`{"current":{}}`), nil
	case v.Number != nil:
		return json.Marshal(struct {
			Number int `json:"number"`
		}{*v.Number})
	default:
		return json.Marshal(struct {
			AsOf string `json:"as_of"`
		}{formatInstant(*v.AsOf, requestSide)})
	}
}

// UnmarshalJSON accepts exactly one of the three forms.
func (v *VersionSelector) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"current": optional, "number": optional, "as_of": optional}, rejectUnknown)
	if err != nil {
		return fmt.Errorf("version: %w", err)
	}
	// checkObject drops null optional keys; here a null member is an error, as in Python, so
	// {"current":{},"number":null} is not read as {"current":{}}.
	var members map[string]json.RawMessage
	if err := json.Unmarshal(b, &members); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	for _, key := range []string{"current", "number", "as_of"} {
		if raw, ok := members[key]; ok && isNull(raw) {
			return fmt.Errorf("version: %s must not be null", key)
		}
	}
	if len(m) != 1 {
		return errors.New("version: exactly one of current, number or as_of must be set")
	}
	var out VersionSelector
	if raw, ok := m["current"]; ok {
		if !bytes.Equal(bytes.Join(bytes.Fields(raw), nil), []byte("{}")) {
			return errors.New("version: current must be an empty object {}")
		}
		out.Current = true
	}
	if _, ok := m["number"]; ok {
		out.Number = new(int)
		if err := decodeField(m, "number", out.Number); err != nil {
			return fmt.Errorf("version: %w", err)
		}
	}
	if out.AsOf, err = decodeInstant(m, "as_of", requestSide); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	*v = out
	return nil
}

// QueryRequest is the body of POST /api/v1/series/query (and QUERY /api/v1/series), spec §4.1.
type QueryRequest struct {
	// Select lists one or more selectors; results refer to them by index.
	Select []Selector
	// From and To bound the half-open window [From, To). Their offsets are kept on the wire.
	From, To time.Time
	// Version is optional; nil means the current values.
	Version *VersionSelector
	// Interval is optional, e.g. "PT15M"; empty means the profile's own interval.
	Interval string
	// Unit is optional, an SAP unit such as "KWH"; empty means the profile's own unit.
	Unit string
	// Cursor continues a previous response (its end record's next); empty starts at the beginning.
	Cursor string
}

// Validate checks the request.
func (q QueryRequest) Validate() error {
	if err := validateSelectors(q.Select); err != nil {
		return err
	}
	if err := validateWindow(q.From, q.To, requestSide); err != nil {
		return err
	}
	if q.Version != nil {
		if err := q.Version.Validate(); err != nil {
			return fieldError("version", err)
		}
	}
	if q.Interval != "" {
		if err := validateInterval(q.Interval); err != nil {
			return fieldError("interval", err)
		}
	}
	if q.Unit != "" {
		if err := validateUnit(q.Unit); err != nil {
			return fieldError("unit", err)
		}
	}
	if q.Cursor != "" {
		return wrapField("cursor", validateToken(q.Cursor))
	}
	return nil
}

type queryRequestWire struct {
	Select   []Selector       `json:"select"`
	From     string           `json:"from"`
	To       string           `json:"to"`
	Version  *VersionSelector `json:"version,omitempty"`
	Interval string           `json:"interval,omitempty"`
	Unit     string           `json:"unit,omitempty"`
	Cursor   string           `json:"cursor,omitempty"`
}

// MarshalJSON validates and writes the request; unset optional fields are omitted.
func (q QueryRequest) MarshalJSON() ([]byte, error) {
	if err := q.Validate(); err != nil {
		return nil, fmt.Errorf("query request: %w", err)
	}
	return json.Marshal(queryRequestWire{
		Select: q.Select, From: formatInstant(q.From, requestSide), To: formatInstant(q.To, requestSide),
		Version: q.Version, Interval: q.Interval, Unit: q.Unit, Cursor: q.Cursor,
	})
}

// UnmarshalJSON parses and validates the request. Unknown keys are rejected; null optional fields
// count as absent.
func (q *QueryRequest) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"select": required, "from": required, "to": required,
		"version": optional, "interval": optional, "unit": optional, "cursor": optional,
	}, rejectUnknown)
	if err != nil {
		return fmt.Errorf("query request: %w", err)
	}
	var out QueryRequest
	if err := decodeWindowRequest(m, &out.Select, &out.From, &out.To); err != nil {
		return fmt.Errorf("query request: %w", err)
	}
	if _, ok := m["version"]; ok {
		out.Version = new(VersionSelector)
	}
	for key, dst := range map[string]any{
		"version": out.Version, "interval": &out.Interval, "unit": &out.Unit, "cursor": &out.Cursor,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("query request: %w", err)
		}
	}
	for key, present := range map[string]bool{"interval": out.Interval != "", "unit": out.Unit != "", "cursor": out.Cursor != ""} {
		if _, ok := m[key]; ok && !present {
			return fmt.Errorf("query request: %s: must not be empty", key)
		}
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("query request: %w", err)
	}
	*q = out
	return nil
}

// decodeWindowRequest decodes select, from and to, which checkObject has proven present.
func decodeWindowRequest(m map[string]json.RawMessage, sel *[]Selector, from, to *time.Time) error {
	if err := decodeField(m, "select", sel); err != nil {
		return err
	}
	var err error
	if *from, err = decodeRequiredInstant(m, "from", requestSide); err != nil {
		return err
	}
	if *to, err = decodeRequiredInstant(m, "to", requestSide); err != nil {
		return err
	}
	return nil
}

// WindowRequest is the body of the resolve and versions endpoints (spec §4.4): selectors and a
// window, nothing else.
type WindowRequest struct {
	Select   []Selector
	From, To time.Time
}

// ResolveRequest is the body of POST /api/v1/series/resolve.
type ResolveRequest = WindowRequest

// VersionsRequest is the body of POST /api/v1/series/versions.
type VersionsRequest = WindowRequest

// Validate checks the request.
func (w WindowRequest) Validate() error {
	if err := validateSelectors(w.Select); err != nil {
		return err
	}
	return validateWindow(w.From, w.To, requestSide)
}

// MarshalJSON validates and writes the request.
func (w WindowRequest) MarshalJSON() ([]byte, error) {
	if err := w.Validate(); err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	return json.Marshal(struct {
		Select []Selector `json:"select"`
		From   string     `json:"from"`
		To     string     `json:"to"`
	}{w.Select, formatInstant(w.From, requestSide), formatInstant(w.To, requestSide)})
}

// UnmarshalJSON parses and validates the request; unknown keys (e.g. version, cursor) are rejected.
func (w *WindowRequest) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"select": required, "from": required, "to": required}, rejectUnknown)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	var out WindowRequest
	if err := decodeWindowRequest(m, &out.Select, &out.From, &out.To); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("request: %w", err)
	}
	*w = out
	return nil
}
