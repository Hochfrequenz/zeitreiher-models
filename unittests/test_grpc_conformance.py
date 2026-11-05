"""Conformance of the proto mapping, ported from zeitreiher-go (internal/rpc/conformance_test.go).

Every valid fixture of ``testdata/`` is pushed through the proto mapping:

* round trip: fixture -> model -> proto -> model -> JSON must equal the models' own JSON of the model;
* field identity: the proto message as JSON (proto field names), normalized, must equal the fixture,
  normalized. The round trip alone cannot see a mapping bug that is symmetric (``from`` and ``to`` swapped
  in both directions); the identity check can.
"""

import json
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

import pytest

pytest.importorskip("grpc")

from google.protobuf import json_format
from google.protobuf.descriptor import FieldDescriptor
from google.protobuf.message import Message

from unittests.test_fixtures import KINDS, TESTDATA, _files
from zeitreiher_models import grpc as zgrpc
from zeitreiher_models import read_stream
from zeitreiher_models.grpc.v1 import query_pb2, results_pb2, selector_pb2, system_pb2

LOCAL_TESTDATA = Path(__file__).resolve().parent / "grpc_testdata"


@dataclass(frozen=True)
class Mapping:
    """The proto mapping of one fixture kind."""

    to_proto: Callable[[Any], Message]
    from_proto: Callable[[Any], Any]


#: Fixture directory -> proto mapping (the Go test's ``kinds`` table). The parse/dump half is KINDS.
MAPPINGS: dict[str, Mapping] = {
    "query_request": Mapping(zgrpc.query_request_to_proto, zgrpc.query_request_from_proto),
    "resolve_request": Mapping(zgrpc.resolve_request_to_proto, zgrpc.resolve_request_from_proto),
    "versions_request": Mapping(zgrpc.versions_request_to_proto, zgrpc.versions_request_from_proto),
    "records": Mapping(zgrpc.record_to_proto, zgrpc.record_from_proto),
    "resolve_response": Mapping(zgrpc.resolve_response_to_proto, zgrpc.resolve_response_from_proto),
    "versions_response": Mapping(zgrpc.versions_response_to_proto, zgrpc.versions_response_from_proto),
    "system_info": Mapping(zgrpc.system_info_to_proto, zgrpc.system_info_from_proto),
}

#: Fixture directories without a proto counterpart: the error envelope is a gRPC status with ErrorInfo, not a
#: message; streams are checked line by line through the records kind.
UNMAPPED = {"error_envelope", "streams"}


@dataclass(frozen=True)
class Case:
    """One valid fixture: its kind, input and the JSON it must come out as."""

    name: str
    kind: str
    input: bytes
    expected: bytes


def _expected(path: Path) -> bytes:
    canonical = path.with_name(path.name.removesuffix(".json") + ".canonical.json")
    return (canonical if canonical.exists() else path).read_bytes()


def _cases() -> list[Case]:
    cases = [
        Case(f"{kind}/{path.name}", kind, path.read_bytes(), _expected(path))
        for kind in MAPPINGS
        for path in _files(kind, "valid", ".json")
    ]
    # fixtures of this repository's gRPC tests: they cover fields that all shared fixtures leave at zero
    cases += [
        Case(f"local/records/{path.name}", "records", path.read_bytes(), path.read_bytes())
        for path in sorted((LOCAL_TESTDATA / "records").glob("*.json"))
    ]
    # every line of a valid stream is a record fixture of its own
    for path in _files("streams", "valid", ".ndjson"):
        lines = path.read_text(encoding="utf-8").removesuffix("\n").split("\n")
        cases += [
            Case(f"streams/{path.name}#{number}", "records", line.encode(), line.encode())
            for number, line in enumerate(lines, start=1)
        ]
    return sorted(cases, key=lambda c: c.name)


CASES = _cases()
PARAMS = [pytest.param(case, id=case.name) for case in CASES]


