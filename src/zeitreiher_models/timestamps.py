"""Instants (RFC 3339 timestamps with an explicit offset)."""

import re
from datetime import UTC, datetime, timedelta
from typing import Annotated, Any

from pydantic import AfterValidator, BeforeValidator, PlainSerializer
from pydantic_core import PydanticCustomError

INSTANT_PATTERN = re.compile(
    r"^[0-9]{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])"
    r"T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]"
    r"(Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$"
)
"""The only accepted spelling of an instant: upper-case ``T``, whole seconds, explicit offset. The Go models use
the same expression.
"""

FRACTIONAL_PATTERN = re.compile(
    r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]+(Z|[+-][0-9]{2}:[0-9]{2})$"
)
"""An otherwise well-formed instant with fractional seconds, rejected with a message that names the rule. The Go
models use the same expression.
"""

MIN_YEAR = "1900"
"""The earliest year an instant may be written with (as four digits); the Go models use the same."""

MIN_UTC_YEAR = 1900
"""The earliest year the instant itself (in UTC) may lie in, so that every request instant can also be written
as a response instant (``Z``). The Go models use the same.
"""

MAX_UTC_YEAR = 9999
"""The latest year the instant itself (in UTC) may lie in; see :data:`MIN_UTC_YEAR`."""


def _error(message: str, value: object) -> PydanticCustomError:
    return PydanticCustomError("instant", "{value}: " + message, {"value": repr(value)})


def _parse(value: Any, *, utc_only: bool) -> datetime:
    if isinstance(value, datetime):
        return value
    if not isinstance(value, str):
        raise _error("must be an RFC 3339 timestamp string", value)
    if FRACTIONAL_PATTERN.fullmatch(value):
        raise _error("fractional seconds are not allowed (whole seconds only)", value)
    if not INSTANT_PATTERN.fullmatch(value):
        raise _error("is not an RFC 3339 timestamp with whole seconds and an explicit offset", value)
    if value[:4] < MIN_YEAR:
        raise _error(f"years before {MIN_YEAR} are not allowed", value)
    if value.endswith("-00:00"):
        raise _error("the offset -00:00 means 'unknown offset' and is not allowed", value)
    if utc_only and not value.endswith("Z"):
        raise _error("response timestamps must be in UTC (Z)", value)
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise _error(f"is not a valid timestamp ({exc})", value) from exc
    if not MIN_UTC_YEAR <= _utc_year(parsed) <= MAX_UTC_YEAR:
        raise _error(f"the instant is outside the years {MIN_UTC_YEAR} to {MAX_UTC_YEAR} in UTC", value)
    return parsed


def _utc_year(value: datetime) -> int:
    """The year of ``value`` in UTC; ``datetime`` cannot hold years after 9999, so those give 10000."""
    try:
        return value.astimezone(UTC).year
    except OverflowError:
        return MAX_UTC_YEAR + 1


def _check(value: datetime) -> datetime:
    if value.tzinfo is None or value.utcoffset() is None:
        raise _error("has no offset (naive datetimes are not allowed)", value)
    if value.microsecond:
        raise _error("only whole seconds are allowed", value)
    return value


def _to_utc(value: datetime) -> datetime:
    try:
        return value.astimezone(UTC)
    except OverflowError as exc:  # the instant lies before year 1 or after year 9999 in UTC
        raise _error(f"the instant is outside the years {MIN_UTC_YEAR} to {MAX_UTC_YEAR} in UTC", value) from exc


def _format(value: datetime) -> str:
    offset = value.utcoffset()
    text = value.isoformat()
    if offset == timedelta(0):
        text = text[: -len("+00:00")] + "Z"
    return text


def _round_trips(value: datetime, *, utc_only: bool) -> datetime:
    """Check the normalized value: its wire form must parse back to the same instant.

    ``datetime`` objects skip the string checks of :func:`_parse`; an offset with seconds would be
    written as ``+00:00:30``, and converting a fractional-second offset to UTC introduces microseconds.
    """
    _check(value)
    offset = value.utcoffset()
    if offset is not None and offset % timedelta(minutes=1):
        # RFC 3339 offsets have whole minutes (Go reports the same).
        raise _error("the offset cannot be written without changing the instant", value)
    if _parse(_format(value), utc_only=utc_only) != value:
        raise _error("does not survive serialization unchanged", value)
    return value


RequestInstant = Annotated[
    datetime,
    BeforeValidator(lambda v: _parse(v, utc_only=False)),
    AfterValidator(_check),
    AfterValidator(lambda v: _round_trips(v, utc_only=False)),
    PlainSerializer(_format, return_type=str),
]
"""A request instant: any explicit offset, kept as given (``+00:00`` is written as ``Z``)."""

ResponseInstant = Annotated[
    datetime,
    BeforeValidator(lambda v: _parse(v, utc_only=True)),
    AfterValidator(_check),
    AfterValidator(_to_utc),
    AfterValidator(lambda v: _round_trips(v, utc_only=True)),
    PlainSerializer(_format, return_type=str),
]
"""A response instant: JSON input must use ``Z``; ``datetime`` objects are converted to UTC."""


def check_window(start: datetime, end: datetime) -> None:
    """Raise if ``[start, end)`` is empty or reversed."""
    if not start < end:
        raise ValueError(f"from ({_format(start)}) must be before to ({_format(end)})")
