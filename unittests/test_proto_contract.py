"""Drift check: the hand-written proto messages carry the same field names as the hand-written models.

The proto is the published gRPC contract; ``zeitreiher-go`` maps proto messages to the models by
hand. This test makes sure that a field added to a model is also added to the proto (and vice versa).
"""

import re
from pathlib import Path

import pytest
from pydantic import BaseModel

from zeitreiher_models import (
    Assignment,
    CurrentVersion,
    CustomSelector,
    EndRecord,
    ErrorRecord,
    MaLoSelector,
    ProfileSelector,
    QueryRequest,
    ResolveRequest,
    ResolveResponse,
    ResolveResult,
    ResultError,
    SeriesRecord,
    SystemInfo,
    ValueRecord,
    Version,
    VersionAsOf,
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
    "VersionSelector": (CurrentVersion, VersionNumber, VersionAsOf),
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
}


@pytest.mark.parametrize("message", sorted(TABLE))
def test_proto_message_matches_models(message: str) -> None:
    assert _proto_messages()[message] == _json_keys(*TABLE[message])


def test_record_oneof_matches_record_types() -> None:
    assert _proto_messages()["Record"] == {"series", "value", "error", "end"}


def test_every_proto_message_is_checked() -> None:
    assert set(_proto_messages()) == {*TABLE, "Record", "CurrentVersion", "SystemRequest"}
