"""NDJSON records of a query response (spec §4.2)."""

from collections.abc import Mapping
from decimal import Decimal
from typing import Annotated, ClassVar, Literal, Self

from pydantic import AfterValidator, Field, StrictBool, model_validator

from zeitreiher_models._base import ResponseModel, freeze_sorted
from zeitreiher_models.errors import RecordCode
from zeitreiher_models.scalars import (
    DecimalString,
    Interval,
    NonEmptyStr,
    NonNegativeInt,
    PositiveInt,
    RawUnit,
    ResponseProfile,
    Status,
    Text,
    Token,
    UnitCode,
    to_decimal,
)
from zeitreiher_models.selectors import EchoedSelector
from zeitreiher_models.timestamps import ResponseInstant, check_window


class Version(ResponseModel):
    """An EDM profile version overlapping a series (spec §4.2, §5)."""

    number: PositiveInt = Field(description="The version number (at least 1).")
    from_: ResponseInstant = Field(
        alias="from", description="Start of the version's range (inclusive), RFC 3339 UTC with Z."
    )
    to: ResponseInstant | None = Field(
        description="End of the version's range (exclusive), RFC 3339 UTC with Z; null for an open end."
    )
    created_at: ResponseInstant | None = Field(
        description="When the version was created (RFC 3339 UTC); null when unknown, then created_at_unknown is true."
    )
    created_at_unknown: StrictBool = Field(description="True exactly when created_at is null.")
    reason: Text = Field(description="The SAP version reason code, as SAP delivers it (may be empty).")
    description: Text = Field(description="The SAP version description (free text, may be empty).")
    deleted: StrictBool = Field(
        description="True for versions flagged as deleted in SAP. They are still listed; querying their values "
        "gives an error record with code version_deleted."
    )

    @model_validator(mode="after")
    def _consistent(self) -> Self:
        if self.to is not None:
            check_window(self.from_, self.to)
        if self.created_at_unknown != (self.created_at is None):
            raise ValueError("created_at_unknown must be true exactly when created_at is null")
        return self


class SeriesRecord(ResponseModel):
    """Opens a series: one profile assignment of one selector, cut to the request window."""

    type: Literal["series"] = "series"
    series: PositiveInt = Field(description="Number of this series within the response; value records refer to it.")
    selector: NonNegativeInt = Field(description="Index of the request selector this series belongs to (0-based).")
    select: EchoedSelector = Field(description="The request selector, echoed (profile numbers without leading zeros).")
    profile: ResponseProfile = Field(description="The profile number without leading zeros (1 to 18 digits).")
    role: NonEmptyStr | None = Field(
        description="The role of the profile assignment (SAP EPROFASS role, e.g. '0001'), or null when the series "
        "was not resolved through a role (e.g. by: profile). The Resolver BAdI may override it."
    )
    from_: ResponseInstant = Field(
        alias="from", description="Start of the series (inclusive), cut to the request window; RFC 3339 UTC with Z."
    )
    to: ResponseInstant = Field(
        description="End of the series (exclusive), cut to the request window; RFC 3339 UTC with Z."
    )
    unit: UnitCode | None = Field(
        description="The canonical unit code (UN/ECE Rec. 20, e.g. 'KWH') of the delivered values (after any "
        "conversion), or null when the SAP unit has no mapping. The series is delivered either way."
    )
    unit_raw: RawUnit = Field(
        description="The SAP unit key (T006 MSEHI) as SAP delivers it, mapped or not: 1 to 30 characters, "
        "no control characters."
    )
    interval: Interval = Field(description="Interval of the delivered values as ISO 8601 minutes, e.g. 'PT15M'.")
    attributes: Annotated[Mapping[Text, Text], AfterValidator(freeze_sorted)] = Field(
        description="Metadata from the customer's Metadata BAdI; always written, possibly {}."
    )
    versions: tuple[Version, ...] = Field(description="Versions overlapping the series; always written, possibly [].")

    @model_validator(mode="after")
    def _window(self) -> Self:
        check_window(self.from_, self.to)
        return self


class ValueRecord(ResponseModel):
    """One value of a series for ``[from, to)``."""

    type: Literal["value"] = "value"
    series: PositiveInt = Field(description="Number of the series record this value belongs to.")
    from_: ResponseInstant = Field(
        alias="from", description="Start of the value's interval (inclusive); RFC 3339 UTC with Z."
    )
    to: ResponseInstant = Field(description="End of the value's interval (exclusive); RFC 3339 UTC with Z.")
    value: DecimalString = Field(
        description="The already-scaled decimal value as a string in its shortest canonical form "
        "('160', '0.022', '-3.5'); use the decimal property for arithmetic."
    )
    status: Status = Field(description="Active SAP status codes overlapping the interval, sorted and unique.")
    version: PositiveInt | None = Field(
        description="The source of the value: the number of the stored version row it came from, or null when it "
        "is a current value. With {'number': N} a series mixes both: current values overlaid with the stored "
        "values of versions >= N (the lowest wins), only inside N's covered range."
    )

    @model_validator(mode="after")
    def _window(self) -> Self:
        check_window(self.from_, self.to)
        return self

    @property
    def decimal(self) -> Decimal:
        """The value as :class:`decimal.Decimal`."""
        return to_decimal(self.value)


class ErrorRecord(ResponseModel):
    """A per-selector or per-assignment problem; the stream continues."""

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"profile"})

    type: Literal["error"] = "error"
    selector: NonNegativeInt = Field(description="Index of the request selector the problem belongs to (0-based).")
    select: EchoedSelector = Field(description="The request selector, echoed.")
    profile: ResponseProfile | None = Field(
        default=None, description="The profile number the problem belongs to; omitted for a selector-level problem."
    )
    code: RecordCode = Field(description="The record-level error code (snake_case); see RecordErrorCode.")


class EndRecord(ResponseModel):
    """Closes a successful response. A response without it is incomplete and must be treated as failed.

    ``series``, ``values`` and ``errors`` count the series, value and error records on this page only (not
    the whole result); a series record re-sent at the start of a continuation page counts again.
    """

    type: Literal["end"] = "end"
    series: NonNegativeInt = Field(description="Number of series records on this page.")
    values: NonNegativeInt = Field(description="Number of value records on this page.")
    errors: NonNegativeInt = Field(description="Number of error records on this page.")
    next: Token | None = Field(description="The cursor for the next page, or null when the result is complete.")


#: One NDJSON line: a union over ``type``.
Record = Annotated[SeriesRecord | ValueRecord | ErrorRecord | EndRecord, Field(discriminator="type")]
