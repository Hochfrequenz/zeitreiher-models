"""Converters between the generated ``zeitreiher.v1`` messages and the hand-written models.

A port of zeitreiher-go's ``internal/rpc`` (``convert.go`` and the directions of ``convert_test.go``):

* requests, proto -> models, go through the models' JSON parser, so proto and JSON requests share exactly the
  same rules and error messages; they raise :class:`pydantic.ValidationError` like the models do;
* request instants stay RFC 3339 strings (the offset a client sent is kept);
* response instants are :class:`google.protobuf.timestamp_pb2.Timestamp` in the proto and aware UTC
  :class:`~datetime.datetime` in the models (whole seconds only);
* decimal values stay strings;
* proto3 ``optional`` presence maps to ``None``/unset exactly as in the models.

The messages have fields called ``from``; they are set and read with ``setattr``/``getattr``.
"""

import json
from collections.abc import Iterable
from datetime import UTC, datetime, timedelta
from typing import Any

from google.protobuf.timestamp_pb2 import Timestamp

from zeitreiher_models.grpc.v1 import query_pb2, results_pb2, selector_pb2, system_pb2
from zeitreiher_models.records import EndRecord, ErrorRecord, Record, SeriesRecord, ValueRecord, Version
from zeitreiher_models.requests import QueryRequest, ResolveRequest, VersionsRequest
from zeitreiher_models.results import (
    Assignment,
    ResolveResponse,
    ResolveResult,
    VersionedAssignment,
    VersionsResponse,
    VersionsResult,
)
from zeitreiher_models.selectors import (
    EchoedCustomSelector,
    EchoedMaLoSelector,
    EchoedProfileSelector,
    EchoedSelector,
    MaLoSelector,
    ProfileSelector,
    Selector,
)
from zeitreiher_models.system import Caps, InactiveObjects, SystemInfo
from zeitreiher_models.timestamps import _format

_EPOCH = datetime(1970, 1, 1, tzinfo=UTC)

# --------------------------------------------------------------------------------------------------
# Selectors and instants
# --------------------------------------------------------------------------------------------------


def _selector_to_proto(selector: Selector | EchoedSelector) -> selector_pb2.Selector:
    out = selector_pb2.Selector(by=selector.by)
    if isinstance(selector, ProfileSelector | EchoedProfileSelector):
        out.profile = selector.profile
    elif isinstance(selector, MaLoSelector | EchoedMaLoSelector):
        out.malo = selector.malo
    else:
        out.params.update(selector.params)
    return out


def _echoed_selector_from_proto(s: selector_pb2.Selector) -> EchoedSelector:
    if s.by == "profile":
        return EchoedProfileSelector(profile=s.profile)
    if s.by == "malo":
        return EchoedMaLoSelector(malo=s.malo)
    return EchoedCustomSelector(by=s.by, params=dict(s.params))


