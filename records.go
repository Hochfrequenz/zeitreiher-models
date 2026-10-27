package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Record types, the value of the "type" key of every NDJSON line.
const (
	RecordTypeSeries = "series"
	RecordTypeValue  = "value"
	RecordTypeError  = "error"
	RecordTypeEnd    = "end"
)

// Record is one line of a query response (NDJSON) and one gRPC Record message. It is one of
// *SeriesRecord, *ValueRecord, *ErrorRecord or *EndRecord.
type Record interface {
	// RecordType returns the value of the "type" key.
	RecordType() string
	// Validate checks the record.
	Validate() error
	isRecord()
}

// Version describes one EDM profile version that overlaps a series (spec §4.2, §5).
type Version struct {
	// Number is the SAP version number (CVERSNO), at least 1.
	Number int
	// From and To bound the covered range [From, To); To is nil for an open end.
	From time.Time
	To   *time.Time
	// CreatedAt is the creation instant; nil when it cannot be determined, then CreatedAtUnknown is true.
	CreatedAt        *time.Time
	CreatedAtUnknown bool
	// Reason and Description are SAP's VERSREASON and VERSDESCR; either may be empty.
	Reason      string
	Description string
	// Deleted is true for versions flagged as deleted in SAP. They are still listed; querying their
	// values gives an error record with code version_deleted.
	Deleted bool
}

// Validate checks the version.
func (v Version) Validate() error {
	if err := validateMin("number", v.Number, 1); err != nil {
		return err
	}
	if v.To != nil {
		if err := validateWindow(v.From, *v.To, responseSide); err != nil {
			return err
		}
	} else if err := validateInstant(v.From, responseSide); err != nil {
		return fieldError("from", err)
	}
	if v.CreatedAt != nil {
		if err := validateInstant(*v.CreatedAt, responseSide); err != nil {
			return fieldError("created_at", err)
		}
	}
	if v.CreatedAtUnknown != (v.CreatedAt == nil) {
		return errors.New("created_at_unknown must be true exactly when created_at is null")
	}
	if err := validateText(v.Reason); err != nil {
		return fieldError("reason", err)
	}
	return wrapField("description", validateText(v.Description))
}

type versionWire struct {
	Number           int     `json:"number"`
	From             string  `json:"from"`
	To               *string `json:"to"`
	CreatedAt        *string `json:"created_at"`
	CreatedAtUnknown bool    `json:"created_at_unknown"`
	Reason           string  `json:"reason"`
	Description      string  `json:"description"`
	Deleted          bool    `json:"deleted"`
}

// MarshalJSON validates and writes the version; To and CreatedAt are written as null when nil.
func (v Version) MarshalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("version %d: %w", v.Number, err)
	}
	return json.Marshal(versionWire{
		Number: v.Number, From: formatInstant(v.From, responseSide), To: instantPtr(v.To, responseSide),
		CreatedAt: instantPtr(v.CreatedAt, responseSide), CreatedAtUnknown: v.CreatedAtUnknown,
		Reason: v.Reason, Description: v.Description, Deleted: v.Deleted,
	})
}

// UnmarshalJSON parses and validates the version.
func (v *Version) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"number": required, "from": required, "to": requiredNullable, "created_at": requiredNullable,
		"created_at_unknown": required, "reason": required, "description": required, "deleted": required,
	}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("version: %w", err)
	}
	var out Version
	if out.From, err = decodeRequiredInstant(m, "from", responseSide); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if out.To, err = decodeInstant(m, "to", responseSide); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	if out.CreatedAt, err = decodeInstant(m, "created_at", responseSide); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	for key, dst := range map[string]any{
		"number": &out.Number, "created_at_unknown": &out.CreatedAtUnknown, "reason": &out.Reason,
		"description": &out.Description, "deleted": &out.Deleted,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("version: %w", err)
		}
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	*v = out
	return nil
}

