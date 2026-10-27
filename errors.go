package zeitreihermodels

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// RequestErrorCode is a request-level error code. It appears in the error envelope of an HTTP
// response that failed as a whole (before any NDJSON record was written).
type RequestErrorCode string

// Request-level error codes (spec §4.3, consolidated list).
const (
	RequestErrorBadRequest          RequestErrorCode = "bad_request"
	RequestErrorUnauthorized        RequestErrorCode = "unauthorized"
	RequestErrorForbidden           RequestErrorCode = "forbidden"
	RequestErrorCursorInvalid       RequestErrorCode = "cursor_invalid"
	RequestErrorCursorMismatch      RequestErrorCode = "cursor_mismatch"
	RequestErrorCursorExpired       RequestErrorCode = "cursor_expired"
	RequestErrorCursorStale         RequestErrorCode = "cursor_stale"
	RequestErrorUpstreamUnavailable RequestErrorCode = "upstream_unavailable"
	RequestErrorUpstreamTimeout     RequestErrorCode = "upstream_timeout"
)

var requestErrorCodes = []RequestErrorCode{
	RequestErrorBadRequest, RequestErrorUnauthorized, RequestErrorForbidden, RequestErrorCursorInvalid,
	RequestErrorCursorMismatch, RequestErrorCursorExpired, RequestErrorCursorStale,
	RequestErrorUpstreamUnavailable, RequestErrorUpstreamTimeout,
}

// AllRequestErrorCodes returns every request-level error code in specification order.
func AllRequestErrorCodes() []RequestErrorCode { return slices.Clone(requestErrorCodes) }

// IsKnown reports whether c is one of the request-level codes this version of the models knows.
// Parsing accepts unknown snake_case codes so that a newer server does not break older clients.
func (c RequestErrorCode) IsKnown() bool { return slices.Contains(requestErrorCodes, c) }

// Validate checks the code syntax (snake_case).
func (c RequestErrorCode) Validate() error { return validateCode(string(c)) }

// MarshalJSON refuses codes that are not snake_case.
func (c RequestErrorCode) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(string(c))
}

// UnmarshalJSON accepts any snake_case code; use IsKnown to tell known from unknown codes.
func (c *RequestErrorCode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("must be a string")
	}
	if err := validateCode(s); err != nil {
		return err
	}
	*c = RequestErrorCode(s)
	return nil
}

// RecordErrorCode is a per-selector or per-assignment error code. It appears in NDJSON error
// records and in the error objects of resolve/versions results; the rest of the response continues.
type RecordErrorCode string

// Record-level error codes (spec §4.3, consolidated list).
const (
	RecordErrorSelectorNotFound        RecordErrorCode = "selector_not_found"
	RecordErrorNoAssignment            RecordErrorCode = "no_assignment"
	RecordErrorUnknownSelector         RecordErrorCode = "unknown_selector"
	RecordErrorBadSelector             RecordErrorCode = "bad_selector"
	RecordErrorAccessDenied            RecordErrorCode = "access_denied"
	RecordErrorProfileNotFound         RecordErrorCode = "profile_not_found"
	RecordErrorProfileTimeZoneMissing  RecordErrorCode = "profile_time_zone_missing"
	RecordErrorIntervalSizeUnsupported RecordErrorCode = "interval_size_unsupported"
	RecordErrorWindowUnaligned         RecordErrorCode = "window_unaligned"
	RecordErrorAssignmentUnaligned     RecordErrorCode = "assignment_unaligned"
	RecordErrorVersionNotFound         RecordErrorCode = "version_not_found"
	RecordErrorConversionNotAllowed    RecordErrorCode = "conversion_not_allowed"
	RecordErrorAmbiguousLocalTime      RecordErrorCode = "ambiguous_local_time"
	RecordErrorBadiFailed              RecordErrorCode = "badi_failed"
)

var recordErrorCodes = []RecordErrorCode{
	RecordErrorSelectorNotFound, RecordErrorNoAssignment, RecordErrorUnknownSelector, RecordErrorBadSelector,
	RecordErrorAccessDenied, RecordErrorProfileNotFound, RecordErrorProfileTimeZoneMissing,
	RecordErrorIntervalSizeUnsupported, RecordErrorWindowUnaligned, RecordErrorAssignmentUnaligned,
	RecordErrorVersionNotFound, RecordErrorConversionNotAllowed, RecordErrorAmbiguousLocalTime,
	RecordErrorBadiFailed,
}

