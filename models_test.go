package zeitreihermodels_test

import (
	"bytes"
	"encoding/json"
	"errors"
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
		zrm.CurrentVersion(), zrm.VersionNumber(1), zrm.VersionAsOf(time.Date(2026, 2, 1, 0, 0, 0, 0, cet)),
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
		"profile leading zeros": &zrm.SeriesRecord{Series: 1, Select: zrm.MaLoSelector("87253749908"), Profile: "056", From: from, To: from.Add(time.Hour), Unit: "KWH", Interval: "PT15M"},
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
		Unit: "KWH", Interval: "PT15M",
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
		Unit: "KWH", Interval: "PT15M", Attributes: attributes,
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
	require.NoError(t, json.Unmarshal([]byte(`"-12.50"`), &d))
	assert.Equal(t, zrm.Decimal("-12.50"), d)
	assert.Error(t, json.Unmarshal([]byte(`12.5`), &d))
	assert.Error(t, zrm.Decimal("-0.0").Validate())
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
