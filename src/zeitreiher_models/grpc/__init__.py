"""gRPC support for the Zeitreiher API: generated stubs (``v1``), converters to the models and error helpers.

Needs the ``grpc`` extra: ``pip install "zeitreiher-models[grpc]"``. The base package never imports this one.
"""

try:
    import google.protobuf
    import google.rpc.error_details_pb2  # noqa: F401
    import grpc  # noqa: F401
except ImportError as exc:  # pragma: no cover - exercised by the packaging test
    raise ImportError(
        'zeitreiher_models.grpc needs the gRPC dependencies: pip install "zeitreiher-models[grpc]"'
    ) from exc

from zeitreiher_models.grpc._convert import (
    query_request_from_proto,
    query_request_to_proto,
    record_from_proto,
    record_to_proto,
    resolve_request_from_proto,
    resolve_request_to_proto,
    resolve_response_from_proto,
    resolve_response_to_proto,
    system_info_from_proto,
    system_info_to_proto,
    versions_request_from_proto,
    versions_request_to_proto,
    versions_response_from_proto,
    versions_response_to_proto,
)
from zeitreiher_models.grpc._errors import ERROR_DOMAIN, error_reason

__all__ = [
    "ERROR_DOMAIN",
    "error_reason",
    "query_request_from_proto",
    "query_request_to_proto",
    "record_from_proto",
    "record_to_proto",
    "resolve_request_from_proto",
    "resolve_request_to_proto",
    "resolve_response_from_proto",
    "resolve_response_to_proto",
    "system_info_from_proto",
    "system_info_to_proto",
    "versions_request_from_proto",
    "versions_request_to_proto",
    "versions_response_from_proto",
    "versions_response_to_proto",
]
