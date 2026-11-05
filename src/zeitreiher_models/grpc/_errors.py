"""The public error code of a failed gRPC call (zeitreiher-go ``internal/rpc/errors.go``, client side)."""

import grpc
from google.protobuf.message import DecodeError
from google.rpc import error_details_pb2, status_pb2

#: The ``ErrorInfo`` domain of every error the service raises.
ERROR_DOMAIN = "zeitreiher.hochfrequenz.de"

_DETAILS_KEY = "grpc-status-details-bin"


def error_reason(err: grpc.RpcError) -> str | None:
    """The public request-level error code of ``err`` (``ErrorInfo.reason``), or ``None``.

    The service attaches a ``google.rpc.ErrorInfo`` (``domain`` = :data:`ERROR_DOMAIN`, ``reason`` = the public
    code, e.g. ``cursor_stale``) to every request-level failure, in the ``grpc-status-details-bin`` trailer.
    Failures without one exist: a client that cancelled the call or whose own deadline ran out, transport
    errors, and a gateway in between. ``None`` for those and for a trailer that cannot be parsed; the reason of
    a newer server is returned as is, even if it is not in :class:`~zeitreiher_models.RequestErrorCode`.
    """
    trailing = getattr(err, "trailing_metadata", None)
    if not callable(trailing):
        return None
    try:
        metadata = trailing()
        raw = next((v for k, v in metadata or () if k == _DETAILS_KEY), None)
    except Exception:
        return None
    if raw is None:
        return None
    try:
        status = status_pb2.Status.FromString(raw if isinstance(raw, bytes) else str(raw).encode("latin-1"))
        for detail in status.details:
            if not detail.Is(error_details_pb2.ErrorInfo.DESCRIPTOR):
                continue
            info = error_details_pb2.ErrorInfo()
            detail.Unpack(info)
            if info.domain == ERROR_DOMAIN:
                return str(info.reason)
    except (DecodeError, UnicodeEncodeError):
        return None
    return None
