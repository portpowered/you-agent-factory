package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRunReportsClosedReplayBoundaryBaseline(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	var stdout bytes.Buffer
	if err := run(root, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := stdout.String(); got != successOutput {
		t.Fatalf("stdout = %q, want %q", got, successOutput)
	}
}
