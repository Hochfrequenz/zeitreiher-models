package zeitreihermodels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckObject(t *testing.T) {
	r := rules{"a": required, "b": requiredNullable, "c": optional}

	m, err := checkObject([]byte(`{"a":1,"b":null,"c":null}`), r, rejectUnknown)
	require.NoError(t, err)
	assert.Contains(t, m, "b", "a required-nullable null is kept")
	assert.NotContains(t, m, "c", "an optional null counts as missing")

	_, err = checkObject([]byte(`{"b":1}`), r, rejectUnknown)
	assert.ErrorContains(t, err, "a: is required")

	_, err = checkObject([]byte(`{"a":null,"b":1}`), r, rejectUnknown)
	assert.ErrorContains(t, err, "a: must not be null")

	_, err = checkObject([]byte(`{"a":1}`), r, rejectUnknown)
	assert.ErrorContains(t, err, "b: is required")

	_, err = checkObject([]byte(`{"a":1,"b":1,"x":1}`), r, rejectUnknown)
	assert.ErrorContains(t, err, "unknown key")

	m, err = checkObject([]byte(`{"a":1,"b":1,"x":1}`), r, ignoreUnknown)
	require.NoError(t, err)
	assert.NotContains(t, m, "x")

	for _, notAnObject := range []string{`null`, `[]`, `"s"`, `1`} {
		_, err = checkObject([]byte(notAnObject), r, ignoreUnknown)
		assert.Error(t, err, notAnObject)
	}
}

func TestCheckText(t *testing.T) {
	bs := "\\"
	for _, ok := range []string{
		`{"a":"x"}`, `{"a":"` + bs + `ud83d` + bs + `ude00"}`, // a surrogate pair
		`{"a":"` + bs + bs + `ud800"}`, // an escaped backslash followed by "ud800"
		`{"a":"` + bs + `"` + bs + bs + `"}`, `{"a":"` + bs + `u00e4"}`,
	} {
		assert.NoError(t, checkText([]byte(ok)), ok)
	}
	for _, bad := range []string{
		`{"a":"` + bs + `ud800"}`, `{"a":"` + bs + `udc00"}`, `{"a":"` + bs + `ud800` + bs + `u0041"}`,
		`{"a":"` + bs + `ud800"`, "{\"a\":\"\xff\"}",
	} {
		assert.Error(t, checkText([]byte(bad)), bad)
	}
}