def _to_proto(case: Case) -> tuple[Any, Message]:
    parse, _ = KINDS[case.kind]
    parsed = parse(case.input)
    return parsed, MAPPINGS[case.kind].to_proto(parsed)


def test_every_fixture_directory_is_mapped() -> None:
    dirs = {p.name for p in TESTDATA.iterdir() if p.is_dir()}
    assert dirs == {*MAPPINGS, *UNMAPPED}


def test_there_are_fixtures_of_every_kind() -> None:
    assert {c.kind for c in CASES} == set(MAPPINGS)
    assert any(c.name.startswith("streams/") for c in CASES)


@pytest.mark.parametrize("case", PARAMS)
def test_fixture_round_trips_through_proto(case: Case) -> None:
    _, dump = KINDS[case.kind]
    parsed, message = _to_proto(case)
    back = MAPPINGS[case.kind].from_proto(message)
    assert dump(back) == dump(parsed)
    # and through the wire format
    again = MAPPINGS[case.kind].from_proto(type(message).FromString(message.SerializeToString()))
    assert dump(again) == dump(parsed)


@pytest.mark.parametrize("case", PARAMS)
def test_proto_fields_are_the_fixture_fields(case: Case) -> None:
    _, message = _to_proto(case)
    assert _normalized_proto_json(message) == _normalized_fixture_json(message, case.expected)


def _is_repeated(fd: FieldDescriptor) -> bool:
    """``fd.is_repeated`` where it exists (protobuf >= 6.31), else the older ``label``."""
    is_repeated = getattr(fd, "is_repeated", None)
    if is_repeated is not None:
        return bool(is_repeated)
    label: int = getattr(fd, "label")  # noqa: B009
    return label == FieldDescriptor.LABEL_REPEATED


def _is_map(fd: FieldDescriptor) -> bool:
    return fd.message_type is not None and fd.message_type.GetOptions().map_entry


def _normalized_proto_json(message: Message) -> dict[str, Any]:
    """The message as JSON (proto field names, unpopulated fields included) in the shape of the fixtures."""
    obj = json_format.MessageToDict(
        message, preserving_proto_field_name=True, always_print_fields_with_no_presence=True
    )
    if message.DESCRIPTOR.name == "Record":
        # the JSON "type" is the name of the oneof field that is set
        name = message.WhichOneof("record")
        assert name is not None
        inner = obj[name]
        inner["type"] = name
        return _normalize_message(inner, message.DESCRIPTOR.fields_by_name[name].message_type)
    return _normalize_message(obj, message.DESCRIPTOR)


def _normalized_fixture_json(message: Message, fixture: bytes) -> dict[str, Any]:
    obj = json.loads(fixture)
    if message.DESCRIPTOR.name == "Record":
        field = message.DESCRIPTOR.fields_by_name.get(obj["type"])
        assert field is not None
        return _normalize_message(obj, field.message_type)
    return _normalize_message(obj, message.DESCRIPTOR)


def _normalize_message(obj: dict[str, Any], descriptor: Any) -> dict[str, Any]:
    """Bring a JSON object into one comparable shape, guided by the message descriptor.

    int64 strings become numbers, timestamps the instant form, and null, empty arrays and empty maps
    disappear (the models write some of them, the proto JSON others).
    """
    out: dict[str, Any] = {}
    for key, value in obj.items():
        field = descriptor.fields_by_name.get(key)
        if field is None:
            # not a proto field (the record "type", or a key the proto lacks): keep it, so that the comparison
            # reports it
            if value is not None:
                out[key] = value
            continue
        keep, normalized = _normalize_field(value, field)
        if keep:
            out[key] = normalized
    if descriptor.full_name == "zeitreiher.v1.Selector":
        # only the fields of the variant named by `by` are in the JSON
        for key in ("profile", "malo"):
            if out.get(key) == "":
                del out[key]
    elif descriptor.full_name == "zeitreiher.v1.Assignment" and "error" in out and out.get("unit_raw") == "":
        # an assignment with an error carries no unit_raw
        del out["unit_raw"]
    return out


