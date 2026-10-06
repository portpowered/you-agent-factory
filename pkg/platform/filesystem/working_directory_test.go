package filesystem

import (
	"os"
	"testing"
)

func TestLocalWorkingDirectoryLeavesHostDirectoryUnchanged(t *testing.T) {
	t.Parallel()
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	owned := t.TempDir()
	got, err := (Local{WorkingDirectory: owned}).Getwd()
	if err != nil || got != owned {
		t.Fatalf("owned directory = %q, %v; want %q", got, err, owned)
	}
	after, err := (Local{}).Getwd()
	if err != nil || after != before {
		t.Fatalf("host directory = %q, %v; want unchanged %q", after, err, before)
	}
}