// SeriesRecord opens a series: one profile assignment of one selector, cut to the request window.
type SeriesRecord struct {
	// Series numbers the series of a response, starting at 1 and continuing across pages.
	Series int
	// Selector is the index of the request selector this series belongs to.
	Selector int
	// Select echoes the request selector (profile numbers without leading zeros).
	Select Selector
	// Profile is the SAP profile number, without leading zeros.
	Profile string
	// Role is the role of the profile assignment (SAP EPROFASS role, e.g. "0001"), or nil when the series
	// was not resolved through a role (e.g. by: profile). The Resolver BAdI may override it.
	Role *string
	// From and To bound the assignment, cut to the request window.
	From, To time.Time
	// Unit is the canonical unit code (UN/ECE Rec. 20, e.g. "KWH") of the delivered values (after any
	// conversion), or nil when the SAP unit has no mapping. The series is delivered either way.
	Unit *string
	// UnitRaw is the SAP unit key (T006 MSEHI) as SAP delivers it, mapped or not: any non-empty text of
	// at most 30 characters without control characters.
	UnitRaw string
	// Interval is that of the delivered values (after any conversion).
	Interval string
	// Attributes come from the customer's Metadata BAdI; never nil after unmarshalling.
	Attributes map[string]string
	// Versions lists the profile versions overlapping the series; never nil after unmarshalling.
	Versions []Version
}

// RecordType returns "series".
func (*SeriesRecord) RecordType() string { return RecordTypeSeries }
func (*SeriesRecord) isRecord()          {}

// Validate checks the record.
func (r *SeriesRecord) Validate() error {
	if err := validateMin("series", r.Series, 1); err != nil {
		return err
	}
	if err := validateMin("selector", r.Selector, 0); err != nil {
		return err
	}
	if err := Selector(r.Select).validate(responseSide); err != nil {
		return fieldError("select", err)
	}
	if err := validateResponseProfile(r.Profile); err != nil {
		return fieldError("profile", err)
	}
	if r.Role != nil {
		if err := validateToken(*r.Role); err != nil {
			return fieldError("role", err)
		}
	}
	if err := validateWindow(r.From, r.To, responseSide); err != nil {
		return err
	}
	if r.Unit != nil {
		if err := validateUnitCode(*r.Unit); err != nil {
			return fieldError("unit", err)
		}
	}
	if err := validateRawUnit(r.UnitRaw); err != nil {
		return fieldError("unit_raw", err)
	}
	if err := validateInterval(r.Interval); err != nil {
		return fieldError("interval", err)
	}
	if err := validateTextMap(r.Attributes); err != nil {
		return fieldError("attributes", err)
	}
	for i, v := range r.Versions {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("versions[%d]: %w", i, err)
		}
	}
	return nil
}

// MarshalJSON validates and writes the record with "type" first; a nil Role or Unit is written as null.
func (r *SeriesRecord) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("series record: %w", err)
	}
	attributes := r.Attributes
	if attributes == nil {
		attributes = map[string]string{}
	}
	versions := r.Versions
	if versions == nil {
		versions = []Version{}
	}
	return json.Marshal(struct {
		Type       string            `json:"type"`
		Series     int               `json:"series"`
		Selector   int               `json:"selector"`
		Select     echoedSelector    `json:"select"`
		Profile    string            `json:"profile"`
		Role       *string           `json:"role"`
		From       string            `json:"from"`
		To         string            `json:"to"`
		Unit       *string           `json:"unit"`
		UnitRaw    string            `json:"unit_raw"`
		Interval   string            `json:"interval"`
		Attributes map[string]string `json:"attributes"`
		Versions   []Version         `json:"versions"`
	}{
		RecordTypeSeries, r.Series, r.Selector, echoedSelector(r.Select), r.Profile, r.Role,
		formatInstant(r.From, responseSide), formatInstant(r.To, responseSide), r.Unit, r.UnitRaw, r.Interval,
		attributes, versions,
	})
}

