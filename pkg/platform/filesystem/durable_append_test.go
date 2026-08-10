package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalAppendDurableCreatesPrivateArtifactAndAppendsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dead-letter.jsonl")
	local := Local{}
	if err := local.AppendDurable(path, []byte("first\n")); err != nil {
		t.Fatalf("AppendDurable(first) error = %v", err)
	}
	if err := local.AppendDurable(path, []byte("second\n")); err != nil {
		t.Fatalf("AppendDurable(second) error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read appended artifact: %v", err)
	}
	if got, want := string(contents), "first\nsecond\n"; got != want {
		t.Fatalf("artifact contents = %q, want %q", got, want)
	}
}
