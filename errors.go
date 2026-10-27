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

// Request-level error codes (spec §4.3, consolidated list). The first eight are btpingo's request-level
// vocabulary (btpingo.ErrorCode in httperr.go), shared by the Hochfrequenz BTP services; the cursor
// codes and upstream_timeout are Zeitreiher's own.
const (
	// RequestErrorInvalidRequest: The request is invalid: missing offset, fractional seconds, from not
	// before to, bad selector shape, too many selectors, unsupported interval, unknown unit, or a window
	// longer than the resolve limit. HTTP 400; gRPC INVALID_ARGUMENT.
	RequestErrorInvalidRequest RequestErrorCode = "invalid_request"
	// RequestErrorUnauthorized: Authentication is missing or invalid. HTTP 401; gRPC UNAUTHENTICATED.
	RequestErrorUnauthorized RequestErrorCode = "unauthorized"
	// RequestErrorForbidden: The caller is authenticated but not allowed to use the API. HTTP 403; gRPC
	// PERMISSION_DENIED.
	RequestErrorForbidden RequestErrorCode = "forbidden"
	// RequestErrorNotFound: Unknown route. HTTP 404; gRPC NOT_FOUND.
	RequestErrorNotFound RequestErrorCode = "not_found"
	// RequestErrorMethodNotAllowed: The HTTP method is not allowed for the route. HTTP 405; gRPC
	// UNIMPLEMENTED.
	RequestErrorMethodNotAllowed RequestErrorCode = "method_not_allowed"
	// RequestErrorRequestTooLarge: The request body exceeds the size limit. HTTP 413; gRPC
	// RESOURCE_EXHAUSTED.
	RequestErrorRequestTooLarge RequestErrorCode = "request_too_large"
	// RequestErrorUpstreamUnreachable: The SAP system could not be reached, or failed in a way the service
	// cannot pass on, before the first record. HTTP 502; gRPC UNAVAILABLE.
	RequestErrorUpstreamUnreachable RequestErrorCode = "upstream_unreachable"
	// RequestErrorInternal: Unexpected failure of the service before the first record. HTTP 500; gRPC
	// INTERNAL.
	RequestErrorInternal RequestErrorCode = "internal"
	// RequestErrorCursorInvalid: The cursor is malformed or its signature is bad. HTTP 400; gRPC
	// INVALID_ARGUMENT.
	RequestErrorCursorInvalid RequestErrorCode = "cursor_invalid"
	// RequestErrorCursorMismatch: The cursor does not match this request. HTTP 400; gRPC INVALID_ARGUMENT.
	RequestErrorCursorMismatch RequestErrorCode = "cursor_mismatch"
	// RequestErrorCursorExpired: The cursor has expired. HTTP 400; gRPC INVALID_ARGUMENT.
	RequestErrorCursorExpired RequestErrorCode = "cursor_expired"
	// RequestErrorCursorStale: The profile assignments changed since the cursor was issued; restart the
	// query. HTTP 409; gRPC ABORTED.
	RequestErrorCursorStale RequestErrorCode = "cursor_stale"
	// RequestErrorUpstreamTimeout: The SAP system did not answer in time before the first record. HTTP
	// 504; gRPC DEADLINE_EXCEEDED.
	RequestErrorUpstreamTimeout RequestErrorCode = "upstream_timeout"
)

var requestErrorCodes = []RequestErrorCode{
	RequestErrorInvalidRequest, RequestErrorUnauthorized, RequestErrorForbidden, RequestErrorNotFound,
	RequestErrorMethodNotAllowed, RequestErrorRequestTooLarge, RequestErrorUpstreamUnreachable,
	RequestErrorInternal, RequestErrorCursorInvalid, RequestErrorCursorMismatch, RequestErrorCursorExpired,
	RequestErrorCursorStale, RequestErrorUpstreamTimeout,
}

// AllRequestErrorCodes returns every request-level error code in specification order.
func AllRequestErrorCodes() []RequestErrorCode { return slices.Clone(requestErrorCodes) }

// IsKnown reports whether c is one of the request-level codes this version of the models knows.
// Parsing accepts unknown snake_case codes so that a newer server does not break older clients.
func (c RequestErrorCode) IsKnown() bool { return slices.Contains(requestErrorCodes, c) }

// Validate checks the code syntax (snake_case). It accepts unknown codes, as parsing does.
func (c RequestErrorCode) Validate() error { return validateCode(string(c)) }