// AllRecordErrorCodes returns every record-level error code in specification order.
func AllRecordErrorCodes() []RecordErrorCode { return slices.Clone(recordErrorCodes) }

// IsKnown reports whether c is one of the record-level codes this version of the models knows.
// Parsing accepts unknown snake_case codes so that a newer server does not break older clients.
func (c RecordErrorCode) IsKnown() bool { return slices.Contains(recordErrorCodes, c) }

// Validate checks the code syntax (snake_case).
func (c RecordErrorCode) Validate() error { return validateCode(string(c)) }

// MarshalJSON refuses codes that are not snake_case.
func (c RecordErrorCode) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(string(c))
}

// UnmarshalJSON accepts any snake_case code; use IsKnown to tell known from unknown codes.
func (c *RecordErrorCode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New("must be a string")
	}
	if err := validateCode(s); err != nil {
		return err
	}
	*c = RecordErrorCode(s)
	return nil
}

// ErrorEnvelope is the body of every failed (non-2xx) HTTP response:
// {"error":{"code":"…","message":"…","request_id":"…"}}. It has the shape of btpingo.ErrorEnvelope.
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the content of an ErrorEnvelope.
type ErrorDetail struct {
	// Code is stable and machine-readable; clients switch on it, never on Message.
	Code RequestErrorCode `json:"code"`
	// Message is a human-readable explanation.
	Message string `json:"message"`
	// RequestID correlates the response with server logs; empty when unknown (omitted on the wire).
	RequestID string `json:"request_id,omitempty"`
}

// Validate checks the envelope.
func (e ErrorEnvelope) Validate() error {
	if err := e.Error.Code.Validate(); err != nil {
		return fieldError("error.code", err)
	}
	return nil
}

// MarshalJSON validates before writing.
func (e ErrorEnvelope) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	type wire ErrorEnvelope
	return json.Marshal(wire(e))
}

// UnmarshalJSON enforces the presence rules of the envelope.
func (e *ErrorEnvelope) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"error": required}, ignoreUnknown)
	if err != nil {
		return fmt.Errorf("error envelope: %w", err)
	}
	var out ErrorEnvelope
	if err := decodeField(m, "error", &out.Error); err != nil {
		return fmt.Errorf("error envelope: %w", err)
	}
	*e = out
	return nil
}

// UnmarshalJSON enforces the presence rules of the detail.
func (d *ErrorDetail) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"code": required, "message": required, "request_id": optional}, ignoreUnknown)
	if err != nil {
		return err
	}
	var out ErrorDetail
	for key, dst := range map[string]any{"code": &out.Code, "message": &out.Message, "request_id": &out.RequestID} {
		if err := decodeField(m, key, dst); err != nil {
			return err
		}
	}
	if _, ok := m["request_id"]; ok && out.RequestID == "" {
		return errors.New("request_id: must not be empty")
	}
	*d = out
	return nil
}

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validateCode checks that an error code is snake_case.
func validateCode(c string) error {
	if !codePattern.MatchString(c) {
		return fmt.Errorf("%q is not an error code (snake_case)", c)
	}
	return nil
}

// ResultError is the error object of a resolve/versions result or assignment: {"code":"…"}.
type ResultError struct {
	Code RecordErrorCode `json:"code"`
}

// MarshalJSON validates before writing.
func (r ResultError) MarshalJSON() ([]byte, error) {
	type wire ResultError
	return json.Marshal(wire(r)) // RecordErrorCode.MarshalJSON validates the code syntax
}

// UnmarshalJSON enforces the presence rules.
func (r *ResultError) UnmarshalJSON(b []byte) error {
	m, err := checkObject(b, rules{"code": required}, ignoreUnknown)
	if err != nil {
		return err
	}
	var out ResultError
	if err := decodeField(m, "code", &out.Code); err != nil {
		return err
	}
	*r = out
	return nil
}
