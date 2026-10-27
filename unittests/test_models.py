"""Unit tests of the Python models beyond the shared fixtures — mirrors models_test.go."""

import copy
import json
import pickle
from collections.abc import Callable
from datetime import UTC, datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from zoneinfo import ZoneInfo

import pytest
from pydantic import TypeAdapter, ValidationError

from zeitreiher_models import (
    Assignment,
    CurrentVersion,
    CustomSelector,
    EndRecord,
    ErrorDetail,
    ErrorEnvelope,
    ErrorRecord,
    MaLoSelector,
    ProfileSelector,
    QueryRequest,
    RecordErrorCode,
    RequestErrorCode,
    ResolveRequest,
    SeriesRecord,
    SystemInfo,
    ValueRecord,
    Version,
    VersionNumber,
    dump_record,
    iter_records,
    parse_record,
)
from zeitreiher_models.timestamps import RequestInstant

#: Validates and writes a single request instant (as in ``from``/``to`` of a request).
_REQUEST_INSTANT: TypeAdapter[datetime] = TypeAdapter(RequestInstant)

CET = timezone(timedelta(hours=1))


def test_query_request_from_python_objects_keeps_offsets_and_omits_unset_fields() -> None:
    request = QueryRequest(
        select=[MaLoSelector(malo="87253749908"), ProfileSelector(profile="50000000000056")],
        from_=datetime(2026, 1, 1, tzinfo=CET),
        to=datetime(2026, 2, 1, tzinfo=CET),
        version=VersionNumber(number=3),
    )
    assert json.loads(request.model_dump_json()) == {
        "select": [{"by": "malo", "malo": "87253749908"}, {"by": "profile", "profile": "50000000000056"}],
        "from": "2026-01-01T00:00:00+01:00",
        "to": "2026-02-01T00:00:00+01:00",
        "version": {"number": 3},
    }


def test_query_request_accepts_dicts_for_nested_models() -> None:
    request = QueryRequest.model_validate(
        {"select": [{"by": "pod", "params": {}}], "from": "2026-01-01T00:00:00Z", "to": "2026-01-02T00:00:00Z"}
    )
    assert isinstance(request.select[0], CustomSelector)


@pytest.mark.parametrize(
    "version",
    [CurrentVersion(), VersionNumber(number=1)],
)
def test_version_selector_forms_round_trip(version: CurrentVersion | VersionNumber) -> None:
    request = QueryRequest(
        select=[ProfileSelector(profile="56")],
        from_=datetime(2026, 1, 1, tzinfo=UTC),
        to=datetime(2026, 1, 2, tzinfo=UTC),
        version=version,
    )
    assert QueryRequest.model_validate_json(request.model_dump_json()) == request


def test_naive_datetime_is_rejected() -> None:
    with pytest.raises(ValidationError, match="naive"):
        QueryRequest(select=[ProfileSelector(profile="56")], from_=datetime(2026, 1, 1), to=datetime(2026, 1, 2))


def test_sub_second_datetime_is_rejected() -> None:
    with pytest.raises(ValidationError, match="whole seconds"):
        QueryRequest(
            select=[ProfileSelector(profile="56")],
            from_=datetime(2026, 1, 1, 0, 0, 0, 1, tzinfo=UTC),
            to=datetime(2026, 1, 2, tzinfo=UTC),
        )


def test_request_datetime_with_sub_minute_offset_is_rejected() -> None:
    # RFC 3339 has no offset seconds: "+00:00:30" cannot be written, and "+00:00" would move the instant.
    odd = timezone(timedelta(seconds=30))
    with pytest.raises(ValidationError, match="the offset cannot be written without changing the instant"):
        QueryRequest(
            select=[ProfileSelector(profile="56")],
            from_=datetime(2026, 1, 1, tzinfo=odd),
            to=datetime(2026, 1, 2, tzinfo=odd),
        )


def test_response_datetime_whose_utc_conversion_has_microseconds_is_rejected() -> None:
    odd = timezone(timedelta(seconds=30, microseconds=500_000))
    with pytest.raises(ValidationError, match="whole seconds"):
        ValueRecord(
            series=1,
            from_=datetime(2026, 1, 1, tzinfo=odd),
            to=datetime(2026, 1, 2, tzinfo=odd),
            value="1",
            status=[],
            version=None,
        )


