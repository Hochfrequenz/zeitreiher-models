# zeitreiher-models

![Go Tests](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Go%20Tests/badge.svg)
![Go Lint](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Go%20Lint/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/hochfrequenz/zeitreiher-models.svg)](https://pkg.go.dev/github.com/hochfrequenz/zeitreiher-models)
![Python Tests](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Python%20Tests/badge.svg)
![Python Coverage](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Python%20Coverage/badge.svg)
![Python Lint](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Python%20Lint/badge.svg)
![Python Formatting](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Python%20Formatting/badge.svg)
![Proto](https://github.com/Hochfrequenz/zeitreiher-models/workflows/Proto/badge.svg)

Public data models of the **Zeitreiher API, version 1**: a read-only API over SAP IS-U EDM profile
values (Lastgänge) — profile metadata plus many `[from, to)` intervals with values.

This repository holds

- **Go** structs (package `zeitreihermodels`, module `github.com/hochfrequenz/zeitreiher-models`),
- **Python** frozen pydantic v2 models (package `zeitreiher_models`),
- the shared **fixtures** in [`testdata/`](testdata) that define the JSON contract, and
- the **gRPC contract** in [`proto/zeitreiher/v1`](proto/zeitreiher/v1).

Everything is **written by hand**. No code is generated and no generated code is committed: every
type, validator and (un)marshaller is ordinary, readable code. Both languages parse every valid
fixture, write it back and compare the result with the fixture, and reject every invalid fixture —
so Go and Python agree on the contract by construction.

## The contract in short

| Topic | Rule |
|---|---|
| Intervals | Half-open everywhere: `[from, to)`; `from` must be before `to`. |
| Request timestamps | RFC 3339, whole seconds, **explicit offset** (`Z` or `±hh:mm`), ASCII digits only, hours 00–23. The instant must lie in the years 1900–9999 **in UTC**, so that it can be echoed in a response (`1900-01-01T00:00:00+01:00` is 1899 in UTC and rejected). Kept as sent (`+02:00` stays `+02:00`). A missing offset (`2026-01-01T00:00:00`) and the "unknown offset" `-00:00` are rejected. |
| Response timestamps | RFC 3339, whole seconds, always UTC (`Z`). Instants built in code must survive the round trip (no offsets with seconds, years 1900–9999). |
| Values | Already-scaled decimal **strings** (`"160"`, `"0.022"`, `"-12.5"`), never JSON numbers. No exponent, no plus sign, no leading zeros, no negative zero. |
| Profile numbers | In requests 1–18 digits, leading zeros allowed, not all zeros; in responses without leading zeros. |
| Units | SAP internal units (T006-MSEHI): 1–3 characters out of `A–Z a–z 0–9 % ° / . _ -`, e.g. `KWH`. |
| Selectors | `{"by":"profile","profile":"…"}`, `{"by":"malo","malo":"<11 digits>"}`, or a customer resolver `{"by":"<snake_case name>","params":{"<key>":"<string>"}}`. |
| Version selector | Exactly one of `{"current":{}}` (default), `{"number":N}`, `{"as_of":"<timestamp>"}`; no `null` members. |
| Intervals (`interval`) | Sub-daily ISO 8601 durations in minutes: `PT15M`, `PT60M`, … |
| Unknown keys | Requests **reject** them (a typo fails loudly). Responses **ignore** them (additive server changes do not break older clients). |
| `null` | Request fields that are optional may be omitted or `null`. In responses, `value.version`, `end.next`, `version.to` and `version.created_at` are always present and may be `null`; other optional response fields are omitted when unset. |
| Error codes | Request-level codes in the error envelope `{"error":{"code","message","request_id"?}}`; record-level codes in `error` records and resolve/versions results. The known codes are listed in [`testdata/error_codes.json`](testdata/error_codes.json). Parsers accept unknown snake_case codes so that a newer server does not break older clients (Go `IsKnown()`, Python `RecordErrorCode | str`). |
| NDJSON | One record per line, lines end with `\n` (the last one optionally), no `\r`, no blank lines. A response that does not end with exactly one `end` record was cut off and **must be treated as a failure**. |

The full design lives in the `zeitreiher-go` repository (`docs/superpowers/specs/2026-10-27-zeitreiher-design.md`).

## Go

```bash
go get github.com/hochfrequenz/zeitreiher-models
```

```go
import zrm "github.com/hochfrequenz/zeitreiher-models"

cet := time.FixedZone("CET", 3600)
req := zrm.QueryRequest{
	Select:   []zrm.Selector{zrm.MaLoSelector("87253749908"), zrm.ProfileSelector("50000000000056")},
	From:     time.Date(2026, 1, 1, 0, 0, 0, 0, cet),
	To:       time.Date(2026, 2, 1, 0, 0, 0, 0, cet),
	Interval: "PT60M",
}
body, err := json.Marshal(req) // validates; an invalid request does not marshal

records, err := zrm.ReadStream(resp.Body) // errors.Is(err, zrm.ErrMissingEnd) for a cut-off stream
for _, rec := range records {
	switch r := rec.(type) {
	case *zrm.SeriesRecord:
	case *zrm.ValueRecord: // r.Value is a zrm.Decimal (string)
	case *zrm.ErrorRecord: // r.Code.IsKnown()
	case *zrm.EndRecord: // r.Next != nil → request the next page with Cursor = *r.Next
	}
}
```

`ReadRecords(io.Reader)` yields records one by one (`iter.Seq2[Record, error]`) for streaming;
`WriteRecord` writes one NDJSON line. Every type validates itself in `UnmarshalJSON` and refuses
to marshal when it is invalid.

## Python

```bash
pip install zeitreiher-models  # once published to PyPI
```

```python
from datetime import datetime, timedelta, timezone

from zeitreiher_models import MaLoSelector, QueryRequest, ValueRecord, iter_records, read_stream

cet = timezone(timedelta(hours=1))
request = QueryRequest(
    select=[MaLoSelector(malo="87253749908")],
    from_=datetime(2026, 1, 1, tzinfo=cet),
    to=datetime(2026, 2, 1, tzinfo=cet),
    interval="PT60M",
)
body = request.model_dump_json()  # {"select":[…],"from":"2026-01-01T00:00:00+01:00",…}

for record in read_stream(response_bytes):  # raises MissingEndError for a cut-off stream
    if isinstance(record, ValueRecord):
        print(record.from_, record.decimal)  # decimal.Decimal, no float rounding
```

`from` is a Python keyword, so the attribute is `from_`; the JSON key is always `from`.

## Fixtures

```
testdata/
  error_codes.json                       the known request- and record-level codes
  query_request/{valid,invalid}/         request bodies (DST days, +02:00 vs Z, naive timestamps, custom selectors, versions, …)
  resolve_request/, versions_request/
  records/{valid,invalid}/               single NDJSON records, one valid fixture per record-level error code
  streams/{valid,invalid}/               whole NDJSON responses (DST days with 92/100 quarter hours, gaps, versions, paging, …)
  resolve_response/, versions_response/
  error_envelope/{valid,invalid}/        one valid fixture per request-level error code
  system_info/
```

`X.canonical.json` next to `X.json` is the expected re-serialization when it differs from the input
(e.g. `+00:00` becomes `Z`, `"cursor": null` is omitted, unknown response keys are dropped).
Consumers in other languages (the Go service, future Rust/C#/TS models) should run the same fixtures.

`testdata/serialized.golden` pins the exact bytes both languages write for every valid fixture
(`<fixture>\t<JSON>` per line), so Go and Python output is byte-for-byte identical: compact JSON,
keys in model order, map keys (`params`, `attributes`) sorted, and Go's string escaping — `<`, `>`,
`&`, U+2028 and U+2029 are written as `<`, `>`, `&`, ` `, ` `; all other
non-ASCII characters are written as UTF-8. Python's `model_dump_json()` and `dump_record()` apply
that escaping; in Go, `json.Marshal`, `MarshalRecord` and a default `json.Encoder` produce it (an
encoder with `SetEscapeHTML(false)` would not). After an intended change, regenerate the file with
`go test -run Golden -update .` and check the diff.

Every invalid fixture has an entry in `testdata/invalid_reasons.json`: for each language a substring
its error message must contain (the field path and the failed rule), so a fixture cannot pass by
being rejected for the wrong reason.

## gRPC

`proto/zeitreiher/v1/*.proto` (package `zeitreiher.v1`, service `SeriesService` with `Query`,
`Resolve`, `ListVersions`, `System`) is the published gRPC contract. Field names equal the JSON keys.
Request timestamps are RFC 3339 strings (so the explicit-offset rule holds for gRPC too); response
timestamps are `google.protobuf.Timestamp`. Conformance with the JSON contract is defined through
the hand-written models (fixture → model → proto → model → JSON), not through protojson. Generate
code in the consuming project, e.g. with buf managed mode.

## Versioning

`v1` is in the URL path (`/api/v1/…`), the proto package (`zeitreiher.v1`) and the models' major
version. Additive changes (new optional fields, new error codes) stay in v1; breaking changes need v2
(Go: module path `…/v2`).

## Development

```bash
go test ./...                      # Go
golangci-lint run && golangci-lint fmt --diff
uv sync --group dev                # Python
uv run pytest unittests
uv run ruff check src unittests && uv run ruff format --check .
uv run mypy --strict src/zeitreiher_models unittests
buf lint && buf build && buf format --diff --exit-code   # buf v1.75.0
```

The Go tests (`fixtures_test.go`) and the Python tests (`unittests/test_fixtures.py`) use the same
table of fixture kinds; a new fixture directory must be added to both. `unittests/test_proto_contract.py`
fails when a model and its proto message disagree on field names.

## License

MIT
