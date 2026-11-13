"""Body of ``GET /api/v1/system`` (spec §4.4)."""

from typing import Annotated, ClassVar, Self

from pydantic import Field, StrictBool, StringConstraints, model_validator
from pydantic.types import StrictStr

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.scalars import Interval, NonEmptyStr, NonNegativeInt, PositiveInt, UnitCode
from zeitreiher_models.timestamps import ResponseInstant

SourceFingerprint = Annotated[StrictStr, StringConstraints(pattern=r"^[0-9a-f]{64}$")]
"""An aggregate SHA-256: exactly 64 lower-case hex characters. The Go models use the same expression."""


class Caps(ResponseModel):
    """The limits of the service (spec section 3.1), reported so that clients know how it works.

    ``max_window_seconds`` and ``max_values`` bound one internal SAP values call; the service splits a query window
    accordingly.
    ``max_resolve_window_seconds`` is the longest request window the service accepts for query, resolve and
    versions; longer windows are rejected with ``400 invalid_request``.
    Versions and resolve are not split and have no value cap.
    The internal ``window_too_large`` answer of SAP never reaches public clients.
    """

    max_window_seconds: PositiveInt = Field(
        description="Longest window in seconds of one internal SAP values call; the service splits longer query "
        "windows (at least 1)."
    )
    max_values: PositiveInt = Field(
        description="Highest number of values of one internal SAP values call (at least 1)."
    )
    max_resolve_window_seconds: PositiveInt = Field(
        description="Longest request window in seconds the service accepts for query, resolve and versions; a "
        "longer window is a 400 invalid_request (at least 1)."
    )


class InactiveObjects(ResponseModel):
    """The inactive ABAP objects of the package, or why that is unknown.

    ``available: true`` carries ``count`` (equal to the number of ``names``) and ``names``;
    ``available: false`` carries a non-empty ``reason`` and neither ``count`` nor ``names``.
    """

    _omit_if_none: ClassVar[frozenset[str]] = frozenset({"count", "names", "reason"})

    available: StrictBool = Field(description="Whether the ABAP backend could determine the inactive objects.")
    count: NonNegativeInt | None = Field(
        default=None, description="Number of inactive objects; set exactly when available, equals len(names)."
    )
    names: tuple[NonEmptyStr, ...] | None = Field(
        default=None, description="Names of the inactive objects; set exactly when available."
    )
    reason: NonEmptyStr | None = Field(
        default=None, description="Why the inactive objects are unknown; set exactly when not available."
    )

    @model_validator(mode="after")
    def _consistent(self) -> Self:
        if self.available:
            if self.reason is not None:
                raise ValueError("reason must be unset when available")
            if self.count is None:
                raise ValueError("count is required when available")
            if self.names is None:
                raise ValueError("names is required when available")
            if self.count != len(self.names):
                raise ValueError(f"count {self.count} does not equal the number of names ({len(self.names)})")
        else:
            if self.count is not None:
                raise ValueError("count must be unset when not available")
            if self.names is not None:
                raise ValueError("names must be unset when not available")
            if self.reason is None:
                raise ValueError("reason is required when not available")
        return self


class SystemInfo(ResponseModel):
    """Versions, SAP system identity, business time zone, supported intervals and units, and caps.

    The three ``abap_*`` members pass the version state of the ABAP backend through; all are optional (older
    services and backends omit them), and an unset member is omitted when writing.
    """

    _omit_if_none: ClassVar[frozenset[str]] = frozenset(
        {"abap_last_source_change_utc", "abap_inactive_objects", "abap_source_fingerprint"}
    )

    service_version: NonEmptyStr = Field(description="Version of the Zeitreiher service (Go).")
    abap_package_version: NonEmptyStr = Field(description="Version of the ABAP package /HFQ/ZEITREIHER.")
    sid: NonEmptyStr = Field(description="SAP system id.")
    client: NonEmptyStr = Field(description="SAP client the service reads from.")
    release: NonEmptyStr = Field(description="SAP release.")
    business_time_zone: NonEmptyStr = Field(description="The configured SAP time zone code, e.g. 'CET'.")
    intervals: tuple[Interval, ...] = Field(
        description="Supported request intervals as ISO 8601 minutes, e.g. ['PT15M', 'PT60M']."
    )
    units: tuple[UnitCode, ...] = Field(
        description="Canonical unit codes (UN/ECE Rec. 20, e.g. 'KWH') the system can map to SAP units; "
        "a request unit outside this list is a 400."
    )
    caps: Caps = Field(description="Limits of one internal SAP call; see the Caps model.")
    abap_last_source_change_utc: ResponseInstant | None = Field(
        default=None,
        description="Instant of the last source change of the ABAP package (RFC 3339 UTC with Z, whole seconds); "
        "absent and null both mean unknown.",
    )
    abap_inactive_objects: InactiveObjects | None = Field(
        default=None, description="The inactive objects of the ABAP package, or why that is unknown."
    )
    abap_source_fingerprint: SourceFingerprint | None = Field(
        default=None,
        description="Aggregate SHA-256 of the ABAP sources: exactly 64 lower-case hex characters.",
    )