def test_response_datetime_with_sub_minute_offset_is_converted_to_utc() -> None:
    # The instant is representable in UTC, so the response side accepts it (as the Go models do).
    value = ValueRecord(
        series=1,
        from_=datetime(2026, 1, 1, tzinfo=timezone(timedelta(seconds=30))),
        to=datetime(2026, 1, 2, tzinfo=UTC),
        value="1",
        status=[],
        version=None,
    )
    assert json.loads(dump_record(value))["from"] == "2025-12-31T23:59:30Z"
    assert parse_record(dump_record(value)) == value


def test_custom_selector_cannot_use_a_built_in_kind() -> None:
    with pytest.raises(ValidationError, match="built-in"):
        CustomSelector(by="malo", params={})


def test_response_datetimes_are_normalized_to_utc() -> None:
    value = ValueRecord(
        series=1,
        from_=datetime(2026, 1, 1, tzinfo=CET),
        to=datetime(2026, 1, 1, 1, tzinfo=CET),
        value="0.022",
        status=["IU015"],
        version=None,
    )
    assert json.loads(dump_record(value)) == {
        "type": "value",
        "series": 1,
        "from": "2025-12-31T23:00:00Z",
        "to": "2026-01-01T00:00:00Z",
        "value": "0.022",
        "status": ["IU015"],
        "version": None,
    }
    assert value.decimal == Decimal("0.022")


def test_series_record_writes_empty_collections() -> None:
    series = SeriesRecord(
        series=1,
        selector=0,
        select=MaLoSelector(malo="87253749908").model_dump(),
        profile="50000000000056",
        from_=datetime(2026, 1, 1, tzinfo=UTC),
        to=datetime(2026, 1, 2, tzinfo=UTC),
        unit="KWH",
        interval="PT15M",
        attributes={},
        versions=[],
    )
    data = json.loads(dump_record(series))
    assert data["attributes"] == {}
    assert data["versions"] == []
    assert data["type"] == "series"


def test_version_created_at_unknown_must_match_null() -> None:
    with pytest.raises(ValidationError, match="created_at_unknown"):
        Version(
            number=1,
            from_=datetime(2026, 1, 1, tzinfo=UTC),
            to=None,
            created_at=None,
            created_at_unknown=False,
            reason="",
            description="",
            deleted=False,
        )


def test_error_record_known_and_unknown_codes() -> None:
    known = parse_record(
        '{"type":"error","selector":0,"select":{"by":"malo","malo":"87253749908"},"code":"no_assignment"}'
    )
    assert isinstance(known, ErrorRecord)
    assert known.code is RecordErrorCode.NO_ASSIGNMENT
    unknown = parse_record(
        '{"type":"error","selector":0,"select":{"by":"malo","malo":"87253749908"},"code":"brand_new"}'
    )
    assert isinstance(unknown, ErrorRecord)
    assert unknown.code == "brand_new"
    assert not isinstance(unknown.code, RecordErrorCode)


def test_error_envelope_code() -> None:
    envelope = ErrorEnvelope.model_validate_json('{"error":{"code":"cursor_stale","message":"assignments changed"}}')
    assert envelope.error.code is RequestErrorCode.CURSOR_STALE
    assert "request_id" not in json.loads(envelope.model_dump_json())["error"]


def test_assignment_with_error_only() -> None:
    assignment = Assignment(profile="50000000000057", error={"code": "access_denied"})
    assert json.loads(assignment.model_dump_json()) == {"profile": "50000000000057", "error": {"code": "access_denied"}}


def test_iter_records_rejects_blank_lines_and_crlf() -> None:
    end = '{"type":"end","series":0,"values":0,"errors":0,"next":null}'
    assert isinstance(next(iter_records([end + "\n"])), EndRecord)
    with pytest.raises(ValueError, match="blank line"):
        list(iter_records(["\n"]))
    with pytest.raises(ValueError, match="carriage return"):
        list(iter_records([end + "\r\n"]))


def test_models_are_frozen() -> None:
    selector = ProfileSelector(profile="56")
    with pytest.raises(ValidationError):
        selector.profile = "57"  # type: ignore[misc]


