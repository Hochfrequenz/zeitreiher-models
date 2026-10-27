package zeitreihermodels

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInstant(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		request  bool // accepted as a request instant
		response bool // accepted as a response instant
		want     string
	}{
		{"utc z", `"2026-01-01T00:00:00Z"`, true, true, "2026-01-01T00:00:00Z"},
		{"plus 0200 keeps offset", `"2026-10-24T00:00:00+02:00"`, true, false, "2026-10-24T00:00:00+02:00"},
		{"minus 0500", `"2026-01-01T00:00:00-05:00"`, true, false, "2026-01-01T00:00:00-05:00"},
		{"plus 0000 becomes z", `"2026-01-01T00:00:00+00:00"`, true, false, "2026-01-01T00:00:00Z"},
		{"naive", `"2026-01-01T00:00:00"`, false, false, ""},
		{"minus 0000 unknown offset", `"2026-01-01T00:00:00-00:00"`, false, false, ""},
		{"fractional seconds", `"2026-01-01T00:00:00.5Z"`, false, false, ""},
		{"lowercase t", `"2026-01-01t00:00:00Z"`, false, false, ""},
		{"lowercase z", `"2026-01-01T00:00:00z"`, false, false, ""},
		{"space separator", `"2026-01-01 00:00:00Z"`, false, false, ""},
		{"compact offset", `"2026-01-01T00:00:00+0200"`, false, false, ""},
		{"date only", `"2026-01-01"`, false, false, ""},
		{"no such day", `"2026-02-30T00:00:00Z"`, false, false, ""},
		{"hour 24", `"2026-01-01T24:00:00Z"`, false, false, ""},
		{"json number", `1767225600`, false, false, ""},
		{"json null", `null`, false, false, ""},
		{"offset hour 24", `"2026-01-01T00:00:00+24:00"`, false, false, ""},
		{"offset minute 60", `"2026-01-01T00:00:00+01:60"`, false, false, ""},
		{"year 0000", `"0000-01-01T00:00:00Z"`, false, false, ""},
		{"leap second", `"2026-12-31T23:59:60Z"`, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInstant(json.RawMessage(tc.raw), requestSide)
			if tc.request {
				require.NoError(t, err)
				assert.Equal(t, tc.want, formatInstant(got, requestSide))
			} else {
				require.Error(t, err)
			}
			_, err = parseInstant(json.RawMessage(tc.raw), responseSide)
			if tc.response {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidateInstant(t *testing.T) {
	cet := time.FixedZone("", 3600)
	assert.Error(t, validateInstant(time.Time{}, requestSide), "zero time means missing")
	assert.Error(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC), requestSide))
	assert.NoError(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), requestSide))
	assert.NoError(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), responseSide))
	assert.Equal(t, "2025-12-31T23:00:00Z", formatInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), responseSide),
		"producers normalize response instants to UTC")
	assert.Equal(t, "2026-01-01T00:00:00+01:00", formatInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), requestSide))
}

func TestValidateWindowComparesInstantsAcrossOffsets(t *testing.T) {
	from := time.Date(2026, 1, 1, 1, 0, 0, 0, time.FixedZone("", 3600)) // = 00:00Z
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Error(t, validateWindow(from, to, requestSide), "equal instants are an empty window")
	assert.NoError(t, validateWindow(from, to.Add(time.Hour), requestSide))
	assert.Error(t, validateWindow(to.Add(time.Hour), from, requestSide))
}

func TestValidateInstantRequiresARoundTrippableWireForm(t *testing.T) {
	subMinute := time.FixedZone("", 30)
	// RFC 3339 drops offset seconds, so the written instant would move by 30 s.
	assert.Error(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, subMinute), requestSide))
	// In UTC the same instant is representable.
	assert.NoError(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, subMinute), responseSide))
	// A five-digit year does not match the wire grammar.
	for _, kind := range []side{requestSide, responseSide} {
		assert.Error(t, validateInstant(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), kind))
		assert.NoError(t, validateInstant(time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), kind))
	}
	// A request window built from such values refuses to marshal instead of writing a different instant.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, subMinute)
	assert.Error(t, validateWindow(from, from.Add(time.Hour), requestSide))
}
