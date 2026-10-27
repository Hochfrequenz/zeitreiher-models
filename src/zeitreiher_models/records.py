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

    number: PositiveInt
    from_: ResponseInstant = Field(alias="from")
    #: ``None`` (``null``) for an open end.
    to: ResponseInstant | None
    #: ``None`` when the creation instant cannot be determined; then ``created_at_unknown`` is true.
    created_at: ResponseInstant | None
    created_at_unknown: StrictBool
    reason: Text
    description: Text
    #: True for versions flagged as deleted in SAP. They are still listed; querying their values gives an
    #: error record with code ``version_deleted``.
    deleted: StrictBool

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
    series: PositiveInt
    selector: NonNegativeInt
    select: EchoedSelector
    profile: ResponseProfile
    #: The role of the profile assignment (SAP EPROFASS role, e.g. ``"0001"``), or ``None`` when the series
    #: was not resolved through a role (e.g. ``by: profile``). The Resolver BAdI may override it.
    role: NonEmptyStr | None
    from_: ResponseInstant = Field(alias="from")
    to: ResponseInstant
    #: The canonical unit code (UN/ECE Rec. 20, e.g. ``"KWH"``) of the delivered values (after any conversion),
    #: or ``None`` when the SAP unit has no mapping. The series is delivered either way.
    unit: UnitCode | None
    #: The SAP unit key (T006 MSEHI) as SAP delivers it, mapped or not.
    unit_raw: RawUnit
    interval: Interval
    attributes: Annotated[Mapping[Text, Text], AfterValidator(freeze_sorted)]
    versions: tuple[Version, ...]

    @model_validator(mode="after")
    def _window(self) -> Self:
        check_window(self.from_, self.to)
        return self


class ValueRecord(ResponseModel):
    """One value of a series for ``[from, to)``."""

    type: Literal["value"] = "value"
    series: PositiveInt
    from_: ResponseInstant = Field(alias="from")
    to: ResponseInstant
    #: Already scaled; use :attr:`decimal` for arithmetic.
    value: DecimalString
    status: Status
    #: The source of the value: the number of the stored version row it came from, or ``None`` when it is
    #: a current value. With ``{"number": N}`` a series mixes both: current values overlaid with the stored
    #: values of versions >= N (the lowest wins), only inside N's covered range.
    version: PositiveInt | None

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
    selector: NonNegativeInt
    select: EchoedSelector
    profile: ResponseProfile | None = None
    code: RecordCode


class EndRecord(ResponseModel):
    """Closes a successful response. A response without it is incomplete and must be treated as failed.

    ``series``, ``values`` and ``errors`` count the series, value and error records on this page only (not
    the whole result); a series record re-sent at the start of a continuation page counts again.
    """

    type: Literal["end"] = "end"
    series: NonNegativeInt
    values: NonNegativeInt
    errors: NonNegativeInt
    #: The cursor for the next page, or ``None`` when the result is complete.
    next: Token | None


#: One NDJSON line: a union over ``type``.
Record = Annotated[SeriesRecord | ValueRecord | ErrorRecord | EndRecord, Field(discriminator="type")]
