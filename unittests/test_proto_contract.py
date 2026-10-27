"""Drift check: the hand-written proto messages carry the same field names as the hand-written models.

The proto is the published gRPC contract; ``zeitreiher-go`` maps proto messages to the models by
hand. This test makes sure that a field added to a model is also added to the proto (and vice versa).
"""

import json
import re
from pathlib import Path

import pytest
from pydantic import BaseModel

from zeitreiher_models import (
    Assignment,
    Caps,
    CurrentVersion,
    CustomSelector,
    EndRecord,
    ErrorRecord,
    MaLoSelector,
    ProfileSelector,
    QueryRequest,
    RecordErrorCode,
    RequestErrorCode,
    ResolveRequest,
    ResolveResponse,
    ResolveResult,
    ResultError,
    SeriesRecord,
    SystemInfo,
    ValueRecord,
    Version,
    VersionedAssignment,
    VersionNumber,
    VersionsRequest,
    VersionsResponse,
)

PROTO_DIR = Path(__file__).resolve().parent.parent / "proto" / "zeitreiher" / "v1"

_MESSAGE = re.compile(r"^message (\w+) \{(?:\}|(.*?)^\})", re.MULTILINE | re.DOTALL)
_FIELD = re.compile(
    r"^\s*(?:optional\s+|repeated\s+)?(?:map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*\d+\s*;",
    re.MULTILINE,
)


_FIELD_DECL = re.compile(
    r"^\s*((?:optional\s+|repeated\s+)?(?:map<[^>]+>|[\w.]+))\s+(\w+)\s*=\s*(\d+)\s*;",
    re.MULTILINE,
)

#: Message → field → "<label and type> = <number>". Field numbers are the wire contract of gRPC: they
#: must never change; new fields are added here together with the proto.
PINNED_FIELDS = Path(__file__).resolve().parent.parent / "testdata" / "proto_fields.json"


def _proto_field_declarations() -> dict[str, dict[str, str]]:
    declarations: dict[str, dict[str, str]] = {}
    for path in sorted(PROTO_DIR.glob("*.proto")):
        text = re.sub(r"//[^\n]*", "", path.read_text(encoding="utf-8"))
        for name, body in _MESSAGE.findall(text):
            declarations[name] = {
                field: f"{' '.join(kind.split())} = {number}" for kind, field, number in _FIELD_DECL.findall(body)
            }
    return declarations


def test_proto_field_numbers_and_types_are_pinned() -> None:
    pinned: dict[str, dict[str, str]] = json.loads(PINNED_FIELDS.read_bytes())
    current = _proto_field_declarations()
    for message, fields in pinned.items():
        for field, declaration in fields.items():
            assert current.get(message, {}).get(field) == declaration, (
                f"{message}.{field} changed from {declaration!r}: field numbers and types must never change"
            )
    for message, fields in current.items():
        numbers = [declaration.rsplit(" = ", 1)[1] for declaration in fields.values()]
        assert len(numbers) == len(set(numbers)), f"{message} reuses a field number"
        for field, declaration in fields.items():
            assert field in pinned.get(message, {}), (
                f"pin the new field {message}.{field} ({declaration}) in {PINNED_FIELDS.name}"
            )


def _proto_messages() -> dict[str, set[str]]:
    messages: dict[str, set[str]] = {}
    for path in sorted(PROTO_DIR.glob("*.proto")):
        text = re.sub(r"//[^\n]*", "", path.read_text(encoding="utf-8"))
        for name, body in _MESSAGE.findall(text):
            messages[name] = set(_FIELD.findall(body))
    return messages


def _json_keys(*models: type[BaseModel]) -> set[str]:
    keys: set[str] = set()
    for model in models:
        keys |= {field.alias or name for name, field in model.model_fields.items()}
    return keys - {"type"}  # the record type is the name of the proto oneof field


#: proto message → the models whose JSON keys it must carry (a union where the proto is flat).
TABLE: dict[str, tuple[type[BaseModel], ...]] = {
    "Selector": (ProfileSelector, MaLoSelector, CustomSelector),
    "QueryRequest": (QueryRequest,),
    "VersionSelector": (CurrentVersion, VersionNumber),
    "SeriesRecord": (SeriesRecord,),
    "Version": (Version,),
    "ValueRecord": (ValueRecord,),
    "ErrorRecord": (ErrorRecord,),
    "EndRecord": (EndRecord,),
    "ResolveRequest": (ResolveRequest,),
    "ListVersionsRequest": (VersionsRequest,),
    "ResolveResponse": (ResolveResponse,),
    "ListVersionsResponse": (VersionsResponse,),
    "SelectorResult": (ResolveResult,),
    "Assignment": (Assignment, VersionedAssignment),
    "ResultError": (ResultError,),
    "SystemResponse": (SystemInfo,),
    "Caps": (Caps,),
}


@pytest.mark.parametrize("message", sorted(TABLE))
def test_proto_message_matches_models(message: str) -> None:
    assert _proto_messages()[message] == _json_keys(*TABLE[message])


def test_record_oneof_matches_record_types() -> None:
    assert _proto_messages()["Record"] == {"series", "value", "error", "end"}


def test_every_proto_message_is_checked() -> None:
    assert set(_proto_messages()) == {*TABLE, "Record", "CurrentVersion", "SystemRequest"}


def _proto_text(name: str) -> str:
    return (PROTO_DIR / name).read_text(encoding="utf-8")


def test_proto_documents_grpc_error_details() -> None:
    """Request-level failures are a gRPC status with a google.rpc.ErrorInfo detail (decision 10)."""
    service = _proto_text("series_service.proto")
    assert "google.rpc.ErrorInfo" in service
    assert 'domain = "zeitreiher.hochfrequenz.de"' in service
    assert "reason = <public request-level code>" in service
    for code in RequestErrorCode:
        assert re.search(rf"\b{code.value}\b", service), f"series_service.proto does not mention {code.value}"


def test_proto_lists_every_record_level_code() -> None:
    query = _proto_text("query.proto")
    for code in RecordErrorCode:
        assert re.search(rf"\b{code.value}\b", query), f"query.proto (ErrorRecord.code) does not list {code.value}"
