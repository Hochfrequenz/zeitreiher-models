"""Body of ``GET /api/v1/system`` (spec §4.4)."""

from zeitreiher_models._base import ResponseModel
from zeitreiher_models.scalars import Interval, NonEmptyStr


class SystemInfo(ResponseModel):
    """Versions, SAP system identity, business time zone and supported intervals."""

    service_version: NonEmptyStr
    abap_package_version: NonEmptyStr
    sid: NonEmptyStr
    client: NonEmptyStr
    release: NonEmptyStr
    #: The configured SAP time zone code, e.g. ``"CET"``.
    business_time_zone: NonEmptyStr
    #: Supported request intervals, e.g. ``("PT15M", "PT60M")``.
    intervals: tuple[Interval, ...]
