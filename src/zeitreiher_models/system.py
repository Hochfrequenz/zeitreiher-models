"""Body of ``GET /api/v1/system`` (spec §4.4)."""

from pydantic import Field

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.scalars import Interval, NonEmptyStr, PositiveInt, UnitCode


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


class SystemInfo(ResponseModel):
    """Versions, SAP system identity, business time zone, supported intervals and units, and caps."""

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
