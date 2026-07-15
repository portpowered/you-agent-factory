package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/configcontractsmoke"
	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestRunPassesCleanRepository(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root := testpath.MustRepoPathFromCaller(t, 0)
	if status := run(root, stdout, stderr); status != 0 {
		t.Fatalf("run() status = %d, stderr = %q", status, stderr)
	}
	if stdout.String() != successMessage+"\n" || stderr.Len() != 0 {
		t.Fatalf("run() stdout = %q, stderr = %q", stdout, stderr)
	}
}

func TestRunReportsStableFamilyAndPathDiagnostic(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	check := func(string) ([]configcontractsmoke.Diagnostic, error) {
		return []configcontractsmoke.Diagnostic{{
			Code: "config.acceptance.mismatch", Family: configcontractsmoke.FamilyFactory,
			Path: "fixtures/factory.json", Message: "loader=accept schema=reject",
		}}, nil
	}
	if status := runWithChecker(".", stdout, stderr, check); status != 1 {
		t.Fatalf("runWithChecker() status = %d, want 1", status)
	}
	want := "fixtures/factory.json family=factory (config.acceptance.mismatch): loader=accept schema=reject"
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), want) {
		t.Fatalf("stdout = %q, stderr = %q, want %q", stdout, stderr, want)
	}
}

func TestRunReportsCheckFailure(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	check := func(string) ([]configcontractsmoke.Diagnostic, error) {
		return nil, errors.New("projection unavailable")
	}
	if status := runWithChecker(".", stdout, stderr, check); status != 1 {
		t.Fatalf("runWithChecker() status = %d, want 1", status)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "check failed: projection unavailable") {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
}