// UnmarshalJSON parses and validates the record.
func (r *SeriesRecord) UnmarshalJSON(b []byte) error {
	m, err := checkRecord(b, RecordTypeSeries, rules{
		"series": required, "selector": required, "select": required, "profile": required,
		"role": requiredNullable, "from": required, "to": required, "unit": requiredNullable, "unit_raw": required, "interval": required,
		"attributes": required, "versions": required,
	})
	if err != nil {
		return err
	}
	var out SeriesRecord
	var sel echoedSelector
	if !isNull(m["role"]) {
		out.Role = new(string)
	}
	if !isNull(m["unit"]) {
		out.Unit = new(string)
	}
	for key, dst := range map[string]any{
		"series": &out.Series, "selector": &out.Selector, "select": &sel, "profile": &out.Profile,
		"role": out.Role, "unit": out.Unit, "unit_raw": &out.UnitRaw, "interval": &out.Interval, "attributes": &out.Attributes,
		"versions": &out.Versions,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("series record: %w", err)
		}
	}
	out.Select = Selector(sel)
	if out.From, err = decodeRequiredInstant(m, "from", responseSide); err != nil {
		return fmt.Errorf("series record: %w", err)
	}
	if out.To, err = decodeRequiredInstant(m, "to", responseSide); err != nil {
		return fmt.Errorf("series record: %w", err)
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("series record: %w", err)
	}
	*r = out
	return nil
}

// ValueRecord is one value of a series for the half-open interval [From, To).
type ValueRecord struct {
	// Series is the number of the series record this value belongs to.
	Series int
	// From and To bound the value's interval.
	From, To time.Time
	// Value is the already-scaled decimal value.
	Value Decimal
	// Status holds the active SAP status codes overlapping the interval, sorted; never nil after
	// unmarshalling.
	Status []string
	// Version is the source of the value: the number of the stored version row it came from, or nil
	// when it is a current value. With {"number":N} a series mixes both: current values overlaid with
	// the stored values of versions >= N (the lowest wins), only inside N's covered range.
	Version *int
}

// RecordType returns "value".
func (*ValueRecord) RecordType() string { return RecordTypeValue }
func (*ValueRecord) isRecord()          {}

// Validate checks the record.
func (r *ValueRecord) Validate() error {
	if err := validateMin("series", r.Series, 1); err != nil {
		return err
	}
	if err := validateWindow(r.From, r.To, responseSide); err != nil {
		return err
	}
	if err := r.Value.Validate(); err != nil {
		return fieldError("value", err)
	}
	if err := validateStatus(r.Status); err != nil {
		return fieldError("status", err)
	}
	if r.Version != nil {
		return validateMin("version", *r.Version, 1)
	}
	return nil
}

// MarshalJSON validates and writes the record; a nil Version is written as null.
func (r *ValueRecord) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("value record: %w", err)
	}
	status := r.Status
	if status == nil {
		status = []string{}
	}
	return json.Marshal(struct {
		Type    string   `json:"type"`
		Series  int      `json:"series"`
		From    string   `json:"from"`
		To      string   `json:"to"`
		Value   Decimal  `json:"value"`
		Status  []string `json:"status"`
		Version *int     `json:"version"`
	}{
		RecordTypeValue, r.Series, formatInstant(r.From, responseSide), formatInstant(r.To, responseSide),
		r.Value, status, r.Version,
	})
}

// UnmarshalJSON parses and validates the record.
func (r *ValueRecord) UnmarshalJSON(b []byte) error {
	m, err := checkRecord(b, RecordTypeValue, rules{
		"series": required, "from": required, "to": required, "value": required, "status": required,
		"version": requiredNullable,
	})
	if err != nil {
		return err
	}
	var out ValueRecord
	if !isNull(m["version"]) {
		out.Version = new(int)
	}
	for key, dst := range map[string]any{
		"series": &out.Series, "value": &out.Value, "status": &out.Status, "version": out.Version,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("value record: %w", err)
		}
	}
	if out.From, err = decodeRequiredInstant(m, "from", responseSide); err != nil {
		return fmt.Errorf("value record: %w", err)
	}
	if out.To, err = decodeRequiredInstant(m, "to", responseSide); err != nil {
		return fmt.Errorf("value record: %w", err)
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("value record: %w", err)
	}
	*r = out
	return nil
}

// ErrorRecord reports a per-selector or per-assignment problem; the stream continues.
type ErrorRecord struct {
	// Selector is the index of the request selector the problem belongs to.
	Selector int
	// Select echoes the request selector.
	Select Selector
	// Profile is set where the problem concerns one profile assignment; empty otherwise.
	Profile string
	// Code says what went wrong; see the RecordError* constants.
	Code RecordErrorCode
}

// RecordType returns "error".
func (*ErrorRecord) RecordType() string { return RecordTypeError }
func (*ErrorRecord) isRecord()          {}

