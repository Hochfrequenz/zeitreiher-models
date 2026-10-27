package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Selector kinds known to every Zeitreiher installation. Any other kind is a custom selector that a
// customer resolver implements.
const (
	SelectorByProfile = "profile"
	SelectorByMaLo    = "malo"
)

// Selector names the business object whose profile assignments are queried. It is a union over By:
//
//	{"by":"profile","profile":"50000000000056"}     – a profile number
//	{"by":"malo","malo":"87253749908"}              – a market location
//	{"by":"<name>","params":{"<key>":"<string>"}}  – a customer resolver
//
// Build selectors with ProfileSelector, MaLoSelector or CustomSelector. Only the fields of the
// variant named by By are used; the others must be empty.
type Selector struct {
	By      string
	Profile string
	MaLo    string
	Params  map[string]string
}

// ProfileSelector selects a profile by number (leading zeros allowed).
func ProfileSelector(profile string) Selector {
	return Selector{By: SelectorByProfile, Profile: profile}
}

// MaLoSelector selects the profiles assigned to a market location.
func MaLoSelector(malo string) Selector { return Selector{By: SelectorByMaLo, MaLo: malo} }

// CustomSelector selects via a customer resolver named by. params may be nil.
func CustomSelector(by string, params map[string]string) Selector {
	return Selector{By: by, Params: params}
}

// IsCustom reports whether s is a custom selector.
func (s Selector) IsCustom() bool { return s.By != SelectorByProfile && s.By != SelectorByMaLo }

// Validate checks the variant and its fields with the request rules (leading zeros allowed).
func (s Selector) Validate() error { return s.validate(requestSide) }

// validate checks the variant; echoed selectors in responses carry the normalized profile number.
func (s Selector) validate(sd side) error {
	switch s.By {
	case SelectorByProfile:
		if s.MaLo != "" || s.Params != nil {
			return errors.New("a profile selector has only by and profile")
		}
		if sd == responseSide {
			return wrapField("profile", validateResponseProfile(s.Profile))
		}
		return wrapField("profile", validateRequestProfile(s.Profile))
	case SelectorByMaLo:
		if s.Profile != "" || s.Params != nil {
			return errors.New("a malo selector has only by and malo")
		}
		return wrapField("malo", validateMaLo(s.MaLo))
	default:
		if !customByPattern.MatchString(s.By) {
			return wrapField("by", fmt.Errorf("%q is not a selector kind (lower-case letters, digits, underscores)", s.By))
		}
		if s.Profile != "" || s.MaLo != "" {
			return errors.New("a custom selector has only by and params")
		}
		for k := range s.Params {
			if k == "" {
				return wrapField("params", errors.New("keys must not be empty"))
			}
		}
		return nil
	}
}

// MarshalJSON writes the keys of the variant only, after validating with the request rules.
func (s Selector) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}
	return s.marshal()
}

// marshal writes the keys of the variant only, without validation.
func (s Selector) marshal() ([]byte, error) {
	switch s.By {
	case SelectorByProfile:
		return json.Marshal(struct {
			By      string `json:"by"`
			Profile string `json:"profile"`
		}{s.By, s.Profile})
	case SelectorByMaLo:
		return json.Marshal(struct {
			By   string `json:"by"`
			MaLo string `json:"malo"`
		}{s.By, s.MaLo})
	default:
		params := s.Params
		if params == nil {
			params = map[string]string{}
		}
		return json.Marshal(struct {
			By     string            `json:"by"`
			Params map[string]string `json:"params"`
		}{s.By, params})
	}
}

// UnmarshalJSON accepts exactly the keys of the variant named by "by" (request rules).
func (s *Selector) UnmarshalJSON(b []byte) error {
	out, err := decodeSelector(b, requestSide)
	if err != nil {
		return err
	}
	*s = out
	return nil
}

// decodeSelector decodes a selector with the rules of sd. In responses the selector is an echo:
// unknown keys are ignored and the profile number is normalized.
func decodeSelector(b []byte, sd side) (Selector, error) {
	head, err := checkObject(b, rules{"by": required}, ignoreUnknown)
	if err != nil {
		return Selector{}, fmt.Errorf("selector: %w", err)
	}
	var out Selector
	if err := decodeField(head, "by", &out.By); err != nil {
		return Selector{}, fmt.Errorf("selector: %w", err)
	}
	var r rules
	switch out.By {
	case SelectorByProfile:
		r = rules{"by": required, "profile": required}
	case SelectorByMaLo:
		r = rules{"by": required, "malo": required}
	default:
		r = rules{"by": required, "params": required}
	}
	m, err := checkObject(b, r, sd.unknownKeyPolicy())
	if err != nil {
		return Selector{}, fmt.Errorf("selector %q: %w", out.By, err)
	}
	for key, dst := range map[string]any{"profile": &out.Profile, "malo": &out.MaLo, "params": &out.Params} {
		if err := decodeField(m, key, dst); err != nil {
			return Selector{}, fmt.Errorf("selector %q: %w", out.By, err)
		}
	}
	if out.IsCustom() && out.Params == nil {
		out.Params = map[string]string{}
	}
	if err := out.validate(sd); err != nil {
		return Selector{}, fmt.Errorf("selector: %w", err)
	}
	return out, nil
}

// echoedSelector wraps a selector inside a response so that it is (un)marshalled with the
// response rules.
type echoedSelector Selector

func (e echoedSelector) MarshalJSON() ([]byte, error) {
	if err := Selector(e).validate(responseSide); err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}
	return Selector(e).marshal()
}

func (e *echoedSelector) UnmarshalJSON(b []byte) error {
	s, err := decodeSelector(b, responseSide)
	if err != nil {
		return err
	}
	*e = echoedSelector(s)
	return nil
}

// validateSelectors checks a non-empty selector list (request rules).
func validateSelectors(sel []Selector) error {
	if len(sel) == 0 {
		return wrapField("select", errors.New("at least one selector is required"))
	}
	for i, s := range sel {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("select[%d]: %w", i, err)
		}
	}
	return nil
}
