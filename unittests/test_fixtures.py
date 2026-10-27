"""Shared-fixture round trip — must stay consistent with fixtures_test.go."""

import itertools
import json
from collections import Counter
from collections.abc import Callable
from datetime import timedelta
from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo

import pytest
from pydantic import BaseModel, ValidationError

from zeitreiher_models import (
    EndRecord,
    ErrorEnvelope,
    MissingEndError,
    QueryRequest,
    RecordErrorCode,
    RequestErrorCode,
    ResolveRequest,
    ResolveResponse,
    SeriesRecord,
    SystemInfo,
    ValueRecord,
    VersionsRequest,
    VersionsResponse,
    dump_record,
    parse_record,
    read_stream,
    split_lines,
)
from zeitreiher_models.records import Record

TESTDATA = Path(__file__).resolve().parent.parent / "testdata"


def _model(cls: type[BaseModel]) -> tuple[Callable[[bytes], Any], Callable[[Any], str]]:
    return (cls.model_validate_json), (lambda m: m.model_dump_json())


#: Directory under testdata/ → (parse, dump). The Go tests use the same table (fixtures_test.go).
KINDS: dict[str, tuple[Callable[[bytes], Any], Callable[[Any], str]]] = {
    "query_request": _model(QueryRequest),
    "resolve_request": _model(ResolveRequest),
    "versions_request": _model(VersionsRequest),
    "records": (parse_record, dump_record),
    "resolve_response": _model(ResolveResponse),
    "versions_response": _model(VersionsResponse),
    "error_envelope": _model(ErrorEnvelope),
    "system_info": _model(SystemInfo),
}


def _files(kind: str, validity: str, suffix: str) -> list[Path]:
    files = sorted(p for p in (TESTDATA / kind / validity).glob(f"*{suffix}") if not p.name.endswith(".canonical.json"))
    assert files, f"no fixtures in {kind}/{validity}"
    return files


def _cases(validity: str) -> list[Any]:
    return [
        pytest.param(kind, path, id=f"{kind}/{path.name}") for kind in KINDS for path in _files(kind, validity, ".json")
    ]


def test_fixture_directories_are_all_covered() -> None:
    dirs = {p.name for p in TESTDATA.iterdir() if p.is_dir()}
    assert dirs == {*KINDS, "streams"}


@pytest.mark.parametrize(("kind", "path"), _cases("valid"))
def test_valid_fixture_round_trips(kind: str, path: Path) -> None:
    parse, dump = KINDS[kind]
    canonical = path.with_name(path.name.removesuffix(".json") + ".canonical.json")
    expected = json.loads((canonical if canonical.exists() else path).read_bytes())

    out = dump(parse(path.read_bytes()))
    assert json.loads(out) == expected
    assert dump(parse(out.encode())) == out, "serialization must be idempotent"


def test_serialized_bytes_match_golden() -> None:
    """Python writes exactly the bytes pinned by Go in testdata/serialized.golden (see golden_test.go)."""
    lines = []
    for kind, (parse, dump) in KINDS.items():
        for path in _files(kind, "valid", ".json"):
            lines.append(f"{kind}/{path.name}\t{dump(parse(path.read_bytes()))}")
    for path in _files("streams", "valid", ".ndjson"):
        for number, record in enumerate(read_stream(path.read_bytes()), start=1):
            lines.append(f"streams/{path.name}:{number}\t{dump_record(record)}")
    golden = (TESTDATA / "serialized.golden").read_text(encoding="utf-8").removesuffix("\n").split("\n")
    assert sorted(lines) == golden


#: Invalid fixture → why each language must reject it (shared with fixtures_test.go): for Go a substring
#: of the error, for Python the pydantic error's ``loc`` (dotted) and ``type``, plus ``msg``, a substring
#: of the message, for the error types whose messages the models write themselves (:data:`OWN_ERROR_TYPES`).
#: pydantic's own messages are not compared, so that a pydantic upgrade that rewords them does not break
#: the fixtures.
REASONS: dict[str, dict[str, Any]] = json.loads((TESTDATA / "invalid_reasons.json").read_bytes())

#: Error types raised by the models' validators (``ValueError`` → ``value_error`` and custom error types).
OWN_ERROR_TYPES = frozenset({"value_error", "instant", "unknown_key", "version_selector"})


def _python_reason(exc: ValidationError) -> dict[str, str]:
    """The reason entry for the first error of ``exc`` (to write a new entry of invalid_reasons.json)."""
    error = exc.errors()[0]
    reason = {"loc": ".".join(map(str, error["loc"])), "type": error["type"]}
    if error["type"] in OWN_ERROR_TYPES:
        reason["msg"] = error["msg"].removeprefix("Value error, ")
    return reason


