package zeitreihermodels_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	zrm "github.com/hochfrequenz/zeitreiher-models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cet = time.FixedZone("", 3600)

func TestQueryRequestFromGoValuesKeepsOffsetsAndOmitsUnsetFields(t *testing.T) {
	v := zrm.VersionNumber(3)
	req := zrm.QueryRequest{
		Select:  []zrm.Selector{zrm.MaLoSelector("87253749908"), zrm.ProfileSelector("50000000000056")},
		From:    time.Date(2026, 1, 1, 0, 0, 0, 0, cet),
		To:      time.Date(2026, 2, 1, 0, 0, 0, 0, cet),
		Version: &v,
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"select": [{"by":"malo","malo":"87253749908"},{"by":"profile","profile":"50000000000056"}],
		"from": "2026-01-01T00:00:00+01:00",
		"to": "2026-02-01T00:00:00+01:00",
		"version": {"number": 3}
	}`, string(b))
}

func TestVersionSelectorForms(t *testing.T) {
	for _, v := range []zrm.VersionSelector{
		zrm.CurrentVersion(), zrm.VersionNumber(1),
	} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var back zrm.VersionSelector
		require.NoError(t, json.Unmarshal(b, &back))
		again, err := json.Marshal(back)
		require.NoError(t, err)
		assert.Equal(t, string(b), string(again))
	}
	_, err := json.Marshal(zrm.VersionSelector{})
	assert.Error(t, err, "the zero VersionSelector is invalid")

	var v zrm.VersionSelector
	assert.ErrorContains(t, json.Unmarshal([]byte(`{"as_of":"2026-02-03T08:00:00Z"}`), &v), `unknown key(s) ["as_of"]`,
		"as-of is not part of v1")
}

func TestInvalidValuesRefuseToMarshal(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]any{
		"empty select":          zrm.QueryRequest{From: from, To: from.Add(time.Hour)},
		"reversed window":       zrm.QueryRequest{Select: []zrm.Selector{zrm.ProfileSelector("56")}, From: from, To: from},
		"sub-second":            zrm.QueryRequest{Select: []zrm.Selector{zrm.ProfileSelector("56")}, From: from.Add(time.Millisecond), To: from.Add(time.Hour)},
		"custom with known by":  zrm.CustomSelector("malo", nil),
		"bad decimal":           &zrm.ValueRecord{Series: 1, From: from, To: from.Add(time.Hour), Value: "1e3"},
		"unsorted status":       &zrm.ValueRecord{Series: 1, From: from, To: from.Add(time.Hour), Value: "1", Status: []string{"B", "A"}},
		"inconsistent version":  zrm.Version{Number: 1, From: from, CreatedAtUnknown: true, CreatedAt: &from},
		"code not snake case":   &zrm.ErrorRecord{Select: zrm.MaLoSelector("87253749908"), Code: "Nope"},
		"profile leading zeros": &zrm.SeriesRecord{Series: 1, Select: zrm.MaLoSelector("87253749908"), Profile: "056", From: from, To: from.Add(time.Hour), Unit: ptr("KWH"), UnitRaw: "KWH", Interval: "PT15M"},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := json.Marshal(v)
			assert.Error(t, err)
		})
	}
}

func TestResponseInstantsAreWrittenInUTC(t *testing.T) {
	rec := &zrm.ValueRecord{
		Series: 1, From: time.Date(2026, 1, 1, 0, 0, 0, 0, cet), To: time.Date(2026, 1, 1, 1, 0, 0, 0, cet),
		Value: "0.022", Status: nil,
	}
	b, err := zrm.MarshalRecord(rec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"value","series":1,"from":"2025-12-31T23:00:00Z","to":"2026-01-01T00:00:00Z",
		"value":"0.022","status":[],"version":null}`, string(b))
	assert.True(t, strings.HasPrefix(string(b), `{"type":"value"`), "type is written first")
}