// MarshalJSON writes only the public codes: error codes are strict on write, lenient on read.
func (c RequestErrorCode) MarshalJSON() ([]byte, error) {
	return marshalCode(string(c), c.IsKnown())
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
	// RecordErrorSelectorNotFound: The selector matches no business object (e.g. an unknown market
	// location).
	RecordErrorSelectorNotFound RecordErrorCode = "selector_not_found"
	// RecordErrorNoAssignment: The selector exists but has no profile assignment in the request window.
	RecordErrorNoAssignment RecordErrorCode = "no_assignment"
	// RecordErrorUnknownSelector: The selector kind (by) is not known to this installation.
	RecordErrorUnknownSelector RecordErrorCode = "unknown_selector"
	// RecordErrorBadSelector: The selector's parameters are not acceptable to its resolver.
	RecordErrorBadSelector RecordErrorCode = "bad_selector"
	// RecordErrorAccessDenied: The caller is not allowed to read this profile.
	RecordErrorAccessDenied RecordErrorCode = "access_denied"
	// RecordErrorProfileNotFound: The profile does not exist.
	RecordErrorProfileNotFound RecordErrorCode = "profile_not_found"
	// RecordErrorProfileTimeZoneMissing: The profile has no time zone, so its values cannot be read.
	RecordErrorProfileTimeZoneMissing RecordErrorCode = "profile_time_zone_missing"
	// RecordErrorIntervalSizeUnsupported: The profile's interval size is not supported.
	RecordErrorIntervalSizeUnsupported RecordErrorCode = "interval_size_unsupported"
	// RecordErrorWindowUnaligned: The request window is not aligned to the profile's intervals.
	RecordErrorWindowUnaligned RecordErrorCode = "window_unaligned"
	// RecordErrorAssignmentUnaligned: The bounds of the profile assignment are not aligned to the
	// profile's intervals.
	RecordErrorAssignmentUnaligned RecordErrorCode = "assignment_unaligned"
	// RecordErrorVersionNotFound: The requested version number does not exist for the profile.
	RecordErrorVersionNotFound RecordErrorCode = "version_not_found"
	// RecordErrorVersionDeleted: The requested {"number": N} names a version that is flagged as deleted
	// (it is still listed by the versions endpoint, with deleted: true).
	RecordErrorVersionDeleted RecordErrorCode = "version_deleted"
	// RecordErrorConversionNotAllowed: The requested unit or interval conversion is not allowed for the
	// profile.
	RecordErrorConversionNotAllowed RecordErrorCode = "conversion_not_allowed"
	// RecordErrorAmbiguousLocalTime: A local time in the window occurs twice (daylight saving change) and
	// cannot be mapped to one instant.
	RecordErrorAmbiguousLocalTime RecordErrorCode = "ambiguous_local_time"
	// RecordErrorBadiFailed: A customer BAdI (resolver or metadata) failed.
	RecordErrorBadiFailed RecordErrorCode = "badi_failed"
)

var recordErrorCodes = []RecordErrorCode{
	RecordErrorSelectorNotFound, RecordErrorNoAssignment, RecordErrorUnknownSelector, RecordErrorBadSelector,
	RecordErrorAccessDenied, RecordErrorProfileNotFound, RecordErrorProfileTimeZoneMissing,
	RecordErrorIntervalSizeUnsupported, RecordErrorWindowUnaligned, RecordErrorAssignmentUnaligned,
	RecordErrorVersionNotFound, RecordErrorVersionDeleted, RecordErrorConversionNotAllowed, RecordErrorAmbiguousLocalTime,
	RecordErrorBadiFailed,
}

// AllRecordErrorCodes returns every record-level error code in specification order.
func AllRecordErrorCodes() []RecordErrorCode { return slices.Clone(recordErrorCodes) }

// IsKnown reports whether c is one of the record-level codes this version of the models knows.
// Parsing accepts unknown snake_case codes so that a newer server does not break older clients.
func (c RecordErrorCode) IsKnown() bool { return slices.Contains(recordErrorCodes, c) }

// Validate checks the code syntax (snake_case). It accepts unknown codes, as parsing does.
func (c RecordErrorCode) Validate() error { return validateCode(string(c)) }

// MarshalJSON writes only the public codes: error codes are strict on write, lenient on read.
func (c RecordErrorCode) MarshalJSON() ([]byte, error) {
	return marshalCode(string(c), c.IsKnown())
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
	// Error is the error code, message and optional request id.
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
	if err := validateText(e.Error.Message); err != nil {
		return fieldError("error.message", err)
	}
	return wrapField("error.request_id", validateText(e.Error.RequestID))
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

// marshalCode writes a known error code. Unknown codes are refused even when they are snake_case: a
// parsed response may carry codes of a newer server, but a writer emits only the public vocabulary.
func marshalCode(c string, known bool) ([]byte, error) {
	if err := validateCode(c); err != nil {
		return nil, err
	}
	if !known {
		return nil, fmt.Errorf("%q is not a public error code; only the known codes can be written", c)
	}
	return json.Marshal(c)
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
	// Code is the record-level error code (snake_case); unknown codes are kept on read (see IsKnown)
	// but refused on write.
	Code RecordErrorCode `json:"code"`
}

// MarshalJSON validates before writing.
func (r ResultError) MarshalJSON() ([]byte, error) {
	type wire ResultError
	return json.Marshal(wire(r)) // RecordErrorCode.MarshalJSON refuses unknown codes
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
