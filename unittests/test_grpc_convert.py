"""Guards of the proto converters (ported from zeitreiher-go's ``internal/rpc/convert_test.go``)."""

import pickle
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Any

import pytest

pytest.importorskip("grpc")

from google.protobuf.message import Message
from google.protobuf.timestamp_pb2 import Timestamp
from pydantic import ValidationError

from zeitreiher_models import EchoedMaLoSelector, EndRecord, ErrorRecord, ValueRecord
from zeitreiher_models.grpc import (
    query_request_from_proto,
    record_from_proto,
    record_to_proto,
)
from zeitreiher_models.grpc.v1 import query_pb2, results_pb2, selector_pb2, series_service_pb2, system_pb2

FROM = "2026-01-14T00:00:00Z"
TO = "2026-01-15T00:00:00Z"


def _valid() -> query_pb2.QueryRequest:
    out = query_pb2.QueryRequest(to=TO, select=[selector_pb2.Selector(by="profile", profile="50000000000056")])
    setattr(out, "from", FROM)
    return out


def _unset_version(r: query_pb2.QueryRequest) -> None:
    r.version.SetInParent()


def _empty_interval(r: query_pb2.QueryRequest) -> None:
    r.interval = ""


def _unknown_offset(r: query_pb2.QueryRequest) -> None:
    setattr(r, "from", "2026-01-14T00:00:00-00:00")


def _missing_offset(r: query_pb2.QueryRequest) -> None:
    r.to = "2026-01-15T00:00:00"


def _no_selector(r: query_pb2.QueryRequest) -> None:
    del r.select[:]


def _mixed_selector(r: query_pb2.QueryRequest) -> None:
    r.select[0].malo = "87253749908"


def _profile_left_out(r: query_pb2.QueryRequest) -> None:
    r.select[0].profile = ""


def _params_on_a_profile(r: query_pb2.QueryRequest) -> None:
    r.select[0].params["a"] = "b"


def _version_number_zero(r: query_pb2.QueryRequest) -> None:
    r.version.number = 0


def _profile_and_malo(r: query_pb2.QueryRequest) -> None:
    r.select[0].by = "profile"
    r.select[0].malo = "87253749908"


def _malo_with_profile(r: query_pb2.QueryRequest) -> None:
    r.select[0].by = "malo"
    r.select[0].malo = "87253749908"  # profile from _valid() stays set


def _params_on_malo(r: query_pb2.QueryRequest) -> None:
    r.select[0].by = "malo"
    r.select[0].profile = ""
    r.select[0].malo = "87253749908"
    r.select[0].params["a"] = "b"


@pytest.mark.parametrize(
    "mutate",
    [
        _unset_version,
        _empty_interval,
        _unknown_offset,
        _missing_offset,
        _no_selector,
        _mixed_selector,
        _profile_left_out,
        _params_on_a_profile,
        _version_number_zero,
        _profile_and_malo,
        _malo_with_profile,
        _params_on_malo,
    ],
)
def test_query_request_from_proto_refuses_like_json(mutate: Callable[[query_pb2.QueryRequest], None]) -> None:
    request = _valid()
    mutate(request)
    with pytest.raises(ValidationError):
        query_request_from_proto(request)


def test_query_request_from_proto_custom_selector_keeps_params() -> None:
    request = _valid()
    del request.select[:]
    request.select.add(by="contract_account").params["account"] = "000012345678"
    model = query_request_from_proto(request)
    selector: Any = model.select[0]
    assert selector.by == "contract_account"
    assert dict(selector.params) == {"account": "000012345678"}
    assert model.model_dump(by_alias=True, mode="json")["from"] == FROM


def test_query_request_from_proto_version_number_and_current() -> None:
    assert query_request_from_proto(_valid()).version is None

    numbered = _valid()
    numbered.version.number = 3
    version: Any = query_request_from_proto(numbered).version
    assert version.number == 3

    current = _valid()
    current.version.current.SetInParent()
    version = query_request_from_proto(current).version
    assert version is not None
    assert not hasattr(version, "number")


def test_record_to_proto_end_without_next_has_no_cursor() -> None:
    message = record_to_proto(EndRecord(series=1, values=2, errors=0, next=None))
    assert not message.end.HasField("next")
    message = record_to_proto(EndRecord(series=0, values=0, errors=0, next="abc"))
    assert message.end.HasField("next")
    assert message.end.next == "abc"


def test_record_to_proto_value_version_null_versus_number() -> None:
    when = datetime(2026, 1, 14, tzinfo=UTC)
    end = datetime(2026, 1, 14, 0, 15, tzinfo=UTC)
    current = record_to_proto(ValueRecord(series=1, from_=when, to=end, value="1", status=(), version=None))
    assert not current.value.HasField("version")
    stored = record_to_proto(ValueRecord(series=1, from_=when, to=end, value="1", status=(), version=3))
    assert stored.value.HasField("version")
    assert stored.value.version == 3


def test_record_to_proto_error_record_without_profile_leaves_it_unset() -> None:
    record = ErrorRecord(
        selector=1, select=EchoedMaLoSelector(malo="87253749908"), profile=None, code="selector_not_found"
    )
    message = record_to_proto(record)
    assert not message.error.HasField("profile")
    back = record_from_proto(message)
    assert isinstance(back, ErrorRecord)
    assert back.profile is None


def test_record_to_proto_refuses_an_unknown_record_type() -> None:
    with pytest.raises(TypeError):
        record_to_proto(None)  # type: ignore[arg-type]


def test_timestamp_with_nanos_is_refused() -> None:
    message = query_pb2.Record()
    value = message.value
    value.series = 1
    value.value = "1"
    getattr(value, "from").CopyFrom(Timestamp(seconds=1, nanos=5))
    value.to.CopyFrom(Timestamp(seconds=2))
    with pytest.raises(ValueError, match="whole seconds"):
        record_from_proto(message)


def test_record_without_a_oneof_is_refused() -> None:
    with pytest.raises(ValueError, match="oneof"):
        record_from_proto(query_pb2.Record())


def _message_classes() -> list[type[Message]]:
    classes: list[type[Message]] = []
    for module in (query_pb2, results_pb2, selector_pb2, system_pb2, series_service_pb2):
        for name in module.DESCRIPTOR.message_types_by_name:
            classes.append(getattr(module, name))
    return classes


@pytest.mark.parametrize("cls", _message_classes(), ids=lambda c: c.__name__)
def test_generated_messages_live_in_the_package(cls: type[Message]) -> None:
    assert cls.__module__.startswith("zeitreiher_models.grpc.v1.")


def test_a_record_survives_pickle() -> None:
    record = query_pb2.Record()
    record.end.series = 2
    record.end.values = 5
    record.end.next = "abc"
    assert pickle.loads(pickle.dumps(record)) == record
