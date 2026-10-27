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
type Assignment struct {
	Profile string
	From    *time.Time
	To      *time.Time
	Error   *ResultError
}

// VersionedAssignment is one profile assignment of a versions result: an Assignment plus the
// overlapping versions. Versions is written as [] when there are none and omitted with an error.
type VersionedAssignment struct {
	Assignment
	Versions []Version
}

// ResolveResult is the outcome for one request selector of the resolve endpoint: either
// Assignments (possibly empty) or Error.
type ResolveResult struct {
	Selector    int
	Select      Selector
	Assignments []Assignment
	Error       *ResultError
}

// VersionsResult is the outcome for one request selector of the versions endpoint.
type VersionsResult struct {
	Selector    int
	Select      Selector
	Assignments []VersionedAssignment
	Error       *ResultError
}

// ResolveResponse is the body of a successful resolve response: {"results":[…]}.
type ResolveResponse struct {
	Results []ResolveResult `json:"results"`
}

// VersionsResponse is the body of a successful versions response: {"results":[…]}.
type VersionsResponse struct {
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
	if a.Error != nil {
		return wrapField("error.code", a.Error.Code.Validate())
	}
	return nil
}

type assignmentWire struct {
	Profile  string       `json:"profile"`
	From     *string      `json:"from,omitempty"`
	To       *string      `json:"to,omitempty"`
	Versions *[]Version   `json:"versions,omitempty"`
	Error    *ResultError `json:"error,omitempty"`
}

func (a Assignment) wire() assignmentWire {
	return assignmentWire{
		Profile: a.Profile, From: instantPtr(a.From, responseSide), To: instantPtr(a.To, responseSide), Error: a.Error,
	}
}

var assignmentRules = rules{"profile": required, "from": optional, "to": optional, "error": optional}

func decodeAssignment(m map[string]json.RawMessage) (Assignment, error) {
	var a Assignment
	var err error
	if err = decodeField(m, "profile", &a.Profile); err != nil {
		return a, err
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
	m, err := checkObject(b, assignmentRules, ignoreUnknown)
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
	for k, p := range assignmentRules {
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
