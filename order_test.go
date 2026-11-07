package zeitreihermodels_test

import (
	"encoding/json"
	"testing"

	zrm "github.com/hochfrequenz/zeitreiher-models"
	"github.com/stretchr/testify/require"
)

// With several problems at once, the first reported one must not depend on map iteration order.
func TestFirstErrorIsDeterministic(t *testing.T) {
	const window = `"select":[{"by":"profile","profile":"50000000000056"}],"from":"2026-01-01T00:00:00+01:00","to":"2026-01-01T01:00:00+01:00"`
	cases := map[string]struct {
		run  func() error
		want string
	}{
		"query request, empty interval and unit": {func() error {
			return json.Unmarshal([]byte(`{`+window+`,"interval":"","unit":""}`), new(zrm.QueryRequest))
		}, "query request: interval: must not be empty"},
		"custom selector, two null params": {func() error {
			return json.Unmarshal([]byte(`{"by":"pod","params":{"a":null,"b":null}}`), new(zrm.Selector))
		}, "selector \"pod\": params: a: must be a string, not null"},
		"end record, several negative counts": {func() error {
			return (&zrm.EndRecord{Series: -1, Values: -1, Errors: -1}).Validate()
		}, "errors: must be at least 0, got -1"},
		"end record decoded, several wrong types": {func() error {
			_, err := zrm.ParseRecord([]byte(`{"type":"end","series":"x","values":"x","errors":"x","next":null}`))
			return err
		}, "end record: errors: json: cannot unmarshal string into Go value of type int"},
		"caps, all zero": {func() error { return zrm.Caps{}.Validate() }, "max_resolve_window_seconds: must be at least 1"},
		"caps decoded": {func() error {
			return json.Unmarshal([]byte(`{"max_window_seconds":"x","max_values":"x","max_resolve_window_seconds":"x"}`), new(zrm.Caps))
		}, "max_resolve_window_seconds: json: cannot unmarshal string into Go value of type int64"},
		"system info, empty": {func() error { return zrm.SystemInfo{}.Validate() }, "abap_package_version: must not be empty"},
		"system info decoded": {func() error {
			return json.Unmarshal([]byte(`{"service_version":1,"abap_package_version":1,"sid":1,"client":1,"release":1,"business_time_zone":1,"intervals":[],"units":[],"caps":{}}`), new(zrm.SystemInfo))
		}, "system info: abap_package_version: json: cannot unmarshal number into Go value of type string"},
		"error detail, wrong types": {func() error {
			return json.Unmarshal([]byte(`{"code":1,"message":2,"request_id":3}`), new(zrm.ErrorDetail))
		}, "code: must be a string"},
		"error record, wrong types": {func() error {
			_, err := zrm.ParseRecord([]byte(`{"type":"error","selector":"x","select":1,"profile":2,"code":3}`))
			return err
		}, "error record: code: must be a string"},
		"value record, wrong types": {func() error {
			_, err := zrm.ParseRecord([]byte(`{"type":"value","series":"x","value":"x","status":"x","version":"x","from":"2026-01-01T00:00:00+01:00","to":"2026-01-01T00:15:00+01:00"}`))
			return err
		}, "value record: series: json: cannot unmarshal string into Go value of type int"},
		"series record, wrong types": {func() error {
			_, err := zrm.ParseRecord([]byte(`{"type":"series","series":"x","selector":"x","profile":1,"interval":1,"versions":1,"attributes":{},"select":1,"role":null,"unit":null,"unit_raw":1,"from":"2026-01-01T00:00:00+01:00","to":"2026-01-01T00:15:00+01:00"}`))
			return err
		}, "series record: interval: json: cannot unmarshal number into Go value of type string"},
		"version, wrong types": {func() error {
			return json.Unmarshal([]byte(`{"number":"x","created_at":"2026-01-01T00:00:00Z","reason":1,"description":1,"from":"2026-01-01T00:00:00Z","to":null,"deleted":"x","created_at_unknown":1}`), new(zrm.Version))
		}, "version: created_at_unknown: json: cannot unmarshal number into Go value of type bool"},
		"assignment, wrong types": {func() error {
			return json.Unmarshal([]byte(`{"role":1,"unit":1,"unit_raw":1,"profile":"x"}`), new(zrm.Assignment))
		}, "assignment: role: json: cannot unmarshal number into Go value of type string"},
		"result, wrong types": {func() error {
			return json.Unmarshal([]byte(`{"selector":"x","select":1,"assignments":1}`), new(zrm.ResolveResult))
		}, "result: assignments: json: cannot unmarshal number into Go value of type []zeitreihermodels.Assignment"},
		"checkObject-based, nothing given": {func() error {
			return json.Unmarshal([]byte(`{}`), new(zrm.QueryRequest))
		}, "query request: from: is required"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			for range 50 {
				err := c.run()
				require.Error(t, err)
				require.EqualError(t, err, c.want)
			}
		})
	}
}
