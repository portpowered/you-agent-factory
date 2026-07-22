package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsWorkNormalizationFromTestsOutsideWorkOwner(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/work/request_normalize_test.go", `package work
func TestOwnerMayNormalize() { NormalizeWorkRequest(WorkRequest{}, WorkRequestNormalizeOptions{}) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/workers/linear_test.go", `package workers
import workdomain "github.com/portpowered/infinite-you/pkg/services/work"
func assertSubmission() { workdomain.NormalizeWorkRequest(workdomain.WorkRequest{}, workdomain.WorkRequestNormalizeOptions{}) }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want cross-owner test Work normalization rejected")
	}
	for _, want := range []string{
		"prohibited cross-owner test Work normalization: NormalizeWorkRequest",
		"pkg/services/workers/linear_test.go:3",
		"relocate pure normalization scenarios to pkg/services/work",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsWorkNormalizationFromReusableTestSupport(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/root/root.go", "package root\n")
	writeGoSourceFile(t, repoRoot, "internal/testutil/work.go", `package testutil
import . "github.com/portpowered/infinite-you/pkg/services/work"
func NormalizeForConsumers() { NormalizeWorkRequest(WorkRequest{}, WorkRequestNormalizeOptions{}) }
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want reusable-support Work normalization rejected")
	}
	if got := stderr.String(); !strings.Contains(got, "internal/testutil/work.go:3") {
		t.Fatalf("run() stderr = %q, want reusable-support source location", got)
	}
}
