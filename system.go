package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SystemInfo is the body of GET /api/v1/system (spec §4.4).
type SystemInfo struct {
	// ServiceVersion is the version of the Zeitreiher service (Go).
	ServiceVersion string
	// AbapPackageVersion is the version of the ABAP package /HFQ/ZEITREIHER.
	AbapPackageVersion string
	// SID and Client identify the SAP system.
	SID    string
	Client string
	// Release is the SAP release.
	Release string
	// BusinessTimeZone is the configured SAP time zone code, e.g. "CET".
	BusinessTimeZone string
	// Intervals lists the supported request intervals, e.g. ["PT15M","PT60M"].
	Intervals []string
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
	return json.Marshal(w)
}

// UnmarshalJSON parses and validates the system info.
func (s *SystemInfo) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{
		"service_version": required, "abap_package_version": required, "sid": required, "client": required,
		"release": required, "business_time_zone": required, "intervals": required,
	}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	var out SystemInfo
	for key, dst := range map[string]any{
		"service_version": &out.ServiceVersion, "abap_package_version": &out.AbapPackageVersion, "sid": &out.SID,
		"client": &out.Client, "release": &out.Release, "business_time_zone": &out.BusinessTimeZone,
		"intervals": &out.Intervals,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return fmt.Errorf("system info: %w", err)
		}
	}
	if err := out.Validate(); err != nil {
		return fmt.Errorf("system info: %w", err)
	}
	*s = out
	return nil
}
