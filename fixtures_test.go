package zeitreihermodels_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	zrm "github.com/hochfrequenz/zeitreiher-models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codec parses and re-serializes one fixture kind.
type codec struct {
	parse func([]byte) (any, error)
	dump  func(any) ([]byte, error)
}

func jsonCodec[T any]() codec {
	return codec{
		parse: func(b []byte) (any, error) {
			v := new(T)
			err := json.Unmarshal(b, v)
			return v, err
		},
		dump: func(v any) ([]byte, error) { return json.Marshal(v) },
	}
}

// kinds maps every directory under testdata/ (except streams) to the type it holds. The Python
// tests use the same table (unittests/test_fixtures.py).
var kinds = map[string]codec{
	"query_request":     jsonCodec[zrm.QueryRequest](),
	"resolve_request":   jsonCodec[zrm.ResolveRequest](),
	"versions_request":  jsonCodec[zrm.VersionsRequest](),
	"records":           {parse: func(b []byte) (any, error) { return zrm.ParseRecord(b) }, dump: func(v any) ([]byte, error) { return zrm.MarshalRecord(v.(zrm.Record)) }},
	"resolve_response":  jsonCodec[zrm.ResolveResponse](),
	"versions_response": jsonCodec[zrm.VersionsResponse](),
	"error_envelope":    jsonCodec[zrm.ErrorEnvelope](),
	"system_info":       jsonCodec[zrm.SystemInfo](),
}

const testdata = "testdata"

func fixtureFiles(t *testing.T, kind, validity, ext string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(testdata, kind, validity, "*"+ext))
	require.NoError(t, err)
	files = slices.DeleteFunc(files, func(f string) bool { return strings.HasSuffix(f, ".canonical.json") })
	require.NotEmpty(t, files, "no fixtures in %s/%s", kind, validity)
	return files
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func TestFixtureDirectoriesAreAllCovered(t *testing.T) {
	entries, err := os.ReadDir(testdata)
	require.NoError(t, err)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	want := []string{"streams"}
	for k := range kinds {
		want = append(want, k)
	}
	assert.ElementsMatch(t, want, dirs, "every testdata directory needs an entry in the kind table and vice versa")
}

func TestValidFixturesRoundTrip(t *testing.T) {
	for kind, c := range kinds {
		for _, path := range fixtureFiles(t, kind, "valid", ".json") {
			t.Run(kind+"/"+filepath.Base(path), func(t *testing.T) {
				input := read(t, path)
				expected := input
				if canonical := strings.TrimSuffix(path, ".json") + ".canonical.json"; fileExists(canonical) {
					expected = read(t, canonical)
				}
				v, err := c.parse(input)
				require.NoError(t, err)
				out, err := c.dump(v)
				require.NoError(t, err)
				assert.JSONEq(t, string(expected), string(out))

				again, err := c.parse(out)
				require.NoError(t, err)
				out2, err := c.dump(again)
				require.NoError(t, err)
				assert.Equal(t, string(out), string(out2), "serialization must be idempotent")
			})
		}
	}
}

// invalidReason says why an invalid fixture must be rejected. Go is a substring of the Go error; the
// Python entry (pydantic loc, type and, for the models' own errors, a message substring) is checked
// by unittests/test_fixtures.py.
type invalidReason struct {
	Go     string          `json:"go"`
	Python json.RawMessage `json:"python"`
}

// invalidReasons maps every invalid fixture to why each language must reject it, so that a fixture
// cannot pass by being rejected for the wrong reason.
func invalidReasons(t *testing.T) map[string]invalidReason {
	t.Helper()
	var reasons map[string]invalidReason
	require.NoError(t, json.Unmarshal(read(t, filepath.Join(testdata, "invalid_reasons.json")), &reasons))
	return reasons
}

func TestInvalidFixturesAreRejected(t *testing.T) {
	reasons := invalidReasons(t)
	seen := map[string]bool{}
	for kind, c := range kinds {
		for _, path := range fixtureFiles(t, kind, "invalid", ".json") {
			name := kind + "/" + filepath.Base(path)
			seen[name] = true
			t.Run(name, func(t *testing.T) {
				_, err := c.parse(read(t, path))
				require.Error(t, err)
				reason, ok := reasons[name]
				require.True(t, ok, "add %s to testdata/invalid_reasons.json; the Go error is: %v", name, err)
				require.NotEmpty(t, reason.Go)
				require.NotEmpty(t, reason.Python)
				assert.Contains(t, err.Error(), reason.Go)
			})
		}
	}
	for name := range reasons {
		assert.True(t, seen[name], "invalid_reasons.json lists %s, which is not an invalid fixture", name)
	}
}

func TestValidStreams(t *testing.T) {
	for _, path := range fixtureFiles(t, "streams", "valid", ".ndjson") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			input := read(t, path)
			records, err := zrm.ReadStream(bytes.NewReader(input))
			require.NoError(t, err)

			lines := strings.Split(strings.TrimSuffix(string(input), "\n"), "\n")
			require.Len(t, records, len(lines))
			var buf bytes.Buffer
			for i, rec := range records {
				out, err := zrm.MarshalRecord(rec)
				require.NoError(t, err)
				assert.JSONEq(t, lines[i], string(out), "line %d", i+1)
				require.NoError(t, zrm.WriteRecord(&buf, rec))
			}
			again, err := zrm.ReadStream(&buf)
			require.NoError(t, err)
			assert.Len(t, again, len(records))

			assertEndCounts(t, records)
		})
	}
}

