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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInstant(json.RawMessage(tc.raw), requestInstant)
			if tc.request {
				require.NoError(t, err)
				assert.Equal(t, tc.want, formatInstant(got))
			} else {
				require.Error(t, err)
			}
			_, err = parseInstant(json.RawMessage(tc.raw), responseInstant)
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
	assert.Error(t, validateInstant(time.Time{}, requestInstant), "zero time means missing")
	assert.Error(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC), requestInstant))
	assert.NoError(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), requestInstant))
	assert.Error(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, cet), responseInstant))
	assert.NoError(t, validateInstant(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), responseInstant))
}

func TestValidateWindowComparesInstantsAcrossOffsets(t *testing.T) {
	from := time.Date(2026, 1, 1, 1, 0, 0, 0, time.FixedZone("", 3600)) // = 00:00Z
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Error(t, validateWindow(from, to, requestInstant), "equal instants are an empty window")
	assert.NoError(t, validateWindow(from, to.Add(time.Hour), requestInstant))
	assert.Error(t, validateWindow(to.Add(time.Hour), from, requestInstant))
}
