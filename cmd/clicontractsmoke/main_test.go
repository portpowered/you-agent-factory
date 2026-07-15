package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clicontract"
)

func TestRunAcceptsCompleteProductionTree(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(repositoryRoot(t), "", stdout, stderr); status != 0 {
		t.Fatalf("run() status = %d, want 0; stderr = %q", status, stderr.String())
	}
	if stdout.String() != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunPropagatesDeliberateViolationDiagnostics(t *testing.T) {
	tests := []struct {
		violation clicontract.DeliberateViolation
		want      string
	}{
		{clicontract.ViolationUncontractedCommand, `uncontracted-command: stable ID "you.experimental" path "you experimental"`},
		{clicontract.ViolationStaleMetadata, `stale-generated-metadata: stable ID "you" path "you" field "name"`},
		{clicontract.ViolationMissingHandler, `missing-handler: stable ID "you.run" path "you run" field "handler"`},
		{clicontract.ViolationAliasAsCanonical, `compatibility-alias-as-canonical: stable ID "you.workflow.preview" path "you workflow preview" field "classification"`},
	}

	for _, tc := range tests {
		t.Run(string(tc.violation), func(t *testing.T) {
			var previous string
			for iteration := 0; iteration < 2; iteration++ {
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				if status := run(repositoryRoot(t), string(tc.violation), stdout, stderr); status != 1 {
					t.Fatalf("run() status = %d, want 1", status)
				}
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
					t.Fatalf("run() stdout = %q, stderr = %q, want diagnostic %q", stdout.String(), stderr.String(), tc.want)
				}
				if iteration > 0 && stderr.String() != previous {
					t.Fatalf("repeated diagnostic changed:\nfirst: %q\nsecond: %q", previous, stderr.String())
				}
				previous = stderr.String()
			}
		})
	}
}

func TestRunRejectsUnknownViolation(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(repositoryRoot(t), "unknown", stdout, stderr); status != 1 {
		t.Fatalf("run() status = %d, want 1", status)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `unknown deliberate CLI contract violation "unknown"`) {
		t.Fatalf("run() stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