// Validate checks the record.
func (r *ErrorRecord) Validate() error {
	if err := validateMin("selector", r.Selector, 0); err != nil {
		return err
	}
	if err := Selector(r.Select).validate(responseSide); err != nil {
		return fieldError("select", err)
	}
	if r.Profile != "" {
		if err := validateResponseProfile(r.Profile); err != nil {
			return fieldError("profile", err)
		}
	}
	return wrapField("code", r.Code.Validate())
}

// MarshalJSON validates and writes the record; an empty Profile is omitted.
func (r *ErrorRecord) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("error record: %w", err)
	}
	return json.Marshal(struct {
		Type     string          `json:"type"`
		Selector int             `json:"selector"`
		Select   echoedSelector  `json:"select"`
		Profile  string          `json:"profile,omitempty"`
		Code     RecordErrorCode `json:"code"`
	}{RecordTypeError, r.Selector, echoedSelector(r.Select), r.Profile, r.Code})
}

// UnmarshalJSON parses and validates the record.
func (r *ErrorRecord) UnmarshalJSON(b []byte) error {
	m, err := checkRecord(b, RecordTypeError, rules{
		"selector": required, "select": required, "profile": optional, "code": required,
	})
	if err != nil {
		return err
	}
	var out ErrorRecord
	var sel echoedSelector
	for key, dst := range map[string]any{
		"selector": &out.Selector, "select": &sel, "profile": &out.Profile, "code": &out.Code,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("error record: %w", err)
		}
	}
	out.Select = Selector(sel)
	if _, ok := m["profile"]; ok && out.Profile == "" {
		return errors.New("error record: profile: must not be empty")
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("error record: %w", err)
	}
	*r = out
	return nil
}

// EndRecord closes a successful response. A response without an end record is incomplete and
// must be treated as a failure.
type EndRecord struct {
	// Series, Values and Errors count the series, value and error records on this page only (not
	// the whole result). A series record re-sent at the start of a continuation page counts again.
	Series, Values, Errors int
	// Next is the cursor for the next page, or nil when the result is complete.
	Next *string
}

// RecordType returns "end".
func (*EndRecord) RecordType() string { return RecordTypeEnd }
func (*EndRecord) isRecord()          {}

// Validate checks the record.
func (r *EndRecord) Validate() error {
	for name, v := range map[string]int{"series": r.Series, "values": r.Values, "errors": r.Errors} {
		if err := validateMin(name, v, 0); err != nil {
			return err
		}
	}
	if r.Next != nil {
		return wrapField("next", validateToken(*r.Next))
	}
	return nil
}

// MarshalJSON validates and writes the record; a nil Next is written as null.
func (r *EndRecord) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("end record: %w", err)
	}
	return json.Marshal(struct {
		Type   string  `json:"type"`
		Series int     `json:"series"`
		Values int     `json:"values"`
		Errors int     `json:"errors"`
		Next   *string `json:"next"`
	}{RecordTypeEnd, r.Series, r.Values, r.Errors, r.Next})
}

// UnmarshalJSON parses and validates the record.
func (r *EndRecord) UnmarshalJSON(b []byte) error {
	m, err := checkRecord(b, RecordTypeEnd, rules{
		"series": required, "values": required, "errors": required, "next": requiredNullable,
	})
	if err != nil {
		return err
	}
	var out EndRecord
	if !isNull(m["next"]) {
		out.Next = new(string)
	}
	for key, dst := range map[string]any{
		"series": &out.Series, "values": &out.Values, "errors": &out.Errors, "next": out.Next,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("end record: %w", err)
		}
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("end record: %w", err)
	}
	*r = out
	return nil
}

// checkRecord enforces the presence rules of a record plus its "type".
func checkRecord(b []byte, recordType string, r rules) (map[string]json.RawMessage, error) {
	r["type"] = required
	m, err := checkObject(b, r, ignoreUnknown)
	if err != nil {
		return nil, fmt.Errorf("%s record: %w", recordType, err)
	}
	var t string
	if err := json.Unmarshal(m["type"], &t); err != nil || t != recordType {
		return nil, fmt.Errorf("%s record: type must be %q", recordType, recordType)
	}
	return m, nil
}
