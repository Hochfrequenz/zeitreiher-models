package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Assignment is one profile assignment of a resolve result (spec §4.4). Either From and To are set
// (the assignment, cut to the request window) or Error is set (e.g. access_denied); an assignment
// with an error may still carry its window.
//
// Role, Unit and UnitRaw describe the profile of the assignment exactly like the fields of the same name
// in a series record. Without an Error, Role and Unit are always written (null when nil) and UnitRaw
// is required. With an Error they may be left out: nil Role, nil Unit and an empty UnitRaw are then omitted.
type Assignment struct {
	// Profile is the profile number without leading zeros.
	Profile string
	// Role is the profile role of the assignment (SAP EPROFASS role, e.g. "0001"; any non-empty text),
	// or nil when the assignment was not resolved through a role (e.g. by: profile).
	Role *string
	// From is the start of the assignment (inclusive), cut to the request window; set together with To.
	From *time.Time
	// To is the end of the assignment (exclusive), cut to the request window.
	To *time.Time
	// Unit is the canonical unit code (UN/ECE Rec. 20, e.g. "KWH") of the profile's values, or nil when the
	// SAP unit has no mapping.
	Unit *string
	// UnitRaw is the SAP unit key (T006 MSEHI) as SAP delivers it, mapped or not: any non-empty text of at
	// most 30 characters without control characters.
	UnitRaw string
	// Error is the assignment-level error; nil for a usable assignment.
	Error *ResultError
}

// VersionedAssignment is one profile assignment of a versions result: an Assignment plus the
// overlapping versions. Versions is written as [] when there are none and omitted with an error.
type VersionedAssignment struct {
	Assignment
	// Versions lists the versions overlapping the assignment; written as [] when empty, omitted with an Error.
	Versions []Version
}

// ResolveResult is the outcome for one request selector of the resolve endpoint: either
// Assignments (possibly empty) or Error.
type ResolveResult struct {
	// Selector is the index of the request selector (0-based).
	Selector int
	// Select is the request selector, echoed.
	Select Selector
	// Assignments are the profile assignments of the selector; possibly empty, unset with an Error.
	Assignments []Assignment
	// Error is a selector-level error; exactly one of Assignments or Error is set.
	Error *ResultError
}

// VersionsResult is the outcome for one request selector of the versions endpoint.
type VersionsResult struct {
	// Selector is the index of the request selector (0-based).
	Selector int
	// Select is the request selector, echoed.
	Select Selector
	// Assignments are the profile assignments with their versions; possibly empty, unset with an Error.
	Assignments []VersionedAssignment
	// Error is a selector-level error; exactly one of Assignments or Error is set.
	Error *ResultError
}

// ResolveResponse is the body of a successful resolve response: {"results":[…]}.
type ResolveResponse struct {
	// Results holds one result per request selector, in request order.
	Results []ResolveResult `json:"results"`
}

// VersionsResponse is the body of a successful versions response: {"results":[…]}.
type VersionsResponse struct {
	// Results holds one result per request selector, in request order.
	Results []VersionsResult `json:"results"`
}

// validate checks the assignment window rules.
func (a Assignment) validate() error {
	if err := validateResponseProfile(a.Profile); err != nil {
		return fieldError("profile", err)
	}
	if (a.From == nil) != (a.To == nil) {
		return errors.New("from and to must be set together")
	}
	if a.From == nil && a.Error == nil {
		return errors.New("an assignment without error needs from and to")
	}
	if a.From != nil {
		if err := validateWindow(*a.From, *a.To, responseSide); err != nil {
			return err
		}
	}
	if a.Role != nil {
		if err := validateToken(*a.Role); err != nil {
			return fieldError("role", err)
		}
	}
	if a.Unit != nil {
		if err := validateUnitCode(*a.Unit); err != nil {
			return fieldError("unit", err)
		}
	}
	if a.Error == nil || a.UnitRaw != "" {
		if err := validateRawUnit(a.UnitRaw); err != nil {
			return fieldError("unit_raw", err)
		}
	}
	if a.Error != nil {
		return wrapField("error.code", a.Error.Code.Validate())
	}
	return nil
}

type assignmentWire struct {
	Profile  string          `json:"profile"`
	Role     json.RawMessage `json:"role,omitempty"`
	From     *string         `json:"from,omitempty"`
	To       *string         `json:"to,omitempty"`
	Unit     json.RawMessage `json:"unit,omitempty"`
	UnitRaw  string          `json:"unit_raw,omitempty"`
	Versions *[]Version      `json:"versions,omitempty"`
	Error    *ResultError    `json:"error,omitempty"`
}

