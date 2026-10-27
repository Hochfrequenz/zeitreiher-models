package zeitreihermodels_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	zrm "github.com/hochfrequenz/zeitreiher-models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite testdata/serialized.golden from the Go output")

// goldenFile pins the exact bytes both languages write for every valid fixture: one line per
// fixture (or stream line), "<fixture>\t<serialized JSON>". The Python tests compare against the
// same file (unittests/test_fixtures.py), so Go and Python output is byte-for-byte identical.
var goldenFile = filepath.Join(testdata, "serialized.golden")

func serializedFixtures(t *testing.T) string {
	t.Helper()
	var lines []string
	for kind, c := range kinds {
		for _, path := range fixtureFiles(t, kind, "valid", ".json") {
			v, err := c.parse(read(t, path))
			require.NoError(t, err, path)
			out, err := c.dump(v)
			require.NoError(t, err, path)
			lines = append(lines, kind+"/"+filepath.Base(path)+"\t"+string(out))
		}
	}
	for _, path := range fixtureFiles(t, "streams", "valid", ".ndjson") {
		records, err := zrm.ReadStream(bytes.NewReader(read(t, path)))
		require.NoError(t, err, path)
		for i, rec := range records {
			out, err := zrm.MarshalRecord(rec)
			require.NoError(t, err, path)
			lines = append(lines, fmt.Sprintf("streams/%s:%d\t%s", filepath.Base(path), i+1, out))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func TestSerializedBytesMatchGolden(t *testing.T) {
	got := serializedFixtures(t)
	if *update {
		require.NoError(t, os.WriteFile(goldenFile, []byte(got), 0o600))
	}
	assert.Equal(t, string(read(t, goldenFile)), got, "run `go test -run Golden -update` after an intended change")
}
