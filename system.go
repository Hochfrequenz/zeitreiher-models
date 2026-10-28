package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
)

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
	for name, v := range map[string]int64{
		"max_window_seconds": c.MaxWindowSeconds, "max_values": c.MaxValues,
		"max_resolve_window_seconds": c.MaxResolveWindowSeconds,
	} {
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
	for key, dst := range map[string]any{
		"max_window_seconds": &out.MaxWindowSeconds, "max_values": &out.MaxValues,
		"max_resolve_window_seconds": &out.MaxResolveWindowSeconds,
	} {
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
}

// Validate checks the system info.
func (s SystemInfo) Validate() error {
	for name, v := range map[string]string{
		"service_version": s.ServiceVersion, "abap_package_version": s.AbapPackageVersion, "sid": s.SID,
		"client": s.Client, "release": s.Release, "business_time_zone": s.BusinessTimeZone,
	} {
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
}

// MarshalJSON validates and writes the system info.
func (s SystemInfo) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("system info: %w", err)
	}
	w := systemInfoWire(s)
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
	}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	var out SystemInfo
	for key, dst := range map[string]any{
		"service_version": &out.ServiceVersion, "abap_package_version": &out.AbapPackageVersion, "sid": &out.SID,
		"client": &out.Client, "release": &out.Release, "business_time_zone": &out.BusinessTimeZone,
		"intervals": &out.Intervals, "caps": &out.Caps,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("system info: %w", err)
		}
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
