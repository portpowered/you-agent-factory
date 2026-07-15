package contractopenapiconverter_test

import (
	"bytes"
	"os"
	"testing"
)

// readGoldenBytes loads a checked-in golden fixture and normalizes CRLF line
// endings so byte comparisons stay stable on Windows runners that check out
// text files with core.autocrlf conversions.
func readGoldenBytes(t *testing.T, path string) []byte {
	t.Helper()
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return bytes.ReplaceAll(golden, []byte("\r\n"), []byte("\n"))
}