// nullableText renders an optional string for a key that is written as null when unset, except in an
// assignment with an error, where an unset value is omitted.
func nullableText(s *string, omitNil bool) json.RawMessage {
	if s == nil {
		if omitNil {
			return nil
		}
		return json.RawMessage("null")
	}
	b, _ := json.Marshal(*s) // a validated string always marshals
	return b
}

func (a Assignment) wire() assignmentWire {
	omit := a.Error != nil
	return assignmentWire{
		Profile: a.Profile, Role: nullableText(a.Role, omit), From: instantPtr(a.From, responseSide),
		To: instantPtr(a.To, responseSide), Unit: nullableText(a.Unit, omit), UnitRaw: a.UnitRaw, Error: a.Error,
	}
}

// assignmentRulesFor returns the key rules for the object b: role and unit (nullable) and unit_raw are
// required unless the assignment carries an error.
func assignmentRulesFor(b []byte) rules {
	r := rules{"profile": required, "from": optional, "to": optional, "error": optional}
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(b, &probe) // a non-object is rejected by checkObject
	if raw, hasError := probe["error"]; hasError && !isNull(raw) {
		r["role"], r["unit"], r["unit_raw"] = optional, optional, optional
	} else {
		r["role"], r["unit"], r["unit_raw"] = requiredNullable, requiredNullable, required
	}
	return r
}

func decodeAssignment(m map[string]json.RawMessage) (Assignment, error) {
	var a Assignment
	var err error
	if err = decodeField(m, "profile", &a.Profile); err != nil {
		return a, err
	}
	if raw, ok := m["role"]; ok && !isNull(raw) {
		a.Role = new(string)
	}
	if raw, ok := m["unit"]; ok && !isNull(raw) {
		a.Unit = new(string)
	}
	for key, dst := range map[string]any{"role": a.Role, "unit": a.Unit, "unit_raw": &a.UnitRaw} {
		if err = decodeField(m, key, dst); err != nil {
			return a, err
		}
	}
	if a.From, err = decodeInstant(m, "from", responseSide); err != nil {
		return a, err
	}
	if a.To, err = decodeInstant(m, "to", responseSide); err != nil {
		return a, err
	}
	if _, ok := m["error"]; ok {
		a.Error = new(ResultError)
		if err = decodeField(m, "error", a.Error); err != nil {
			return a, err
		}
	}
	return a, a.validate()
}

// MarshalJSON validates and writes the assignment.
func (a Assignment) MarshalJSON() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, fmt.Errorf("assignment: %w", err)
	}
	return json.Marshal(a.wire())
}

// UnmarshalJSON parses and validates the assignment; unknown keys (such as versions) are ignored.
func (a *Assignment) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, assignmentRulesFor(b), ignoreUnknown)
	if err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	out, err := decodeAssignment(m)
	if err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	*a = out
	return nil
}

func (v VersionedAssignment) validate() error {
	if err := v.Assignment.validate(); err != nil {
		return err
	}
	if v.Error != nil && len(v.Versions) > 0 {
		return errors.New("an assignment with error has no versions")
	}
	for i, ver := range v.Versions {
		if err := ver.Validate(); err != nil {
			return fmt.Errorf("versions[%d]: %w", i, err)
		}
	}
	return nil
}

// MarshalJSON validates and writes the assignment.
func (v VersionedAssignment) MarshalJSON() ([]byte, error) {
	if err := v.validate(); err != nil {
		return nil, fmt.Errorf("assignment: %w", err)
	}
	w := v.wire()
	if v.Error == nil {
		versions := v.Versions
		if versions == nil {
			versions = []Version{}
		}
		w.Versions = &versions
	}
	return json.Marshal(w)
}