func TestSeriesRecordWritesEmptyCollections(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b, err := zrm.MarshalRecord(&zrm.SeriesRecord{
		Series: 1, Select: zrm.ProfileSelector("56"), Profile: "56", From: from, To: from.Add(time.Hour),
		Unit: ptr("KWH"), UnitRaw: "KWH", Interval: "PT15M",
	})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"attributes":{}`)
	assert.Contains(t, string(b), `"versions":[]`)
}

func TestErrorCodesKnownAndUnknown(t *testing.T) {
	rec, err := zrm.ParseRecord([]byte(`{"type":"error","selector":0,"select":{"by":"malo","malo":"87253749908"},"code":"brand_new"}`))
	require.NoError(t, err)
	code := rec.(*zrm.ErrorRecord).Code
	assert.Equal(t, zrm.RecordErrorCode("brand_new"), code)
	assert.False(t, code.IsKnown())
	assert.True(t, zrm.RecordErrorProfileNotFound.IsKnown())

	// Strict on write: an unknown code is read, but never written.
	_, err = zrm.MarshalRecord(rec)
	assert.ErrorContains(t, err, `"brand_new" is not a public error code`)
	_, err = json.Marshal(zrm.ResultError{Code: "brand_new"})
	assert.ErrorContains(t, err, `"brand_new" is not a public error code`)
	_, err = json.Marshal(zrm.ErrorEnvelope{Error: zrm.ErrorDetail{Code: "brand_new", Message: "m"}})
	assert.ErrorContains(t, err, `"brand_new" is not a public error code`)
	for _, c := range zrm.AllRecordErrorCodes() {
		_, err = json.Marshal(zrm.ResultError{Code: c})
		assert.NoError(t, err, c)
	}
	for _, c := range zrm.AllRequestErrorCodes() {
		_, err = json.Marshal(zrm.ErrorEnvelope{Error: zrm.ErrorDetail{Code: c, Message: "m"}})
		assert.NoError(t, err, c)
	}
}

func TestErrorEnvelope(t *testing.T) {
	var env zrm.ErrorEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"error":{"code":"cursor_stale","message":"assignments changed"}}`), &env))
	assert.Equal(t, zrm.RequestErrorCursorStale, env.Error.Code)
	b, err := json.Marshal(env)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "request_id")
}

func TestAssignmentWithErrorOnly(t *testing.T) {
	b, err := json.Marshal(zrm.Assignment{Profile: "50000000000057", Error: &zrm.ResultError{Code: zrm.RecordErrorAccessDenied}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"profile":"50000000000057","error":{"code":"access_denied"}}`, string(b))
}

func TestResultWritesEmptyAssignments(t *testing.T) {
	b, err := json.Marshal(zrm.ResolveResult{Select: zrm.CustomSelector("pod", nil)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"selector":0,"select":{"by":"pod","params":{}},"assignments":[]}`, string(b))
}

func TestReadRecordsFraming(t *testing.T) {
	end := `{"type":"end","series":0,"values":0,"errors":0,"next":null}`
	records, err := zrm.ReadStream(strings.NewReader(end))
	require.NoError(t, err, "the final newline is optional")
	assert.Len(t, records, 1)

	_, err = zrm.ReadStream(strings.NewReader(end + "\n\n"))
	assert.ErrorContains(t, err, "blank line")

	_, err = zrm.ReadStream(strings.NewReader(end + "\r\n"))
	assert.ErrorContains(t, err, "carriage return")

	_, err = zrm.ReadStream(strings.NewReader(""))
	assert.True(t, errors.Is(err, zrm.ErrMissingEnd))
}

func TestReadRecordsHandlesLongLines(t *testing.T) {
	attributes := map[string]string{"blob": strings.Repeat("x", 1<<20)}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	require.NoError(t, zrm.WriteRecord(&buf, &zrm.SeriesRecord{
		Series: 1, Select: zrm.ProfileSelector("56"), Profile: "56", From: from, To: from.Add(time.Hour),
		Unit: ptr("KWH"), UnitRaw: "KWH", Interval: "PT15M", Attributes: attributes,
	}))
	require.NoError(t, zrm.WriteRecord(&buf, &zrm.EndRecord{Series: 1}))
	records, err := zrm.ReadStream(&buf)
	require.NoError(t, err)
	assert.Len(t, records[0].(*zrm.SeriesRecord).Attributes["blob"], 1<<20)
}

