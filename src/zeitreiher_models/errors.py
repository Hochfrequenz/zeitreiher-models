"""Error codes and the error shapes of the API."""

from enum import StrEnum
from typing import Annotated, Any, ClassVar

from pydantic import BeforeValidator, Field, PlainSerializer

from zeitreiher_models._base import DocumentedStrEnum, ResponseModel
from zeitreiher_models.scalars import CodeStr, Text


class RequestErrorCode(DocumentedStrEnum):
    """Request-level error codes (spec §4.3), carried by :class:`ErrorEnvelope`.

    The first eight are btpingo's request-level vocabulary (``btpingo.ErrorCode`` in ``httperr.go``), shared
    by the Hochfrequenz BTP services; the cursor codes and ``upstream_timeout`` are Zeitreiher's own.
    """

    INVALID_REQUEST = "invalid_request"
    """The request is invalid: missing offset, fractional seconds, from not before to, bad selector shape, too many
    selectors, unsupported interval, unknown unit, or a window longer than the resolve limit. HTTP 400; gRPC
    INVALID_ARGUMENT.
    """
    UNAUTHORIZED = "unauthorized"
    """Authentication is missing or invalid. HTTP 401; gRPC UNAUTHENTICATED."""
    FORBIDDEN = "forbidden"
    """The caller is authenticated but not allowed to use the API. HTTP 403; gRPC PERMISSION_DENIED."""
    NOT_FOUND = "not_found"
    """Unknown route. HTTP 404; gRPC NOT_FOUND."""
    METHOD_NOT_ALLOWED = "method_not_allowed"
    """The HTTP method is not allowed for the route. HTTP 405; gRPC UNIMPLEMENTED."""
    REQUEST_TOO_LARGE = "request_too_large"
    """The request body exceeds the size limit. HTTP 413; gRPC RESOURCE_EXHAUSTED."""
    UPSTREAM_UNREACHABLE = "upstream_unreachable"
    """The SAP system could not be reached, or failed in a way the service cannot pass on, before the first record.
    HTTP 502; gRPC UNAVAILABLE.
    """
    INTERNAL = "internal"
    """Unexpected failure of the service before the first record. HTTP 500; gRPC INTERNAL."""
    CURSOR_INVALID = "cursor_invalid"
    """The cursor is malformed or its signature is bad. HTTP 400; gRPC INVALID_ARGUMENT."""
    CURSOR_MISMATCH = "cursor_mismatch"
    """The cursor does not match this request. HTTP 400; gRPC INVALID_ARGUMENT."""
    CURSOR_EXPIRED = "cursor_expired"
    """The cursor has expired. HTTP 400; gRPC INVALID_ARGUMENT."""
    CURSOR_STALE = "cursor_stale"
    """The profile assignments changed since the cursor was issued; restart the query. HTTP 409; gRPC ABORTED."""
    UPSTREAM_TIMEOUT = "upstream_timeout"
    """The SAP system did not answer in time before the first record. HTTP 504; gRPC DEADLINE_EXCEEDED."""


class RecordErrorCode(DocumentedStrEnum):
    """Per-selector / per-assignment error codes (spec §4.3), carried by error records and results."""

    SELECTOR_NOT_FOUND = "selector_not_found"
    """The selector matches no business object (e.g. an unknown market location)."""
    NO_ASSIGNMENT = "no_assignment"
    """The selector exists but has no profile assignment in the request window."""
    UNKNOWN_SELECTOR = "unknown_selector"
    """The selector kind (by) is not known to this installation."""
    BAD_SELECTOR = "bad_selector"
    """The selector's parameters are not acceptable to its resolver."""
    ACCESS_DENIED = "access_denied"
    """The caller is not allowed to read this profile."""
    PROFILE_NOT_FOUND = "profile_not_found"
    """The profile does not exist."""
    PROFILE_TIME_ZONE_MISSING = "profile_time_zone_missing"
    """The profile has no time zone, so its values cannot be read."""
    INTERVAL_SIZE_UNSUPPORTED = "interval_size_unsupported"
    """The profile's interval size is not supported."""
    WINDOW_UNALIGNED = "window_unaligned"
    """The request window is not aligned to the profile's intervals."""
    ASSIGNMENT_UNALIGNED = "assignment_unaligned"
    """The bounds of the profile assignment are not aligned to the profile's intervals."""
    VERSION_NOT_FOUND = "version_not_found"
    """The requested version number does not exist for the profile."""
    VERSION_DELETED = "version_deleted"
    """The requested {"number": N} names a version that is flagged as deleted (it is still listed by the versions
    endpoint, with deleted: true).
    """
    CONVERSION_NOT_ALLOWED = "conversion_not_allowed"
    """The requested unit or interval conversion is not allowed for the profile."""
    AMBIGUOUS_LOCAL_TIME = "ambiguous_local_time"
    """A local time in the window occurs twice (daylight saving change) and cannot be mapped to one instant."""
    BADI_FAILED = "badi_failed"
    """A customer BAdI (resolver or metadata) failed."""


def _known(enum: type[StrEnum]) -> Any:
    def convert(value: Any) -> Any:
        if isinstance(value, str) and not isinstance(value, enum) and value in enum._value2member_map_:
            return enum(value)
        return value

    return BeforeValidator(convert)


def _public_only(enum: type[StrEnum]) -> Any:
    """JSON serializer that writes only the public codes (as Go's ``MarshalJSON``).

    Python-mode ``model_dump()`` keeps unknown codes, so a parsed response can still be turned into a dict.
    """

    def serialize(value: StrEnum | str) -> str:
        if not isinstance(value, enum):
            raise ValueError(f"{value!r} is not a public error code; only the known codes can be written")
        return value.value

    return PlainSerializer(serialize, return_type=str, when_used="json")


RequestCode = Annotated[RequestErrorCode | CodeStr, _known(RequestErrorCode), _public_only(RequestErrorCode)]
"""A request-level code: strict on write, lenient on read. Known codes become :class:`RequestErrorCode` members;
unknown snake_case codes are kept as ``str`` so that a newer server does not break older clients, but they
cannot be written (serializing raises).
"""

RecordCode = Annotated[RecordErrorCode | CodeStr, _known(RecordErrorCode), _public_only(RecordErrorCode)]
"""A record-level code; see :data:`RequestCode`."""


class ErrorDetail(ResponseModel):
    """The content of an :class:`ErrorEnvelope`."""

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"request_id"})

    code: RequestCode = Field(description="The request-level error code (snake_case); see RequestErrorCode.")
    message: Text = Field(description="Human-readable message; not meant for programmatic use.")
    request_id: Text | None = Field(
        default=None, min_length=1, description="Correlation id of the failed request, when the service has one."
    )


class ErrorEnvelope(ResponseModel):
    """Body of every failed HTTP response: ``{"error":{"code","message","request_id"?}}``."""

    error: ErrorDetail = Field(description="The error code, message and optional request id.")


class ResultError(ResponseModel):
    """The error object of a resolve/versions result or assignment: ``{"code": "…"}``."""

    code: RecordCode = Field(description="The record-level error code (snake_case); see RecordErrorCode.")