// assertEndCounts checks that the end record counts the records of its page.
func assertEndCounts(t *testing.T, records []zrm.Record) {
	t.Helper()
	counts := map[string]int{}
	for _, rec := range records {
		counts[rec.RecordType()]++
	}
	end, ok := records[len(records)-1].(*zrm.EndRecord)
	require.True(t, ok)
	assert.Equal(t, counts[zrm.RecordTypeSeries], end.Series, "end.series")
	assert.Equal(t, counts[zrm.RecordTypeValue], end.Values, "end.values")
	assert.Equal(t, counts[zrm.RecordTypeError], end.Errors, "end.errors")
}

func TestInvalidStreamsAreRejected(t *testing.T) {
	for _, path := range fixtureFiles(t, "streams", "invalid", ".ndjson") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := zrm.ReadStream(bytes.NewReader(read(t, path)))
			assert.Error(t, err)
		})
	}
	_, err := zrm.ReadStream(bytes.NewReader(read(t, filepath.Join(testdata, "streams", "invalid", "missing_end.ndjson"))))
	assert.True(t, errors.Is(err, zrm.ErrMissingEnd))
}

func streamValues(t *testing.T, name string) (*zrm.SeriesRecord, []*zrm.ValueRecord) {
	t.Helper()
	records, err := zrm.ReadStream(bytes.NewReader(read(t, filepath.Join(testdata, "streams", "valid", name))))
	require.NoError(t, err)
	series, ok := records[0].(*zrm.SeriesRecord)
	require.True(t, ok)
	var values []*zrm.ValueRecord
	for _, rec := range records {
		if v, ok := rec.(*zrm.ValueRecord); ok {
			values = append(values, v)
		}
	}
	return series, values
}

func TestDSTStreamsCoverTheLocalDay(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	for name, n := range map[string]int{"dst_spring_day_pt15m.ndjson": 92, "dst_autumn_day_pt15m.ndjson": 100} {
		t.Run(name, func(t *testing.T) {
			series, values := streamValues(t, name)
			require.Len(t, values, n)
			assert.Equal(t, series.From, values[0].From)
			assert.Equal(t, series.To, values[len(values)-1].To)
			for i, v := range values {
				assert.Equal(t, 15*time.Minute, v.To.Sub(v.From))
				if i > 0 {
					assert.Equal(t, values[i-1].To, v.From, "values must be contiguous")
				}
			}
			from := series.From.In(berlin)
			assert.Equal(t, 0, from.Hour(), "series starts at local midnight")
			assert.Equal(t, from.AddDate(0, 0, 1), series.To.In(berlin), "series ends at the next local midnight")
		})
	}
}

func TestGapsStayGaps(t *testing.T) {
	series, values := streamValues(t, "gaps.ndjson")
	holes := 0
	for i := 1; i < len(values); i++ {
		if !values[i-1].To.Equal(values[i].From) {
			holes++
		}
	}
	assert.Equal(t, 1, holes, "one interior gap")
	assert.True(t, values[len(values)-1].To.Before(series.To), "tail gap: no values up to the series end")
}

func TestVersionStreams(t *testing.T) {
	_, values := streamValues(t, "version_number.ndjson")
	for _, v := range values {
		require.NotNil(t, v.Version)
		assert.Equal(t, 3, *v.Version)
	}
	_, values = streamValues(t, "version_as_of_mixed.ndjson")
	var got []any
	for _, v := range values {
		if v.Version == nil {
			got = append(got, nil)
		} else {
			got = append(got, *v.Version)
		}
	}
	assert.Equal(t, []any{nil, 3, 4, nil}, got)
}

func TestPagedStreamResendsSeries(t *testing.T) {
	s1, _ := streamValues(t, "paged_page1.ndjson")
	s2, _ := streamValues(t, "paged_page2.ndjson")
	assert.Equal(t, s1, s2, "the series record of a continuing series is re-sent with the same number")
}

func TestErrorCodesMatchFixture(t *testing.T) {
	var codes struct {
		Request []string `json:"request"`
		Record  []string `json:"record"`
	}
	require.NoError(t, json.Unmarshal(read(t, filepath.Join(testdata, "error_codes.json")), &codes))

	var request, record []string
	for _, c := range zrm.AllRequestErrorCodes() {
		request = append(request, string(c))
		assert.True(t, c.IsKnown())
		assert.FileExists(t, filepath.Join(testdata, "error_envelope", "valid", string(c)+".json"))
	}
	for _, c := range zrm.AllRecordErrorCodes() {
		record = append(record, string(c))
		assert.True(t, c.IsKnown())
		assert.FileExists(t, filepath.Join(testdata, "records", "valid", "error_"+string(c)+".json"))
	}
	assert.Equal(t, codes.Request, request)
	assert.Equal(t, codes.Record, record)
	assert.False(t, zrm.RecordErrorCode("some_future_code").IsKnown())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
