"""Hand-written public data models of the Zeitreiher API, version 1.

The contract is the JSON form of these models, pinned by the shared fixtures in ``testdata/`` that
the Go and the Python models both round-trip. Nothing here is generated.
"""

from zeitreiher_models.errors import (
    ErrorDetail,
    ErrorEnvelope,
    RecordCode,
    RecordErrorCode,
    RequestCode,
    RequestErrorCode,
    ResultError,
)
from zeitreiher_models.ndjson import (
    RECORD_ADAPTER,
    MissingEndError,
    dump_record,
    iter_records,
    parse_record,
    read_stream,
    split_lines,
)
from zeitreiher_models.records import EndRecord, ErrorRecord, Record, SeriesRecord, ValueRecord, Version
from zeitreiher_models.requests import (
    CurrentVersion,
    QueryRequest,
    ResolveRequest,
    VersionNumber,
    VersionSelector,
    VersionsRequest,
)
from zeitreiher_models.results import (
    Assignment,
    ResolveResponse,
    ResolveResult,
    VersionedAssignment,
    VersionsResponse,
    VersionsResult,
)
from zeitreiher_models.scalars import DecimalString, to_decimal
from zeitreiher_models.selectors import (
    CustomSelector,
    EchoedCustomSelector,
    EchoedMaLoSelector,
    EchoedProfileSelector,
    EchoedSelector,
    MaLoSelector,
    ProfileSelector,
    Selector,
)
from zeitreiher_models.system import SystemInfo

__all__ = [
    "RECORD_ADAPTER",
    "Assignment",
    "CurrentVersion",
    "CustomSelector",
    "DecimalString",
    "EchoedCustomSelector",
    "EchoedMaLoSelector",
    "EchoedProfileSelector",
    "EchoedSelector",
    "EndRecord",
    "ErrorDetail",
    "ErrorEnvelope",
    "ErrorRecord",
    "MaLoSelector",
    "MissingEndError",
    "ProfileSelector",
    "QueryRequest",
    "Record",
    "RecordCode",
    "RecordErrorCode",
    "RequestCode",
    "RequestErrorCode",
    "ResolveRequest",
    "ResolveResponse",
    "ResolveResult",
    "ResultError",
    "Selector",
    "SeriesRecord",
    "SystemInfo",
    "ValueRecord",
    "Version",
    "VersionNumber",
    "VersionSelector",
    "VersionedAssignment",
    "VersionsRequest",
    "VersionsResponse",
    "VersionsResult",
    "dump_record",
    "iter_records",
    "parse_record",
    "read_stream",
    "split_lines",
    "to_decimal",
]
