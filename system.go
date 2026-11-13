package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// fingerprintPattern is the spelling of the aggregate source fingerprint: a SHA-256 as 64 lower-case hex
// characters. The Python models use the same expression.
var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// InactiveObjects reports the inactive ABAP objects of the package, or why that is unknown.
// Available true: Count and Names are set (Count equals len(Names)) and Reason is empty.
// Available false: Reason is set and Count and Names are not.
type InactiveObjects struct {
	// Available says whether the ABAP backend could determine the inactive objects.
	Available bool
	// Count is the number of inactive objects; set exactly when Available (equals len(Names)).
	Count *int64
	// Names are the inactive object names; nil or empty is written as [] when Available, and must be nil
	// otherwise.
	Names []string
	// Reason says why the inactive objects are unknown; set exactly when not Available (non-empty text).
	Reason string
}

type inactiveObjectsWire struct {
	Available bool      `json:"available"`
	Count     *int64    `json:"count,omitempty"`
	Names     *[]string `json:"names,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// Validate checks the inactive-objects report.
func (o InactiveObjects) Validate() error {
	if o.Available {
		if o.Reason != "" {
			return fieldError("reason", errors.New("must be unset when available"))
		}
		if o.Count == nil {
			return fieldError("count", errors.New("is required when available"))
		}
		if *o.Count < 0 {
			return fieldError("count", errors.New("must not be negative"))
		}
		if *o.Count != int64(len(o.Names)) {
			return fieldError("count", fmt.Errorf("%d does not equal the number of names (%d)", *o.Count, len(o.Names)))
		}
		for i, n := range o.Names {
			if n == "" {
				return fmt.Errorf("names[%d]: must not be empty", i)
			}
			if err := validateText(n); err != nil {
				return fmt.Errorf("names[%d]: %w", i, err)
			}
		}
		return nil
	}
	if o.Count != nil {
		return fieldError("count", errors.New("must be unset when not available"))
	}
	if o.Names != nil {
		return fieldError("names", errors.New("must be unset when not available"))
	}
	if o.Reason == "" {
		return fieldError("reason", errors.New("is required when not available"))
	}
	if err := validateText(o.Reason); err != nil {
		return fieldError("reason", err)
	}
	return nil
}

// MarshalJSON validates and writes the report.
func (o InactiveObjects) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("inactive objects: %w", err)
	}
	w := inactiveObjectsWire{Available: o.Available, Count: o.Count, Reason: o.Reason}
	if o.Available {
		names := o.Names
		if names == nil {
			names = []string{}
		}
		w.Names = &names
	}
	return json.Marshal(w)
}

// UnmarshalJSON parses and validates the report; unknown keys are ignored.
func (o *InactiveObjects) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"available": required, "count": optional, "names": optional, "reason": optional,
	}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("inactive objects: %w", err)
	}
	var out InactiveObjects
	for key, dst := range sortedItems(map[string]any{
		"available": &out.Available, "count": &out.Count, "reason": &out.Reason,
	}) {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("inactive objects: %w", err)
		}
	}
	var names []*string // pointers, so that a null entry is reported as such
	if err := decodeField(m, "names", &names); err != nil {
		return fmt.Errorf("inactive objects: %w", err)
	}
	if raw, ok := m["names"]; ok && !isNull(raw) {
		out.Names = make([]string, len(names))
		for i, n := range names {
			if n == nil {
				return fmt.Errorf("inactive objects: names[%d]: must not be null", i)
			}
			out.Names[i] = *n
		}
	}
	// Validate cannot tell an empty reason or missing names from an unset one; the keys can.
	if raw, ok := m["reason"]; ok && !isNull(raw) && out.Available {
		return fmt.Errorf("inactive objects: %w", fieldError("reason", errors.New("must be unset when available")))
	}
	if raw, ok := m["names"]; out.Available && (!ok || isNull(raw)) {
		return fmt.Errorf("inactive objects: %w", fieldError("names", errors.New("is required when available")))
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("inactive objects: %w", err)
	}
	*o = out
	return nil
}

// Caps are the limits of the service (spec section 3.1), reported so that clients know how it works.
// MaxWindowSeconds and MaxValues bound one internal SAP values call; the service splits a query window
// accordingly.
// MaxResolveWindowSeconds is the longest request window the service accepts for query, resolve and versions;
// longer windows are 400 invalid_request.
// Versions and resolve are not split and have no value cap.
// The internal window_too_large answer of SAP never reaches public clients.
type Caps struct {
	// MaxWindowSeconds is the longest window in seconds of one internal SAP values call; the service splits
	// longer query windows (at least 1).
	MaxWindowSeconds int64
	// MaxValues is the highest number of values of one internal SAP values call (at least 1).
	MaxValues int64
	// MaxResolveWindowSeconds is the longest request window in seconds the service accepts for query, resolve
	// and versions; a longer window is a 400 invalid_request (at least 1).
	MaxResolveWindowSeconds int64
}

type capsWire struct {
	MaxWindowSeconds        *int64 `json:"max_window_seconds"`
	MaxValues               *int64 `json:"max_values"`
	MaxResolveWindowSeconds *int64 `json:"max_resolve_window_seconds"`
}

// Validate checks that every cap is positive.
func (c Caps) Validate() error {
	for name, v := range sortedItems(map[string]int64{
		"max_window_seconds": c.MaxWindowSeconds, "max_values": c.MaxValues,
		"max_resolve_window_seconds": c.MaxResolveWindowSeconds,
	}) {
		if v < 1 {
			return fieldError(name, errors.New("must be at least 1"))
		}
	}
	return nil
}

// MarshalJSON validates and writes the caps.
func (c Caps) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("caps: %w", err)
	}
	return json.Marshal(capsWire{&c.MaxWindowSeconds, &c.MaxValues, &c.MaxResolveWindowSeconds})
}

// UnmarshalJSON parses and validates the caps; unknown keys are ignored.
func (c *Caps) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"max_window_seconds": required, "max_values": required, "max_resolve_window_seconds": required,
	}, ignoreUnknown)
	if err != nil {
		return err
	}
	var out Caps
	for key, dst := range sortedItems(map[string]any{
		"max_window_seconds": &out.MaxWindowSeconds, "max_values": &out.MaxValues,
		"max_resolve_window_seconds": &out.MaxResolveWindowSeconds,
	}) {
		if err := decodeField(m, key, dst); err != nil {
			return err
		}
	}
	if err := out.Validate(); err != nil {
		return err
	}
	*c = out
	return nil
}

// SystemInfo is the body of GET /api/v1/system (spec §4.4).
type SystemInfo struct {
	// ServiceVersion is the version of the Zeitreiher service (Go).
	ServiceVersion string
	// AbapPackageVersion is the version of the ABAP package /HFQ/ZEITREIHER.
	AbapPackageVersion string
	// SID is the SAP system id.
	SID string
	// Client is the SAP client the service reads from.
	Client string
	// Release is the SAP release.
	Release string
	// BusinessTimeZone is the configured SAP time zone code, e.g. "CET".
	BusinessTimeZone string
	// Intervals lists the supported request intervals, e.g. ["PT15M","PT60M"].
	Intervals []string
	// Units lists the canonical unit codes (UN/ECE Rec. 20, e.g. "KWH") the system can map to SAP units;
	// a request unit outside this list is a 400. Never nil after unmarshalling.
	Units []string
	// Caps are the per-call self-protection caps.
	Caps Caps
	// AbapLastSourceChangeUTC is the instant of the last source change of the ABAP package (whole seconds,
	// written in UTC "Z"); nil when unknown, then the key is omitted. Absent and null both mean unknown.
	AbapLastSourceChangeUTC *time.Time
	// AbapInactiveObjects reports the inactive objects of the ABAP package; nil when the backend does not
	// say, then the key is omitted.
	AbapInactiveObjects *InactiveObjects
	// AbapSourceFingerprint is the aggregate SHA-256 of the ABAP sources, exactly 64 lower-case hex
	// characters; nil when unknown, then the key is omitted.
	AbapSourceFingerprint *string
}

// Validate checks the system info.
func (s SystemInfo) Validate() error {
	for name, v := range sortedItems(map[string]string{
		"service_version": s.ServiceVersion, "abap_package_version": s.AbapPackageVersion, "sid": s.SID,
		"client": s.Client, "release": s.Release, "business_time_zone": s.BusinessTimeZone,
	}) {
		if v == "" {
			return fieldError(name, errors.New("must not be empty"))
		}
		if err := validateText(v); err != nil {
			return fieldError(name, err)
		}
	}
	if err := s.Caps.Validate(); err != nil {
		return fieldError("caps", err)
	}
	if s.AbapLastSourceChangeUTC != nil {
		if err := validateInstant(*s.AbapLastSourceChangeUTC, responseSide); err != nil {
			return fieldError("abap_last_source_change_utc", err)
		}
	}
	if s.AbapInactiveObjects != nil {
		if err := s.AbapInactiveObjects.Validate(); err != nil {
			return fieldError("abap_inactive_objects", err)
		}
	}
	if s.AbapSourceFingerprint != nil && !fingerprintPattern.MatchString(*s.AbapSourceFingerprint) {
		return fieldError("abap_source_fingerprint", errors.New("must be 64 lower-case hex characters (SHA-256)"))
	}
	for i, u := range s.Units {
		if err := validateUnitCode(u); err != nil {
			return fmt.Errorf("units[%d]: %w", i, err)
		}
	}
	for i, iv := range s.Intervals {
		if err := validateInterval(iv); err != nil {
			return fmt.Errorf("intervals[%d]: %w", i, err)
		}
	}
	return nil
}

type systemInfoWire struct {
	ServiceVersion     string   `json:"service_version"`
	AbapPackageVersion string   `json:"abap_package_version"`
	SID                string   `json:"sid"`
	Client             string   `json:"client"`
	Release            string   `json:"release"`
	BusinessTimeZone   string   `json:"business_time_zone"`
	Intervals          []string `json:"intervals"`
	Units              []string `json:"units"`
	Caps               Caps     `json:"caps"`

	AbapLastSourceChangeUTC *string          `json:"abap_last_source_change_utc,omitempty"`
	AbapInactiveObjects     *InactiveObjects `json:"abap_inactive_objects,omitempty"`
	AbapSourceFingerprint   *string          `json:"abap_source_fingerprint,omitempty"`
}

// MarshalJSON validates and writes the system info.
func (s SystemInfo) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("system info: %w", err)
	}
	w := systemInfoWire{
		ServiceVersion: s.ServiceVersion, AbapPackageVersion: s.AbapPackageVersion, SID: s.SID, Client: s.Client,
		Release: s.Release, BusinessTimeZone: s.BusinessTimeZone, Intervals: s.Intervals, Units: s.Units, Caps: s.Caps,
		AbapLastSourceChangeUTC: instantPtr(s.AbapLastSourceChangeUTC, responseSide),
		AbapInactiveObjects:     s.AbapInactiveObjects, AbapSourceFingerprint: s.AbapSourceFingerprint,
	}
	if w.Intervals == nil {
		w.Intervals = []string{}
	}
	if w.Units == nil {
		w.Units = []string{}
	}
	return json.Marshal(w)
}

// UnmarshalJSON parses and validates the system info.
func (s *SystemInfo) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"service_version": required, "abap_package_version": required, "sid": required, "client": required,
		"release": required, "business_time_zone": required, "intervals": required, "units": required, "caps": required,
		"abap_last_source_change_utc": optional, "abap_inactive_objects": optional, "abap_source_fingerprint": optional,
	}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	var out SystemInfo
	for key, dst := range sortedItems(map[string]any{
		"service_version": &out.ServiceVersion, "abap_package_version": &out.AbapPackageVersion, "sid": &out.SID,
		"client": &out.Client, "release": &out.Release, "business_time_zone": &out.BusinessTimeZone,
		"intervals": &out.Intervals, "caps": &out.Caps,
		"abap_inactive_objects": &out.AbapInactiveObjects, "abap_source_fingerprint": &out.AbapSourceFingerprint,
	}) {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("system info: %w", err)
		}
	}
	if raw, ok := m["abap_last_source_change_utc"]; ok && !isNull(raw) {
		t, err := parseInstant(raw, responseSide)
		if err != nil {
			return fmt.Errorf("system info: %w", fieldError("abap_last_source_change_utc", err))
		}
		out.AbapLastSourceChangeUTC = &t
	}
	var units []*string // pointers, so that a null entry is reported as such
	if err := decodeField(m, "units", &units); err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	out.Units = make([]string, len(units))
	for i, u := range units {
		if u == nil {
			return fmt.Errorf("system info: units[%d]: must not be null", i)
		}
		out.Units[i] = *u
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	*s = out
	return nil
}
