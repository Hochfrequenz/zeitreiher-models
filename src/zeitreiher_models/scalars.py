"""Scalar types with the contract's syntax rules."""

import itertools
import re
import unicodedata
from decimal import Decimal
from typing import Annotated, Any

from pydantic import AfterValidator, BeforeValidator, Field, StrictInt, StrictStr, StringConstraints

#: Upper bound of every integer, so that Go (int64) and Python agree.
INT64_MAX = 2**63 - 1

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


#: A string that can be written as UTF-8 unchanged (no lone surrogates). Every free-text field uses it.
Text = Annotated[StrictStr, BeforeValidator(_check_text)]

#: The validator of :data:`Text`, for types that add constraints (``Annotated[StrictStr, …, CHECK_TEXT]``).
CHECK_TEXT = BeforeValidator(_check_text)


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


#: An already-scaled decimal number in its shortest canonical string form, e.g. ``"160"``, ``"0.022"``,
#: ``"25.25"`` or ``"-3.5"``. JSON numbers, exponents, a plus sign, leading zeros, negative zero and
#: trailing zeros after the decimal point (``"160.0"``) are rejected: after parsing, 160.0 and 160 are the
#: same number, so the wire carries one spelling.
DecimalString = Annotated[StrictStr, AfterValidator(_check_decimal)]

#: A profile number in a request: 1 to 18 digits, leading zeros allowed, not all zeros.
RequestProfile = Annotated[
    StrictStr, StringConstraints(pattern=r"^[0-9]{1,18}$"), AfterValidator(_check_request_profile)
]

#: A profile number in a response: without leading zeros.
ResponseProfile = Annotated[StrictStr, StringConstraints(pattern=r"^[1-9][0-9]{0,17}$")]

#: A market location id: 11 digits.
MaLo = Annotated[StrictStr, StringConstraints(pattern=r"^[0-9]{11}$")]

#: A sub-daily interval as an ISO 8601 duration in minutes, e.g. ``"PT15M"``.
Interval = Annotated[StrictStr, StringConstraints(pattern=r"^PT[1-9][0-9]*M$")]

# Units are passed through as SAP delivers them; which T006 column (internal MSEHI such as "KWH" or an
# external form such as "kWh") the API carries is still open, so the rules are deliberately lenient.
#: The longest requested unit, in characters (code points).
MAX_REQUEST_UNIT_LENGTH = 6
#: The longest unit of a series record, in characters (code points).
MAX_RESPONSE_UNIT_LENGTH = 30
#: The non-alphanumeric characters a requested unit may contain.
UNIT_SYMBOLS = "%°/._-"


def _check_request_unit(value: str) -> str:
    """1 to 6 characters, each a Unicode letter (L*), number (N*) or one of ``%°/._-`` (as in Go)."""
    if not 1 <= len(value) <= MAX_REQUEST_UNIT_LENGTH or not all(
        unicodedata.category(c)[0] in "LN" or c in UNIT_SYMBOLS for c in value
    ):
        raise ValueError(f"{value!r} is not a unit (1 to {MAX_REQUEST_UNIT_LENGTH} letters, digits or {UNIT_SYMBOLS})")
    return value


def _check_response_unit(value: str) -> str:
    """Any non-empty unit of at most 30 characters without control characters (Unicode Cc), as in Go."""
    if not 1 <= len(value) <= MAX_RESPONSE_UNIT_LENGTH or any(unicodedata.category(c) == "Cc" for c in value):
        raise ValueError(f"{value!r} is not a unit (1 to {MAX_RESPONSE_UNIT_LENGTH} characters, no control characters)")
    return value


#: A requested unit such as ``"KWH"``, ``"kvarh"``, ``"µM"`` or ``"°C"``: 1 to 6 Unicode letters, digits or
#: ``%°/._-``; no whitespace, no control characters.
RequestUnit = Annotated[StrictStr, AfterValidator(_check_request_unit), CHECK_TEXT]

#: The unit of a series record, as SAP delivers it: any non-empty text of at most 30 characters without
#: control characters (read leniently, like unknown keys and error codes).
ResponseUnit = Annotated[StrictStr, AfterValidator(_check_response_unit), CHECK_TEXT]

#: A non-empty opaque token (cursor).
Token = Annotated[StrictStr, StringConstraints(min_length=1), CHECK_TEXT]

#: A non-empty string.
NonEmptyStr = Annotated[StrictStr, StringConstraints(min_length=1), CHECK_TEXT]

#: Status codes, sorted and unique.
Status = Annotated[tuple[NonEmptyStr, ...], AfterValidator(_check_status)]

#: An integer ≥ 0 (indices and counters).
NonNegativeInt = Annotated[StrictInt, Field(ge=0, le=INT64_MAX)]

#: An integer ≥ 1 (series and version numbers).
PositiveInt = Annotated[StrictInt, Field(ge=1, le=INT64_MAX)]

#: A snake_case error code.
CodeStr = Annotated[StrictStr, StringConstraints(pattern=r"^[a-z][a-z0-9_]*$")]


def to_decimal(value: str) -> Decimal:
    """Convert a :data:`DecimalString` to :class:`decimal.Decimal` without loss."""
    return Decimal(value)