def test_mappings_are_read_only_sorted_hashable_and_picklable() -> None:
    selector = CustomSelector(by="contract_account", params={"role": "supplier", "account": "1"})
    assert list(selector.params) == ["account", "role"], "keys are sorted, as Go writes map keys"
    with pytest.raises(TypeError):
        selector.params["role"] = "x"  # type: ignore[index]
    with pytest.raises(TypeError):
        selector.params.clear()  # type: ignore[attr-defined]
    assert selector.params == {"account": "1", "role": "supplier"}
    assert hash(selector) == hash(CustomSelector(by="contract_account", params={"account": "1", "role": "supplier"}))
    assert pickle.loads(pickle.dumps(selector)) == selector
    assert copy.deepcopy(selector) == selector
    with pytest.raises(TypeError):
        CurrentVersion().current["x"] = 1  # type: ignore[index]


def test_json_output_escapes_like_go() -> None:
    selector = CustomSelector(by="x", params={"k": "<&>\u2028\u2029"})
    assert selector.model_dump_json() == '{"by":"x","params":{"k":"\\u003c\\u0026\\u003e\\u2028\\u2029"}}'


def test_local_mean_time_offsets_are_rejected_in_requests_and_converted_in_responses() -> None:
    monrovia = ZoneInfo("Africa/Monrovia")  # -00:44:30 until 1972, as in timestamps_test.go
    with pytest.raises(ValidationError, match="the offset cannot be written without changing the instant"):
        _REQUEST_INSTANT.validate_python(datetime(1960, 1, 1, tzinfo=monrovia))
    instant = _REQUEST_INSTANT.validate_python(datetime(2026, 1, 1, tzinfo=monrovia))
    assert _REQUEST_INSTANT.dump_json(instant) == b'"2026-01-01T00:00:00Z"'
    value = ValueRecord(
        series=1,
        from_=datetime(1960, 1, 1, tzinfo=monrovia),
        to=datetime(1960, 1, 2, tzinfo=monrovia),
        value="1",
        status=[],
        version=None,
    )
    assert json.loads(dump_record(value))["from"] == "1960-01-01T00:44:30Z"


@pytest.mark.parametrize(
    "instant",
    [
        datetime(1900, 1, 1, tzinfo=CET),  # 1899-12-31T23:00:00Z
        datetime(9999, 12, 31, 22, tzinfo=timezone(timedelta(hours=-5))),  # 10000-01-01T03:00:00Z
    ],
)
def test_request_year_bounds_apply_to_the_utc_instant(instant: datetime) -> None:
    with pytest.raises(ValidationError, match="outside the years 1900 to 9999 in UTC"):
        _REQUEST_INSTANT.validate_python(instant)


def test_request_year_bounds_accept_the_utc_limits() -> None:
    _REQUEST_INSTANT.validate_python(datetime(1900, 1, 1, tzinfo=timezone(timedelta(hours=-1))))
    _REQUEST_INSTANT.validate_python(datetime(9999, 12, 31, 18, 59, 59, tzinfo=timezone(timedelta(hours=-5))))


@pytest.mark.parametrize(
    "instant",
    [
        datetime(9999, 12, 31, 22, tzinfo=timezone(timedelta(hours=-5))),  # 10000 in UTC
        datetime(1, 1, 1, tzinfo=CET),  # year 0 in UTC
    ],
)
def test_response_datetime_outside_the_utc_range_raises_validation_error(instant: datetime) -> None:
    # Converting to UTC overflows; like Go's error, that must be a ValidationError, not an OverflowError.
    with pytest.raises(ValidationError, match="outside the years 1900 to 9999 in UTC"):
        ValueRecord(series=1, from_=instant, to=datetime(9999, 12, 31, tzinfo=UTC), value="1", status=[], version=None)


_BAD = "a\ud800b"  # a lone surrogate cannot be written as UTF-8
_FROM = datetime(2026, 1, 1, tzinfo=UTC)
_TO = datetime(2026, 1, 2, tzinfo=UTC)
_SERIES: dict[str, Any] = {
    "series": 1,
    "selector": 0,
    "select": {"by": "profile", "profile": "56"},
    "profile": "56",
    "from_": _FROM,
    "to": _TO,
    "unit": "KWH",
    "interval": "PT15M",
    "attributes": {},
    "versions": [],
}
_VERSION: dict[str, Any] = {
    "number": 1,
    "from_": _FROM,
    "to": None,
    "created_at": None,
    "created_at_unknown": True,
    "reason": "",
    "description": "",
    "deleted": False,
}
_SYSTEM: dict[str, Any] = {
    "service_version": "1",
    "abap_package_version": "1",
    "sid": "HF1",
    "client": "100",
    "release": "758",
    "business_time_zone": "CET",
    "intervals": [],
}


