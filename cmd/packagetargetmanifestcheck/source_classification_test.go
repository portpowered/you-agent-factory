package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunClassifiesPackageTargetEdgesBySourceClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		production bool
		testOnly   bool
		wantErr    bool
		counts     string
		violations string
		classes    []string
	}{
		{
			name:       "production-only",
			production: true,
			counts:     "package-target observations: production=1 test-only=0",
			violations: "dependency violation counts: production=0 test-only=0",
		},
		{
			name:       "test-only",
			testOnly:   true,
			counts:     "package-target observations: production=0 test-only=1",
			violations: "dependency violation counts: production=0 test-only=1",
			classes:    []string{"[class=test-only]"},
		},
		{
			name:       "mixed",
			production: true,
			testOnly:   true,
			counts:     "package-target observations: production=1 test-only=1",
			violations: "dependency violation counts: production=0 test-only=1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeGoPackage(t, root, "pkg/services/work", "package work\n")
			if test.production {
				writeGoPackage(t, root, "pkg/services/work/edge", "package edge\n")
			}
			if test.testOnly {
				writeGoPackage(t, root, "pkg/services/work/edge", "package edge_test\n", "edge_test.go")
			}
			writeFixtureMoveLedger(t, root, []PackageMapping{{
				PackagePath: "pkg/services/work/edge",
				Destination: "work/internal",
				Successor:   "pkg/services/work/internal",
			}})

			var stdout, stderr bytes.Buffer
			err := run(config{root: root}, &stdout, &stderr)
			if (err != nil) != test.wantErr {
				t.Fatalf("run() error = %v, wantErr=%t; stdout=%q stderr=%q", err, test.wantErr, stdout.String(), stderr.String())
			}
			output := stdout.String() + stderr.String()
			for _, want := range []string{test.counts, test.violations} {
				if !strings.Contains(output, want) {
					t.Fatalf("run() output = %q, want %q", output, want)
				}
			}
			for _, class := range test.classes {
				if !strings.Contains(output, class) {
					t.Fatalf("run() output = %q, want class %q", output, class)
				}
			}
			if test.name == "test-only" && err != nil {
				t.Fatalf("test-only observation entered the blocking path: %v; stderr=%q", err, stderr.String())
			}
		})
	}
}

func TestRunBlocksStaleProductionRowsWithoutBlockingTestOnlyObservations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work", "package work\n")
	writeGoPackage(t, root, "pkg/services/work/onlytest", "package onlytest_test\n", "onlytest_test.go")
	writeFixtureMoveLedger(t, root, []PackageMapping{
		{
			PackagePath: "pkg/services/work/missing",
			Destination: "work/internal",
			Successor:   "pkg/services/work/internal",
		},
		{
			PackagePath: "pkg/services/work/onlytest",
			Destination: "work/internal",
			Successor:   "pkg/services/work/internal",
		},
	})

	var stdout, stderr bytes.Buffer
	err := run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want stale production row failure")
	}
	output := stdout.String() + stderr.String()
	for _, want := range []string{
		"package-target observations: production=0 test-only=1",
		"dependency violation counts: production=1 test-only=1",
		"test-only observation: pkg/services/work/onlytest -> work/internal (successor pkg/services/work/internal) [class=test-only]",
		"stale production package-target row: pkg/services/work/missing -> work/internal (successor pkg/services/work/internal) [class=production]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("run() output = %q, want %q", output, want)
		}
	}
	if !strings.Contains(err.Error(), "LINT_VIOLATION_COUNT: 1") {
		t.Fatalf("run() error = %v, want one production violation", err)
	}
	if strings.Contains(output, "test-only package-target baseline") {
		t.Fatalf("run() output = %q, must not mention a test-only baseline", output)
	}
}

func TestPackageTargetSourceClassDiscoveryRetainsBothClasses(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work/edge", "package edge\n")
	writeGoPackage(t, root, "pkg/services/work/edge", "package edge_test\n", "edge_test.go")

	classes, err := packageTargetSourceClasses(root, "pkg/services/work/edge")
	if err != nil {
		t.Fatalf("packageTargetSourceClasses() error = %v", err)
	}
	if _, ok := classes[packageTargetProductionSourceClass]; !ok {
		t.Fatalf("classes = %#v, missing production", classes)
	}
	if _, ok := classes[packageTargetTestOnlySourceClass]; !ok {
		t.Fatalf("classes = %#v, missing test-only", classes)
	}
}

func TestPackageTargetFindingKeyIncludesSourceClass(t *testing.T) {
	production := packageTargetFinding{
		PackagePath: "pkg/services/work/edge",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
		Class:       packageTargetProductionSourceClass,
	}
	testOnly := production
	testOnly.Class = packageTargetTestOnlySourceClass
	if packageTargetFindingKey(production) == packageTargetFindingKey(testOnly) {
		t.Fatalf("package-target finding keys collapsed source classes: %q", packageTargetFindingKey(production))
	}
}

func TestPackageTargetDoesNotCreateTestOnlyBaseline(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work", "package work\n")
	writeGoPackage(t, root, "pkg/services/work/edge", "package edge_test\n", "edge_test.go")
	writeFixtureMoveLedger(t, root, []PackageMapping{{
		PackagePath: "pkg/services/work/edge",
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}})

	var stdout, stderr bytes.Buffer
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v; stderr=%s", err, stderr.String())
	}
	entries, err := os.ReadDir(filepath.Join(root, "docs", "internal", "baselines"))
	if err != nil {
		t.Fatalf("read baseline directory: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "test-only") {
			t.Fatalf("run() created test-only baseline %q", entry.Name())
		}
	}
}
