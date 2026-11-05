"""``error_reason`` with hand-built ``grpc.RpcError`` objects (no server)."""

from typing import Any

import pytest

pytest.importorskip("grpc")

import grpc
from google.protobuf import any_pb2
from google.rpc import code_pb2, error_details_pb2, status_pb2

from zeitreiher_models.grpc import ERROR_DOMAIN, error_reason


class FakeCall(grpc.RpcError, grpc.Call):  # type: ignore[misc]
    """A failed call that only knows its trailing metadata."""

    def __init__(self, trailing: Any) -> None:
        super().__init__()
        self._trailing = trailing

    def add_callback(self, callback: Any) -> bool:
        return False

    def cancel(self) -> bool:
        return False

    def initial_metadata(self) -> Any:
        return ()

    def is_active(self) -> bool:
        return False

    def time_remaining(self) -> float:
        return 0.0

    def trailing_metadata(self) -> Any:
        if isinstance(self._trailing, Exception):
            raise self._trailing
        return self._trailing


def _status(*details: Any) -> bytes:
    status = status_pb2.Status(code=code_pb2.ABORTED, message="x")
    for detail in details:
        packed = any_pb2.Any()
        packed.Pack(detail)
        status.details.append(packed)
    return bytes(status.SerializeToString())


def _info(reason: str, domain: str = ERROR_DOMAIN) -> error_details_pb2.ErrorInfo:
    return error_details_pb2.ErrorInfo(reason=reason, domain=domain)


def _err(raw: bytes | str | None) -> FakeCall:
    metadata: list[tuple[str, Any]] = [("content-type", "application/grpc")]
    if raw is not None:
        metadata.append(("grpc-status-details-bin", raw))
    return FakeCall(tuple(metadata))


def test_reason_of_the_service_domain() -> None:
    assert error_reason(_err(_status(_info("cursor_stale")))) == "cursor_stale"


def test_unknown_reason_of_a_newer_server_is_passed_on() -> None:
    assert error_reason(_err(_status(_info("something_new")))) == "something_new"


def test_other_details_and_domains_are_skipped() -> None:
    other = error_details_pb2.RetryInfo()
    raw = _status(other, _info("foreign", "example.com"), _info("internal"))
    assert error_reason(_err(raw)) == "internal"


def test_foreign_domain_only() -> None:
    assert error_reason(_err(_status(_info("foreign", "example.com")))) is None


@pytest.mark.parametrize(
    "raw", [None, b"", _status(), b"\xff\xff\xff not a status"], ids=["absent", "empty", "no-details", "garbage"]
)
def test_none_without_error_info(raw: bytes | None) -> None:
    assert error_reason(_err(raw)) is None


def test_no_metadata() -> None:
    assert error_reason(FakeCall(None)) is None
    assert error_reason(FakeCall(())) is None


def test_broken_call_object() -> None:
    assert error_reason(FakeCall(RuntimeError("closed"))) is None
    assert error_reason(grpc.RpcError()) is None