@pytest.mark.parametrize(
    "build",
    [
        pytest.param(lambda: CustomSelector(by="x", params={"k": _BAD}), id="selector param value"),
        pytest.param(lambda: CustomSelector(by="x", params={_BAD: "v"}), id="selector param key"),
        pytest.param(
            lambda: QueryRequest(select=[ProfileSelector(profile="56")], from_=_FROM, to=_TO, unit="K\ud800"),
            id="request unit",
        ),
        pytest.param(
            lambda: QueryRequest(select=[ProfileSelector(profile="56")], from_=_FROM, to=_TO, cursor=_BAD),
            id="request cursor",
        ),
        pytest.param(
            lambda: ResolveRequest(select=[{"by": "x", "params": {"k": _BAD}}], from_=_FROM, to=_TO),
            id="resolve param value",
        ),
        pytest.param(lambda: SeriesRecord(**{**_SERIES, "unit": "K\ud800"}), id="series unit"),
        pytest.param(lambda: SeriesRecord(**{**_SERIES, "attributes": {"k": _BAD}}), id="series attribute value"),
        pytest.param(lambda: SeriesRecord(**{**_SERIES, "attributes": {_BAD: "v"}}), id="series attribute key"),
        pytest.param(
            lambda: SeriesRecord(**{**_SERIES, "select": {"by": "x", "params": {"k": _BAD}}}), id="series echoed param"
        ),
        pytest.param(lambda: Version(**{**_VERSION, "reason": _BAD}), id="version reason"),
        pytest.param(lambda: Version(**{**_VERSION, "description": _BAD}), id="version description"),
        pytest.param(
            lambda: ValueRecord(series=1, from_=_FROM, to=_TO, value="1", status=[_BAD], version=None),
            id="value status",
        ),
        pytest.param(lambda: EndRecord(series=0, values=0, errors=0, next=_BAD), id="end next"),
        pytest.param(lambda: ErrorDetail(code="bad_request", message=_BAD), id="error message"),
        pytest.param(lambda: ErrorDetail(code="bad_request", message="m", request_id=_BAD), id="error request id"),
        pytest.param(lambda: SystemInfo(**{**_SYSTEM, "sid": _BAD}), id="system sid"),
        pytest.param(lambda: SystemInfo(**{**_SYSTEM, "release": _BAD}), id="system release"),
    ],
)
def test_lone_surrogates_are_rejected_with_validation_error(build: Callable[[], object]) -> None:
    """Python's counterpart of Go's invalid-UTF-8 check (TestInvalidUTF8RefusesToMarshal).

    Text that cannot be written unchanged is rejected when the model is built, with the usual ValidationError.
    """
    with pytest.raises(ValidationError, match="not valid UTF-8"):
        build()


def test_as_of_is_not_a_version_selector() -> None:
    with pytest.raises(ValidationError, match="exactly one of current or number must be set"):
        QueryRequest.model_validate_json(
            '{"select":[{"by":"profile","profile":"56"}],"from":"2026-01-01T00:00:00Z","to":"2026-01-02T00:00:00Z",'
            '"version":{"as_of":"2026-02-03T08:00:00Z"}}'
        )


@pytest.mark.parametrize("value", ["160", "0.022", "25.25", "-3.5", "0", "100", "-10"])
def test_decimal_in_shortest_form_is_accepted(value: str) -> None:
    assert ValueRecord(series=1, from_=_FROM, to=_TO, value=value, status=[], version=None).value == value


@pytest.mark.parametrize(
    ("value", "shortest"), [("160.0", "160"), ("0.0220", "0.022"), ("0.0", "0"), ("-3.50", "-3.5")]
)
def test_decimal_with_trailing_zeros_is_rejected(value: str, shortest: str) -> None:
    """Responses carry the shortest canonical form: after parsing, 160.0 and 160 are the same number."""
    with pytest.raises(ValidationError, match=f"write '{shortest}'"):
        ValueRecord(series=1, from_=_FROM, to=_TO, value=value, status=[], version=None)
