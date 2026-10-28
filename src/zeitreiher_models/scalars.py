"""Scalar types with the contract's syntax rules."""

import itertools
import re
import unicodedata
from decimal import Decimal
from typing import Annotated, Any

from pydantic import AfterValidator, BeforeValidator, Field, StrictInt, StrictStr, StringConstraints

INT64_MAX = 2**63 - 1
"""Upper bound of every integer, so that Go (int64) and Python agree."""

_DECIMAL = re.compile(r"^-?(0|[1-9][0-9]*)(\.[0-9]+)?$")


def _check_text(value: Any) -> Any:
    """Reject lone surrogates: they cannot be written as UTF-8 (Go rejects invalid UTF-8 likewise).

    It runs before the string checks, which would otherwise report a less specific error.
    """
    if isinstance(value, str) and not value.isascii():
        try:
            value.encode("utf-8")
        except UnicodeEncodeError:
            raise ValueError(f"{value!r} is not valid UTF-8 (it contains a lone surrogate)") from None
    return value


Text = Annotated[StrictStr, BeforeValidator(_check_text)]
"""A string that can be written as UTF-8 unchanged (no lone surrogates). Every free-text field uses it."""

CHECK_TEXT = BeforeValidator(_check_text)
"""The validator of :data:`Text`, for types that add constraints (``Annotated[StrictStr, …, CHECK_TEXT]``)."""


def _check_decimal(value: str) -> str:
    if not _DECIMAL.fullmatch(value):
        raise ValueError(f"{value!r} is not a decimal string")
    if value.startswith("-") and not value.strip("-0."):
        raise ValueError(f"{value!r}: negative zero is not allowed")
    if "." in value and value.endswith("0"):
        shortest = value.rstrip("0").removesuffix(".")
        raise ValueError(
            f"{value!r} is not in shortest form (no trailing zeros after the decimal point): write {shortest!r}"
        )
    return value


def _check_request_profile(value: str) -> str:
    if not value.strip("0"):
        raise ValueError(f"{value!r}: profile number 0 does not exist")
    return value


def _check_status(value: tuple[str, ...]) -> tuple[str, ...]:
    for previous, current in itertools.pairwise(value):
        if previous >= current:
            raise ValueError(f"status codes must be sorted and unique, got {previous!r} before {current!r}")
    return value


DecimalString = Annotated[StrictStr, AfterValidator(_check_decimal)]
"""An already-scaled decimal number in its shortest canonical string form, e.g. ``"160"``, ``"0.022"``, ``"25.25"``
or ``"-3.5"``. JSON numbers, exponents, a plus sign, leading zeros, negative zero and trailing zeros after the
decimal point (``"160.0"``) are rejected: after parsing, 160.0 and 160 are the same number, so the wire carries
one spelling.
"""

RequestProfile = Annotated[
    StrictStr, StringConstraints(pattern=r"^[0-9]{1,18}$"), AfterValidator(_check_request_profile)
]
"""A profile number in a request: 1 to 18 digits, leading zeros allowed, not all zeros."""

ResponseProfile = Annotated[StrictStr, StringConstraints(pattern=r"^[1-9][0-9]{0,17}$")]
"""A profile number in a response: without leading zeros."""

MaLo = Annotated[StrictStr, StringConstraints(pattern=r"^[0-9]{11}$")]
"""A market location id: 11 digits."""

_INTERVAL = re.compile(r"^PT[1-9][0-9]*M$")


def _check_interval(value: str) -> str:
    if not _INTERVAL.fullmatch(value):
        raise ValueError(
            f"{value!r} is not a sub-daily interval of the form PT<minutes>M, e.g. PT15M or PT60M "
            "(hours are written in minutes: PT60M, not PT1H)"
        )
    return value


Interval = Annotated[StrictStr, AfterValidator(_check_interval)]
"""A sub-daily interval as an ISO 8601 duration in minutes, e.g. ``"PT15M"``. Hours are written in minutes
(``"PT60M"``); ``"PT1H"`` is rejected so that every interval has exactly one spelling.
"""

MAX_RAW_UNIT_LENGTH = 30
"""The longest ``unit_raw`` (SAP unit key of a series record), in characters (code points)."""

_UNIT_CODE = re.compile(r"^[A-Z0-9]{2,3}$")


def _check_unit_code(value: str) -> str:
    """A UN/ECE Rec. 20 code: 2 or 3 upper-case ASCII letters or digits (as in Go)."""
    if not _UNIT_CODE.fullmatch(value):
        raise ValueError(
            f"{value!r} is not a unit code (UN/ECE Rec. 20: 2 or 3 upper-case letters or digits, e.g. KWH)"
        )
    return value


def _check_raw_unit(value: str) -> str:
    """Any non-empty SAP unit of at most 30 characters without control characters (Unicode Cc), as in Go."""
    if not 1 <= len(value) <= MAX_RAW_UNIT_LENGTH or any(unicodedata.category(c) == "Cc" for c in value):
        raise ValueError(f"{value!r} is not a SAP unit (1 to {MAX_RAW_UNIT_LENGTH} characters, no control characters)")
    return value


UnitCode = Annotated[StrictStr, AfterValidator(_check_unit_code), CHECK_TEXT]
"""A canonical unit: a UN/ECE Recommendation 20 code such as ``"KWH"``, ``"MWH"`` or ``"K3"``
(``K3`` = kvarh; 2 or 3 upper-case ASCII letters or digits). Requests and series records use it;
whether a well-formed code is supported is the server's decision (an unknown code is a 400).
"""

RawUnit = Annotated[StrictStr, AfterValidator(_check_raw_unit), CHECK_TEXT]
"""The SAP unit key (T006 MSEHI) of a series record as SAP delivers it, mapped or not: any non-empty text of at
most 30 characters without control characters (read leniently, like unknown keys).
"""

Token = Annotated[StrictStr, StringConstraints(min_length=1), CHECK_TEXT]
"""A non-empty opaque token (cursor)."""

NonEmptyStr = Annotated[StrictStr, StringConstraints(min_length=1), CHECK_TEXT]
"""A non-empty string."""

Status = Annotated[tuple[NonEmptyStr, ...], AfterValidator(_check_status)]
"""Status codes, sorted and unique."""

NonNegativeInt = Annotated[StrictInt, Field(ge=0, le=INT64_MAX)]
"""An integer ≥ 0 (indices and counters)."""

PositiveInt = Annotated[StrictInt, Field(ge=1, le=INT64_MAX)]
"""An integer ≥ 1 (series and version numbers)."""

CodeStr = Annotated[StrictStr, StringConstraints(pattern=r"^[a-z][a-z0-9_]*$")]
"""A snake_case error code."""


def to_decimal(value: str) -> Decimal:
    """Convert a :data:`DecimalString` to :class:`decimal.Decimal` without loss."""
    return Decimal(value)