func TestReadRecordsStopsEarly(t *testing.T) {
	end := `{"type":"end","series":0,"values":0,"errors":0,"next":null}` + "\n"
	n := 0
	for _, err := range zrm.ReadRecords(strings.NewReader(end + end + end)) {
		require.NoError(t, err)
		n++
		break
	}
	assert.Equal(t, 1, n)
}

func TestDecimal(t *testing.T) {
	var d zrm.Decimal
	require.NoError(t, json.Unmarshal([]byte(`"-12.5"`), &d))
	assert.Equal(t, zrm.Decimal("-12.5"), d)
	assert.Error(t, json.Unmarshal([]byte(`12.5`), &d))
	assert.Error(t, zrm.Decimal("-0.0").Validate())
	for _, ok := range []zrm.Decimal{"160", "0.022", "25.25", "-3.5", "0", "100", "-10"} {
		assert.NoError(t, ok.Validate(), ok)
	}
	// Responses carry the shortest canonical form: after parsing, 160.0 and 160 are the same number.
	for bad, want := range map[zrm.Decimal]string{"160.0": "160", "0.0220": "0.022", "0.0": "0", "-3.50": "-3.5"} {
		assert.ErrorContains(t, bad.Validate(), fmt.Sprintf("write %q", want), bad)
		_, err := json.Marshal(bad)
		assert.Error(t, err, "%s must not be written", bad)
	}
}

func TestNilRecordsRefuseToMarshal(t *testing.T) {
	for _, rec := range []zrm.Record{nil, (*zrm.SeriesRecord)(nil), (*zrm.ValueRecord)(nil), (*zrm.ErrorRecord)(nil), (*zrm.EndRecord)(nil)} {
		_, err := zrm.MarshalRecord(rec)
		require.Error(t, err, "%T", rec)
		var buf bytes.Buffer
		require.Error(t, zrm.WriteRecord(&buf, rec), "%T", rec)
		assert.Empty(t, buf.String(), "nothing may be written for %T", rec)
	}
}

// TestInvalidUTF8RefusesToMarshal: encoding/json would replace invalid UTF-8 with U+FFFD, so writers
// must refuse such text instead of silently changing it. Python refuses lone surrogates likewise.
func TestInvalidUTF8RefusesToMarshal(t *testing.T) {
	const bad = "a\xffb"
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	window := func(sel zrm.Selector) zrm.QueryRequest {
		return zrm.QueryRequest{Select: []zrm.Selector{sel}, From: from, To: to}
	}
	series := func(edit func(*zrm.SeriesRecord)) *zrm.SeriesRecord {
		r := &zrm.SeriesRecord{
			Series: 1, Select: zrm.ProfileSelector("56"), Profile: "56", From: from, To: to,
			Unit: ptr("KWH"), UnitRaw: "KWH", Interval: "PT15M",
		}
		edit(r)
		return r
	}
	version := func(edit func(*zrm.Version)) *zrm.SeriesRecord {
		v := zrm.Version{Number: 1, From: from, CreatedAtUnknown: true}
		edit(&v)
		return series(func(r *zrm.SeriesRecord) { r.Versions = []zrm.Version{v} })
	}
	system := func(edit func(*zrm.SystemInfo)) zrm.SystemInfo {
		s := zrm.SystemInfo{
			ServiceVersion: "1", AbapPackageVersion: "1", SID: "HF1", Client: "100", Release: "758",
			BusinessTimeZone: "CET",
			Caps:             zrm.Caps{MaxWindowSeconds: 1, MaxValues: 1, MaxResolveWindowSeconds: 1},
		}
		edit(&s)
		return s
	}
	badString := bad
	cases := map[string]any{
		"selector param value": zrm.CustomSelector("x", map[string]string{"k": bad}),
		"selector param key":   zrm.CustomSelector("x", map[string]string{bad: "v"}),
		"request param value":  window(zrm.CustomSelector("x", map[string]string{"k": bad})),
		"request unit":         func() zrm.QueryRequest { q := window(zrm.ProfileSelector("56")); q.Unit = "K\xff"; return q }(),
		"request cursor":       func() zrm.QueryRequest { q := window(zrm.ProfileSelector("56")); q.Cursor = bad; return q }(),
		"resolve param value": zrm.ResolveRequest{
			Select: []zrm.Selector{zrm.CustomSelector("x", map[string]string{"k": bad})}, From: from, To: to,
		},
		"series unit_raw":        series(func(r *zrm.SeriesRecord) { r.UnitRaw = "K\xff" }),
		"series role":            series(func(r *zrm.SeriesRecord) { r.Role = ptr(bad) }),
		"series attribute value": series(func(r *zrm.SeriesRecord) { r.Attributes = map[string]string{"k": bad} }),
		"series attribute key":   series(func(r *zrm.SeriesRecord) { r.Attributes = map[string]string{bad: "v"} }),
		"series echoed param": series(func(r *zrm.SeriesRecord) {
			r.Select = zrm.CustomSelector("x", map[string]string{"k": bad})
		}),
		"version reason":      version(func(v *zrm.Version) { v.Reason = bad }),
		"version description": version(func(v *zrm.Version) { v.Description = bad }),
		"value status": &zrm.ValueRecord{
			Series: 1, From: from, To: to, Value: "1", Status: []string{bad},
		},
		"end next": &zrm.EndRecord{Next: &badString},
		"error message": zrm.ErrorEnvelope{Error: zrm.ErrorDetail{
			Code: zrm.RequestErrorInvalidRequest, Message: bad,
		}},
		"error request id": zrm.ErrorEnvelope{Error: zrm.ErrorDetail{
			Code: zrm.RequestErrorInvalidRequest, Message: "m", RequestID: bad,
		}},
		"system sid":     system(func(s *zrm.SystemInfo) { s.SID = bad }),
		"system release": system(func(s *zrm.SystemInfo) { s.Release = bad }),
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			if val, ok := v.(interface{ Validate() error }); ok {
				assert.ErrorContains(t, val.Validate(), "not valid UTF-8")
			}
			_, err := json.Marshal(v)
			assert.ErrorContains(t, err, "not valid UTF-8")
		})
	}
}