def _instant_to_proto(value: datetime) -> Timestamp:
    return Timestamp(seconds=(value - _EPOCH) // timedelta(seconds=1))


def _set_instant(message: Any, field: str, value: datetime | None) -> None:
    """Set the Timestamp field ``field`` of ``message``; ``None`` leaves it unset."""
    if value is not None:
        getattr(message, field).CopyFrom(_instant_to_proto(value))


def _instant_from_proto(ts: Timestamp) -> datetime:
    """The aware UTC datetime of ``ts``; an unset timestamp is the epoch, as in Go."""
    if ts.nanos:
        raise ValueError(f"timestamp {ts.seconds}s {ts.nanos}ns: instants have whole seconds")
    return datetime.fromtimestamp(ts.seconds, UTC)


def _optional_instant_from_proto(message: Any, field: str) -> datetime | None:
    return _instant_from_proto(getattr(message, field)) if message.HasField(field) else None


def _optional(message: Any, field: str) -> Any:
    """The value of the proto3 ``optional`` field, or ``None`` when it is not set."""
    return getattr(message, field) if message.HasField(field) else None


# --------------------------------------------------------------------------------------------------
# Requests
# --------------------------------------------------------------------------------------------------


def _window_to_proto(message: Any, request: QueryRequest | ResolveRequest | VersionsRequest) -> None:
    message.select.extend(_selector_to_proto(s) for s in request.select)
    setattr(message, "from", _format(request.from_))
    message.to = _format(request.to)


def query_request_to_proto(request: QueryRequest) -> query_pb2.QueryRequest:
    """The proto message of a query request; request instants stay RFC 3339 strings."""
    out = query_pb2.QueryRequest()
    _window_to_proto(out, request)
    version = request.version
    if version is not None:
        out.version.SetInParent()
        if hasattr(version, "number"):
            out.version.number = version.number
        else:
            out.version.current.SetInParent()
    if request.interval is not None:
        out.interval = request.interval
    if request.unit is not None:
        out.unit = request.unit
    if request.cursor is not None:
        out.cursor = request.cursor
    return out


def resolve_request_to_proto(request: ResolveRequest) -> results_pb2.ResolveRequest:
    """The proto message of a resolve request."""
    out = results_pb2.ResolveRequest()
    _window_to_proto(out, request)
    return out


def versions_request_to_proto(request: VersionsRequest) -> results_pb2.ListVersionsRequest:
    """The proto message of a versions request."""
    out = results_pb2.ListVersionsRequest()
    _window_to_proto(out, request)
    return out


def _selector_json(s: selector_pb2.Selector) -> dict[str, Any]:
    """Write the keys of the variant named by ``by`` and every other key that is set.

    The models then refuse a selector that mixes variants.
    """
    obj: dict[str, Any] = {"by": s.by}
    if s.by == "profile" or s.profile != "":
        obj["profile"] = s.profile
    if s.by == "malo" or s.malo != "":
        obj["malo"] = s.malo
    if s.by not in ("profile", "malo") or len(s.params) > 0:
        obj["params"] = dict(s.params)
    return obj


def _window_json(message: Any) -> dict[str, Any]:
    return {
        "select": [_selector_json(s) for s in message.select],
        "from": getattr(message, "from"),
        "to": message.to,
    }


def query_request_from_proto(message: query_pb2.QueryRequest) -> QueryRequest:
    """The query request of a proto message, validated by the models' JSON parser.

    Raises:
        pydantic.ValidationError: the request breaks a rule of the models (as the same request in JSON would).
    """
    obj = _window_json(message)
    if message.HasField("version"):
        version: dict[str, Any] = {}
        kind = message.version.WhichOneof("kind")
        if kind == "current":
            version["current"] = {}
        elif kind == "number":
            version["number"] = message.version.number
        # an unset oneof stays an empty object: the models refuse it
        obj["version"] = version
    for field in ("interval", "unit", "cursor"):
        if message.HasField(field):
            obj[field] = getattr(message, field)
    return QueryRequest.model_validate_json(json.dumps(obj))


def resolve_request_from_proto(message: results_pb2.ResolveRequest) -> ResolveRequest:
    """The resolve request of a proto message; see :func:`query_request_from_proto`."""
    return ResolveRequest.model_validate_json(json.dumps(_window_json(message)))


def versions_request_from_proto(message: results_pb2.ListVersionsRequest) -> VersionsRequest:
    """The versions request of a proto message; see :func:`query_request_from_proto`."""
    return VersionsRequest.model_validate_json(json.dumps(_window_json(message)))


# --------------------------------------------------------------------------------------------------
# Versions and records
# --------------------------------------------------------------------------------------------------


def _versions_to_proto(versions: Iterable[Version]) -> list[query_pb2.Version]:
    out = []
    for v in versions:
        m = query_pb2.Version(
            number=v.number,
            created_at_unknown=v.created_at_unknown,
            reason=v.reason,
            description=v.description,
            deleted=v.deleted,
        )
        _set_instant(m, "from", v.from_)
        _set_instant(m, "to", v.to)
        _set_instant(m, "created_at", v.created_at)
        out.append(m)
    return out


def _versions_from_proto(versions: Iterable[query_pb2.Version]) -> tuple[Version, ...]:
    return tuple(
        Version(
            number=v.number,
            from_=_instant_from_proto(getattr(v, "from")),
            to=_optional_instant_from_proto(v, "to"),
            created_at=_optional_instant_from_proto(v, "created_at"),
            created_at_unknown=v.created_at_unknown,
            reason=v.reason,
            description=v.description,
            deleted=v.deleted,
        )
        for v in versions
    )


def record_to_proto(record: Record) -> query_pb2.Record:
    """The proto message of one record of the ``Query`` stream.

    Raises:
        TypeError: ``record`` is not one of the four record models.
    """
    out = query_pb2.Record()
    if isinstance(record, SeriesRecord):
        s = out.series
        s.series = record.series
        s.selector = record.selector
        s.select.CopyFrom(_selector_to_proto(record.select))
        s.profile = record.profile
        _set_instant(s, "from", record.from_)
        _set_instant(s, "to", record.to)
        if record.unit is not None:
            s.unit = record.unit
        s.unit_raw = record.unit_raw
        s.interval = record.interval
        s.attributes.update(record.attributes)
        s.versions.extend(_versions_to_proto(record.versions))
        if record.role is not None:
            s.role = record.role
    elif isinstance(record, ValueRecord):
        v = out.value
        v.series = record.series
        _set_instant(v, "from", record.from_)
        _set_instant(v, "to", record.to)
        v.value = record.value
        v.status.extend(record.status)
        if record.version is not None:
            v.version = record.version
    elif isinstance(record, ErrorRecord):
        e = out.error
        e.selector = record.selector
        e.select.CopyFrom(_selector_to_proto(record.select))
        if record.profile is not None:
            e.profile = record.profile
        e.code = str(record.code)
    elif isinstance(record, EndRecord):
        end = out.end
        end.series = record.series
        end.values = record.values
        end.errors = record.errors
        if record.next is not None:
            end.next = record.next
    else:
        raise TypeError(f"record type {type(record).__name__} has no proto message")
    return out


def record_from_proto(message: query_pb2.Record) -> Record:
    """The record model of one proto record.

    Raises:
        ValueError: no variant of the ``record`` oneof is set.
    """
    kind = message.WhichOneof("record")
    if kind == "series":
        s = message.series
        return SeriesRecord(
            series=s.series,
            selector=s.selector,
            select=_echoed_selector_from_proto(s.select),
            profile=s.profile,
            role=_optional(s, "role"),
            from_=_instant_from_proto(getattr(s, "from")),
            to=_instant_from_proto(s.to),
            unit=_optional(s, "unit"),
            unit_raw=s.unit_raw,
            interval=s.interval,
            attributes=dict(s.attributes),
            versions=_versions_from_proto(s.versions),
        )
    if kind == "value":
        v = message.value
        return ValueRecord(
            series=v.series,
            from_=_instant_from_proto(getattr(v, "from")),
            to=_instant_from_proto(v.to),
            value=v.value,
            status=tuple(v.status),
            version=_optional(v, "version"),
        )
    if kind == "error":
        e = message.error
        return ErrorRecord(
            selector=e.selector,
            select=_echoed_selector_from_proto(e.select),
            profile=_optional(e, "profile"),
            code=e.code,
        )
    if kind == "end":
        end = message.end
        return EndRecord(series=end.series, values=end.values, errors=end.errors, next=_optional(end, "next"))
    raise ValueError("record without a set oneof")


# --------------------------------------------------------------------------------------------------
# Responses
# --------------------------------------------------------------------------------------------------


def _result_error_to_proto(error: Any) -> results_pb2.ResultError:
    return results_pb2.ResultError(code=str(error.code))


def _assignment_to_proto(a: Assignment) -> results_pb2.Assignment:
    out = results_pb2.Assignment(profile=a.profile, unit_raw=a.unit_raw or "")
    if a.role is not None:
        out.role = a.role
    if a.unit is not None:
        out.unit = a.unit
    _set_instant(out, "from", a.from_)
    _set_instant(out, "to", a.to)
    if a.error is not None:
        out.error.CopyFrom(_result_error_to_proto(a.error))
    return out


def _result_to_proto(
    result: ResolveResult | VersionsResult, assignments: Iterable[results_pb2.Assignment] | None
) -> results_pb2.SelectorResult:
    out = results_pb2.SelectorResult(selector=result.selector)
    out.select.CopyFrom(_selector_to_proto(result.select))
    if result.error is not None:
        out.error.CopyFrom(_result_error_to_proto(result.error))
    if assignments is not None:
        out.assignments.extend(assignments)
    return out


def resolve_response_to_proto(response: ResolveResponse) -> results_pb2.ResolveResponse:
    """The proto message of a resolve response."""
    out = results_pb2.ResolveResponse()
    for r in response.results:
        assignments = None if r.assignments is None else [_assignment_to_proto(a) for a in r.assignments]
        out.results.append(_result_to_proto(r, assignments))
    return out


def versions_response_to_proto(response: VersionsResponse) -> results_pb2.ListVersionsResponse:
    """The proto message of a versions response."""
    out = results_pb2.ListVersionsResponse()
    for r in response.results:
        assignments = None
        if r.assignments is not None:
            assignments = []
            for a in r.assignments:
                pa = _assignment_to_proto(a)
                pa.versions.extend(_versions_to_proto(a.versions or ()))
                assignments.append(pa)
        out.results.append(_result_to_proto(r, assignments))
    return out


def _assignment_fields_from_proto(a: results_pb2.Assignment) -> dict[str, Any]:
    return {
        "profile": a.profile,
        "role": _optional(a, "role"),
        "from_": _optional_instant_from_proto(a, "from"),
        "to": _optional_instant_from_proto(a, "to"),
        "unit": _optional(a, "unit"),
        # an assignment with an error has no unit_raw (the proto string is "")
        "unit_raw": a.unit_raw or None,
        "error": {"code": a.error.code} if a.HasField("error") else None,
    }


def resolve_response_from_proto(message: results_pb2.ResolveResponse) -> ResolveResponse:
    """The resolve response model of a proto message."""
    results = []
    for r in message.results:
        error = {"code": r.error.code} if r.HasField("error") else None
        # exactly one of assignments and error: an empty repeated field cannot tell "none" from "unset"
        assignments = (
            None if error is not None else tuple(Assignment(**_assignment_fields_from_proto(a)) for a in r.assignments)
        )
        results.append(
            ResolveResult(
                selector=r.selector,
                select=_echoed_selector_from_proto(r.select),
                error=error,
                assignments=assignments,
            )
        )
    return ResolveResponse(results=tuple(results))


def versions_response_from_proto(message: results_pb2.ListVersionsResponse) -> VersionsResponse:
    """The versions response model of a proto message."""
    results = []
    for r in message.results:
        error = {"code": r.error.code} if r.HasField("error") else None
        assignments: tuple[VersionedAssignment, ...] | None = None
        if error is None:
            assignments = tuple(
                VersionedAssignment(
                    **_assignment_fields_from_proto(a),
                    versions=None if a.HasField("error") else _versions_from_proto(a.versions),
                )
                for a in r.assignments
            )
        results.append(
            VersionsResult(
                selector=r.selector,
                select=_echoed_selector_from_proto(r.select),
                error=error,
                assignments=assignments,
            )
        )
    return VersionsResponse(results=tuple(results))


# --------------------------------------------------------------------------------------------------
# System info
# --------------------------------------------------------------------------------------------------


def system_info_to_proto(info: SystemInfo) -> system_pb2.SystemResponse:
    """The proto message of the system info."""
    message = system_pb2.SystemResponse(
        service_version=info.service_version,
        abap_package_version=info.abap_package_version,
        sid=info.sid,
        client=info.client,
        release=info.release,
        business_time_zone=info.business_time_zone,
        intervals=info.intervals,
        units=info.units,
        caps=system_pb2.Caps(
            max_window_seconds=info.caps.max_window_seconds,
            max_values=info.caps.max_values,
            max_resolve_window_seconds=info.caps.max_resolve_window_seconds,
        ),
        abap_source_fingerprint=info.abap_source_fingerprint,
    )
    _set_instant(message, "abap_last_source_change_utc", info.abap_last_source_change_utc)
    if info.abap_inactive_objects is not None:
        inactive = info.abap_inactive_objects
        message.abap_inactive_objects.CopyFrom(
            system_pb2.InactiveObjects(
                available=inactive.available,
                count=inactive.count,
                names=inactive.names,
                reason=inactive.reason,
            )
        )
    return message


def system_info_from_proto(message: system_pb2.SystemResponse) -> SystemInfo:
    """The system info model of a proto message."""
    return SystemInfo(
        service_version=message.service_version,
        abap_package_version=message.abap_package_version,
        sid=message.sid,
        client=message.client,
        release=message.release,
        business_time_zone=message.business_time_zone,
        intervals=tuple(message.intervals),
        units=tuple(message.units),
        caps=Caps(
            max_window_seconds=message.caps.max_window_seconds,
            max_values=message.caps.max_values,
            max_resolve_window_seconds=message.caps.max_resolve_window_seconds,
        ),
        abap_last_source_change_utc=_optional_instant_from_proto(message, "abap_last_source_change_utc"),
        abap_inactive_objects=_inactive_objects_from_proto(message.abap_inactive_objects)
        if message.HasField("abap_inactive_objects")
        else None,
        abap_source_fingerprint=(
            message.abap_source_fingerprint if message.HasField("abap_source_fingerprint") else None
        ),
    )


def _inactive_objects_from_proto(message: system_pb2.InactiveObjects) -> InactiveObjects:
    return InactiveObjects(
        available=message.available,
        count=message.count if message.HasField("count") else None,
        names=tuple(message.names) if message.available else None,
        reason=message.reason if message.HasField("reason") else None,
    )
