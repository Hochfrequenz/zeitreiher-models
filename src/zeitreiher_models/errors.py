"""Error codes and the error shapes of the API."""

from enum import StrEnum
from typing import Annotated, Any, ClassVar

from pydantic import BeforeValidator, Field, PlainSerializer

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.scalars import CodeStr, Text


class RequestErrorCode(StrEnum):
    """Request-level error codes (spec §4.3), carried by :class:`ErrorEnvelope`.

    The first eight are btpingo's request-level vocabulary (``btpingo.ErrorCode`` in ``httperr.go``), shared
    by the Hochfrequenz BTP services; the cursor codes and ``upstream_timeout`` are Zeitreiher's own.
    """

    INVALID_REQUEST = "invalid_request"
    UNAUTHORIZED = "unauthorized"
    FORBIDDEN = "forbidden"
    NOT_FOUND = "not_found"
    METHOD_NOT_ALLOWED = "method_not_allowed"
    REQUEST_TOO_LARGE = "request_too_large"
    UPSTREAM_UNREACHABLE = "upstream_unreachable"
    INTERNAL = "internal"
    CURSOR_INVALID = "cursor_invalid"
    CURSOR_MISMATCH = "cursor_mismatch"
    CURSOR_EXPIRED = "cursor_expired"
    CURSOR_STALE = "cursor_stale"
    UPSTREAM_TIMEOUT = "upstream_timeout"


class RecordErrorCode(StrEnum):
    """Per-selector / per-assignment error codes (spec §4.3), carried by error records and results."""

    SELECTOR_NOT_FOUND = "selector_not_found"
    NO_ASSIGNMENT = "no_assignment"
    UNKNOWN_SELECTOR = "unknown_selector"
    BAD_SELECTOR = "bad_selector"
    ACCESS_DENIED = "access_denied"
    PROFILE_NOT_FOUND = "profile_not_found"
    PROFILE_TIME_ZONE_MISSING = "profile_time_zone_missing"
    INTERVAL_SIZE_UNSUPPORTED = "interval_size_unsupported"
    WINDOW_UNALIGNED = "window_unaligned"
    ASSIGNMENT_UNALIGNED = "assignment_unaligned"
    VERSION_NOT_FOUND = "version_not_found"
    #: The requested ``{"number": N}`` names a version flagged as deleted (listed with ``deleted: true``).
    VERSION_DELETED = "version_deleted"
    CONVERSION_NOT_ALLOWED = "conversion_not_allowed"
    AMBIGUOUS_LOCAL_TIME = "ambiguous_local_time"
    BADI_FAILED = "badi_failed"


def _known(enum: type[StrEnum]) -> Any:
    def convert(value: Any) -> Any:
        if isinstance(value, str) and not isinstance(value, enum) and value in enum._value2member_map_:
            return enum(value)
        return value

    return BeforeValidator(convert)


def _public_only(enum: type[StrEnum]) -> Any:
    """Serializer that writes only the public codes (as Go's ``MarshalJSON``)."""

    def serialize(value: StrEnum | str) -> str:
        if not isinstance(value, enum):
            raise ValueError(f"{value!r} is not a public error code; only the known codes can be written")
        return value.value

    return PlainSerializer(serialize, return_type=str)


#: A request-level code: strict on write, lenient on read. Known codes become :class:`RequestErrorCode`
#: members; unknown snake_case codes are kept as ``str`` so that a newer server does not break older
#: clients, but they cannot be written (serializing raises).
RequestCode = Annotated[RequestErrorCode | CodeStr, _known(RequestErrorCode), _public_only(RequestErrorCode)]

#: A record-level code; see :data:`RequestCode`.
RecordCode = Annotated[RecordErrorCode | CodeStr, _known(RecordErrorCode), _public_only(RecordErrorCode)]


class ErrorDetail(ResponseModel):
    """The content of an :class:`ErrorEnvelope`."""

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"request_id"})

    code: RequestCode
    message: Text
    request_id: Text | None = Field(default=None, min_length=1)


class ErrorEnvelope(ResponseModel):
    """Body of every failed HTTP response: ``{"error":{"code","message","request_id"?}}``."""

    error: ErrorDetail


class ResultError(ResponseModel):
    """The error object of a resolve/versions result or assignment: ``{"code": "…"}``."""

    code: RecordCode