def _normalize_field(value: Any, field: FieldDescriptor) -> tuple[bool, Any]:
    if value is None:
        return False, None
    if _is_map(field):
        assert isinstance(value, dict), f"{field.full_name}: expected a JSON object, got {type(value)}"
        return bool(value), value
    if _is_repeated(field):
        assert isinstance(value, list), f"{field.full_name}: expected a JSON array, got {type(value)}"
        items = [_normalize_value(item, field) for item in value]
        return bool(items), items
    return True, _normalize_value(value, field)


def _normalize_value(value: Any, field: FieldDescriptor) -> Any:
    if field.type == FieldDescriptor.TYPE_INT64 and isinstance(value, str):
        return int(value)
    if field.type == FieldDescriptor.TYPE_MESSAGE:
        assert field.message_type is not None
        if field.message_type.full_name == "google.protobuf.Timestamp":
            if isinstance(value, str):
                return datetime.fromisoformat(value.replace("Z", "+00:00")).isoformat()
            return value
        if isinstance(value, dict):
            return _normalize_message(value, field.message_type)
    return value


_KIND_NAMES = {
    FieldDescriptor.TYPE_STRING: "string",
    FieldDescriptor.TYPE_INT64: "int64",
    FieldDescriptor.TYPE_BOOL: "bool",
}


def _declared_type(field: FieldDescriptor) -> str:
    """``optional string``, ``repeated Version``, ``map<string, string>``: the form of proto_fields.json."""
    if _is_map(field):
        assert field.message_type is not None
        entry = field.message_type.fields_by_name
        return f"map<{_KIND_NAMES[entry['key'].type]}, {_KIND_NAMES[entry['value'].type]}>"
    if field.type == FieldDescriptor.TYPE_MESSAGE:
        assert field.message_type is not None
        kind = field.message_type.full_name.removeprefix("zeitreiher.v1.")
    else:
        kind = _KIND_NAMES[field.type]
    if _is_repeated(field):
        return f"repeated {kind}"
    oneof = field.containing_oneof
    if oneof is not None and oneof.name == f"_{field.name}":  # the synthetic oneof of a proto3 optional
        return f"optional {kind}"
    return kind


def _proto_shape() -> dict[str, dict[str, str]]:
    shape: dict[str, dict[str, str]] = {}
    for module in (query_pb2, results_pb2, selector_pb2, system_pb2):
        for name, message in module.DESCRIPTOR.message_types_by_name.items():
            shape[name] = {f.name: f"{_declared_type(f)} = {f.number}" for f in message.fields}
    return shape


def test_generated_messages_match_pinned_fields() -> None:
    pinned = json.loads((TESTDATA / "proto_fields.json").read_bytes())
    assert _proto_shape() == pinned


def _mark_set_fields(message: Message, seen: set[str]) -> None:
    """Record every field that is set anywhere in ``message`` as ``Message.field``."""
    for field, value in message.ListFields():
        seen.add(f"{message.DESCRIPTOR.name}.{field.name}")
        if field.type != FieldDescriptor.TYPE_MESSAGE:
            continue
        if _is_map(field):
            for item in value.values():
                if isinstance(item, Message):
                    _mark_set_fields(item, seen)
        elif _is_repeated(field):
            for item in value:
                _mark_set_fields(item, seen)
        else:
            _mark_set_fields(value, seen)


def test_every_proto_field_is_exercised() -> None:
    seen: set[str] = set()
    for case in CASES:
        _, message = _to_proto(case)
        _mark_set_fields(message, seen)
    missing = sorted(f"{m}.{f}" for m, fields in _proto_shape().items() for f in fields if f"{m}.{f}" not in seen)
    assert missing == []


def test_streams_convert_record_by_record() -> None:
    for path in _files("streams", "valid", ".ndjson"):
        records = read_stream(path.read_bytes())
        assert [zgrpc.record_from_proto(zgrpc.record_to_proto(r)) for r in records] == list(records)