@pytest.mark.parametrize(("kind", "path"), _cases("invalid"))
def test_invalid_fixture_is_rejected(kind: str, path: Path) -> None:
    parse, _ = KINDS[kind]
    with pytest.raises(ValidationError) as excinfo:
        parse(path.read_bytes())
    name = f"{kind}/{path.name}"
    assert name in REASONS, f"add {name} to testdata/invalid_reasons.json, e.g. python: {_python_reason(excinfo.value)}"
    reason = REASONS[name]["python"]
    assert set(reason) <= {"loc", "type", "msg"}
    assert ("msg" in reason) == (reason["type"] in OWN_ERROR_TYPES), "msg is given exactly for the models' own errors"
    matching = [
        error
        for error in excinfo.value.errors()
        if ".".join(map(str, error["loc"])) == reason["loc"]
        and error["type"] == reason["type"]
        and reason.get("msg", "") in error["msg"]
    ]
    assert matching, f"{name}: expected {reason}, got {excinfo.value.errors()}"


def test_invalid_reasons_cover_exactly_the_invalid_fixtures() -> None:
    assert set(REASONS) == {f"{kind}/{path.name}" for kind in KINDS for path in _files(kind, "invalid", ".json")}


def _end_counts_match(records: list[Record]) -> None:
    counts = Counter(r.type for r in records)
    end = records[-1]
    assert isinstance(end, EndRecord)
    assert (end.series, end.values, end.errors) == (counts["series"], counts["value"], counts["error"])


@pytest.mark.parametrize("path", _files("streams", "valid", ".ndjson"), ids=lambda p: p.name)
def test_valid_stream(path: Path) -> None:
    data = path.read_bytes()
    records = read_stream(data)
    lines = split_lines(data)
    assert len(records) == len(lines)
    for record, line in zip(records, lines, strict=True):
        assert json.loads(dump_record(record)) == json.loads(line)
    _end_counts_match(records)


@pytest.mark.parametrize("path", _files("streams", "invalid", ".ndjson"), ids=lambda p: p.name)
def test_invalid_stream_is_rejected(path: Path) -> None:
    with pytest.raises(ValueError):  # ValidationError and MissingEndError are ValueErrors
        read_stream(path.read_bytes())


def test_missing_end_is_reported_as_such() -> None:
    with pytest.raises(MissingEndError):
        read_stream((TESTDATA / "streams" / "invalid" / "missing_end.ndjson").read_bytes())


def _stream(name: str) -> tuple[SeriesRecord, list[ValueRecord]]:
    records = read_stream((TESTDATA / "streams" / "valid" / name).read_bytes())
    series = records[0]
    assert isinstance(series, SeriesRecord)
    return series, [r for r in records if isinstance(r, ValueRecord)]


@pytest.mark.parametrize(("name", "count"), [("dst_spring_day_pt15m.ndjson", 92), ("dst_autumn_day_pt15m.ndjson", 100)])
def test_dst_streams_cover_the_local_day(name: str, count: int) -> None:
    berlin = ZoneInfo("Europe/Berlin")
    series, values = _stream(name)
    assert len(values) == count
    assert values[0].from_ == series.from_
    assert values[-1].to == series.to
    for previous, current in itertools.pairwise(values):
        assert previous.to == current.from_, "values must be contiguous"
    assert all(v.to - v.from_ == timedelta(minutes=15) for v in values)
    local_from = series.from_.astimezone(berlin)
    local_to = series.to.astimezone(berlin)
    assert (local_from.hour, local_from.minute) == (0, 0)
    assert (local_to.hour, local_to.minute) == (0, 0)
    assert local_to.date() - local_from.date() == timedelta(days=1)


def test_gaps_stay_gaps() -> None:
    series, values = _stream("gaps.ndjson")
    holes = sum(1 for a, b in itertools.pairwise(values) if a.to != b.from_)
    assert holes == 1, "one interior gap"
    assert values[-1].to < series.to, "tail gap"


def test_version_number_stream() -> None:
    """``{"number":3}``: current values overlaid with stored values of versions >= 3 (lowest wins), only
    inside version 3's covered range; each value says its source (version row or ``None`` = current)."""
    series, values = _stream("version_number.ndjson")
    assert series.versions[0].number == 3
    assert all(v.from_ >= series.versions[0].from_ for v in values), "no value outside the covered range"
    assert [v.version for v in values] == [3, 3, 4, None]
    assert series.from_ < values[0].from_, "the part before the covered range is a gap"


def test_paged_stream_resends_series() -> None:
    first, _ = _stream("paged_page1.ndjson")
    second, _ = _stream("paged_page2.ndjson")
    assert first == second


def test_error_codes_match_fixture() -> None:
    codes = json.loads((TESTDATA / "error_codes.json").read_bytes())
    assert [c.value for c in RequestErrorCode] == codes["request"]
    assert [c.value for c in RecordErrorCode] == codes["record"]
    for request_code in RequestErrorCode:
        assert (TESTDATA / "error_envelope" / "valid" / f"{request_code.value}.json").exists()
    for record_code in RecordErrorCode:
        assert (TESTDATA / "records" / "valid" / f"error_{record_code.value}.json").exists()