func ptr[T any](v T) *T { return &v }

// TestSeriesUnits: unit is the canonical UN/ECE Rec. 20 code or null when the SAP unit is not mapped;
// unit_raw always carries the SAP unit key.
func TestSeriesUnits(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	series := func(unit *string, raw string) *zrm.SeriesRecord {
		return &zrm.SeriesRecord{
			Series: 1, Select: zrm.ProfileSelector("56"), Profile: "56", From: from, To: from.Add(time.Hour),
			Unit: unit, UnitRaw: raw, Interval: "PT15M",
		}
	}
	b, err := zrm.MarshalRecord(series(nil, "µM"))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"unit":null,"unit_raw":"µM"`)
	back, err := zrm.ParseRecord(b)
	require.NoError(t, err)
	assert.Nil(t, back.(*zrm.SeriesRecord).Unit)

	b, err = zrm.MarshalRecord(series(ptr("K3"), "kvarh"))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"unit":"K3","unit_raw":"kvarh"`)

	for _, bad := range []string{"kWh", "", "KWHH", "K", "°C"} {
		_, err = zrm.MarshalRecord(series(ptr(bad), "KWH"))
		assert.ErrorContains(t, err, "is not a unit code", bad)
	}
	_, err = zrm.MarshalRecord(series(ptr("KWH"), ""))
	assert.ErrorContains(t, err, "unit_raw")
}

// TestSeriesRole: every series record carries the role of its profile assignment, or null when the
// series was not resolved through a role (e.g. by: profile).
func TestSeriesRole(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	series := func(role *string) *zrm.SeriesRecord {
		return &zrm.SeriesRecord{
			Series: 1, Select: zrm.MaLoSelector("87253749908"), Profile: "56", Role: role, From: from,
			To: from.Add(time.Hour), Unit: ptr("KWH"), UnitRaw: "KWH", Interval: "PT15M",
		}
	}
	b, err := zrm.MarshalRecord(series(ptr("0001")))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"profile":"56","role":"0001","from"`)
	back, err := zrm.ParseRecord(b)
	require.NoError(t, err)
	assert.Equal(t, "0001", *back.(*zrm.SeriesRecord).Role)

	b, err = zrm.MarshalRecord(series(nil))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"role":null`)

	_, err = zrm.MarshalRecord(series(ptr("")))
	assert.ErrorContains(t, err, "role: must not be empty")
}