// UnmarshalJSON parses and validates the assignment; versions is required unless error is set.
func (v *VersionedAssignment) UnmarshalJSON(b []byte) error {
	r := rules{"versions": optional}
	for k, p := range assignmentRulesFor(b) {
		r[k] = p
	}
	m, err := checkObject(b, r, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	var out VersionedAssignment
	if out.Assignment, err = decodeAssignment(m); err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	_, hasVersions := m["versions"]
	if !hasVersions && out.Error == nil {
		return errors.New("assignment: versions: is required")
	}
	if hasVersions && out.Error != nil {
		return errors.New("assignment: an assignment with error has no versions")
	}
	if err := decodeField(m, "versions", &out.Versions); err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	if out.Error == nil && out.Versions == nil {
		out.Versions = []Version{}
	}
	if err := out.validate(); err != nil {
		return fmt.Errorf("assignment: %w", err)
	}
	*v = out
	return nil
}

// resultWire is the wire form shared by both result kinds.
type resultWire[A any] struct {
	Selector    int            `json:"selector"`
	Select      echoedSelector `json:"select"`
	Assignments *[]A           `json:"assignments,omitempty"`
	Error       *ResultError   `json:"error,omitempty"`
}

func marshalResult[A any](selector int, sel Selector, assignments []A, rerr *ResultError) ([]byte, error) {
	if err := validateMin("selector", selector, 0); err != nil {
		return nil, err
	}
	w := resultWire[A]{Selector: selector, Select: echoedSelector(sel), Error: rerr}
	if rerr == nil {
		if assignments == nil {
			assignments = []A{}
		}
		w.Assignments = &assignments
	} else if len(assignments) > 0 {
		return nil, errors.New("result: a result with error has no assignments")
	}
	return json.Marshal(w)
}

func unmarshalResult[A any](b []byte) (resultWire[A], error) {
	m, err := checkObject(b, rules{
		"selector": required, "select": required, "assignments": optional, "error": optional,
	}, ignoreUnknown)
	if err != nil {
		return resultWire[A]{}, fmt.Errorf("result: %w", err)
	}
	var w resultWire[A]
	_, hasAssignments := m["assignments"]
	_, hasError := m["error"]
	if hasAssignments == hasError {
		return w, errors.New("result: exactly one of assignments or error must be set")
	}
	if hasAssignments {
		w.Assignments = new([]A)
	} else {
		w.Error = new(ResultError)
	}
	for key, dst := range map[string]any{
		"selector": &w.Selector, "select": &w.Select, "assignments": w.Assignments, "error": w.Error,
	} {
		if err := decodeField(m, key, dst); err != nil {
			return w, fmt.Errorf("result: %w", err)
		}
	}
	if err := validateMin("selector", w.Selector, 0); err != nil {
		return w, fmt.Errorf("result: %w", err)
	}
	if w.Assignments != nil && *w.Assignments == nil {
		*w.Assignments = []A{}
	}
	return w, nil
}

// MarshalJSON validates and writes the result.
func (r ResolveResult) MarshalJSON() ([]byte, error) {
	return marshalResult(r.Selector, r.Select, r.Assignments, r.Error)
}

// UnmarshalJSON parses and validates the result.
func (r *ResolveResult) UnmarshalJSON(b []byte) error {
	w, err := unmarshalResult[Assignment](b)
	if err != nil {
		return err
	}
	*r = ResolveResult{Selector: w.Selector, Select: Selector(w.Select), Error: w.Error}
	if w.Assignments != nil {
		r.Assignments = *w.Assignments
	}
	return nil
}

// MarshalJSON validates and writes the result.
func (r VersionsResult) MarshalJSON() ([]byte, error) {
	return marshalResult(r.Selector, r.Select, r.Assignments, r.Error)
}

// UnmarshalJSON parses and validates the result.
func (r *VersionsResult) UnmarshalJSON(b []byte) error {
	w, err := unmarshalResult[VersionedAssignment](b)
	if err != nil {
		return err
	}
	*r = VersionsResult{Selector: w.Selector, Select: Selector(w.Select), Error: w.Error}
	if w.Assignments != nil {
		r.Assignments = *w.Assignments
	}
	return nil
}

// MarshalJSON writes {"results":[…]}; nil Results is written as [].
func (r ResolveResponse) MarshalJSON() ([]byte, error) {
	results := r.Results
	if results == nil {
		results = []ResolveResult{}
	}
	return json.Marshal(struct {
		Results []ResolveResult `json:"results"`
	}{results})
}

// UnmarshalJSON parses the response.
func (r *ResolveResponse) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"results": required}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("resolve response: %w", err)
	}
	var out ResolveResponse
	if err := decodeField(m, "results", &out.Results); err != nil {
		return fmt.Errorf("resolve response: %w", err)
	}
	*r = out
	return nil
}

// MarshalJSON writes {"results":[…]}; nil Results is written as [].
func (r VersionsResponse) MarshalJSON() ([]byte, error) {
	results := r.Results
	if results == nil {
		results = []VersionsResult{}
	}
	return json.Marshal(struct {
		Results []VersionsResult `json:"results"`
	}{results})
}

// UnmarshalJSON parses the response.
func (r *VersionsResponse) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"results": required}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("versions response: %w", err)
	}
	var out VersionsResponse
	if err := decodeField(m, "results", &out.Results); err != nil {
		return fmt.Errorf("versions response: %w", err)
	}
	*r = out
	return nil
}
