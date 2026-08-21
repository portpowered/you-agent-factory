package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPackageTargetFindingsSeparateProductionAndTestOnlyClasses(t *testing.T) {
	t.Parallel()

	const packagePath = "pkg/services/work/dual"
	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work", "package work\n")
	writeGoPackage(t, root, packagePath, "package dual\n")
	writeGoPackage(t, root, packagePath, "package dual\n", "dual_test.go")
	writeFixtureMoveLedger(t, root, []PackageMapping{{
		PackagePath: packagePath,
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}})

	var stdout, stderr bytes.Buffer
	err := run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want an unrecorded test-only observation")
	}
	for _, want := range []string{
		"package-target observations: production=1 test-only=1",
		"dependency violation counts: production=0 test-only=1",
		"[class=test-only]",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), "stale production package-target row") {
		t.Fatalf("run() stderr = %q, a production-live package must not be stale", stderr.String())
	}

	writePackageTargetTestOnlyBaseline(t, root, packageTargetTestOnlyBaselineEntry{
		PackagePath:  packagePath,
		Destination:  "work/internal",
		Successor:    "pkg/services/work/internal",
		Class:        string(packageTargetTestOnlySourceClass),
		Stage:        packageTargetTestOnlyBaselineStage,
		DeletionGate: packageTargetTestOnlyDeletionGate,
	})
	stdout.Reset()
	stderr.Reset()
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() with exact test-only baseline error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dependency violation counts: production=0 test-only=0") {
		t.Fatalf("run() stdout = %q, want zero class-separated violations", stdout.String())
	}
}

func TestTestOnlyPackageDoesNotSatisfyProductionLiveness(t *testing.T) {
	t.Parallel()

	const packagePath = "pkg/services/work/testonly"
	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work", "package work\n")
	writeGoPackage(t, root, packagePath, "package testonly\n", "testonly_test.go")
	writeFixtureMoveLedger(t, root, []PackageMapping{{
		PackagePath: packagePath,
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}})

	var stdout, stderr bytes.Buffer
	err := run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() error = nil, want test-only debt and production-liveness failure")
	}
	for _, want := range []string{
		"package-target observations: production=0 test-only=1",
		"dependency violation counts: production=1 test-only=1",
		"[class=test-only]",
		"stale production package-target row: pkg/services/work/testonly -> work/internal (successor pkg/services/work/internal) [class=production]",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
		}
	}

	writePackageTargetTestOnlyBaseline(t, root, packageTargetTestOnlyBaselineEntry{
		PackagePath:  packagePath,
		Destination:  "work/internal",
		Successor:    "pkg/services/work/internal",
		Class:        string(packageTargetTestOnlySourceClass),
		Stage:        packageTargetTestOnlyBaselineStage,
		DeletionGate: packageTargetTestOnlyDeletionGate,
	})
	stdout.Reset()
	stderr.Reset()
	err = run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("run() with accepted test-only edge error = nil, want production-liveness failure")
	}
	for _, want := range []string{
		"package-target observations: production=0 test-only=1",
		"dependency violation counts: production=1 test-only=0",
		"stale production package-target row: pkg/services/work/testonly -> work/internal (successor pkg/services/work/internal) [class=production]",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want production-liveness diagnostic %q", stderr.String(), want)
		}
	}
}

func TestPackageTargetTestOnlyBaselineRejectsMalformedAndStaleEntries(t *testing.T) {
	t.Parallel()

	const (
		packagePath = "pkg/services/work/testonly"
		destination = "work/internal"
		successor   = "pkg/services/work/internal"
	)
	entry := packageTargetTestOnlyBaselineEntry{
		PackagePath:  packagePath,
		Destination:  destination,
		Successor:    successor,
		Class:        string(packageTargetTestOnlySourceClass),
		Stage:        packageTargetTestOnlyBaselineStage,
		DeletionGate: packageTargetTestOnlyDeletionGate,
	}

	t.Run("wildcard is rejected", func(t *testing.T) {
		root := packageTargetTestOnlyFixture(t, packagePath, false)
		wildcard := entry
		wildcard.Destination = "work/*"
		writePackageTargetTestOnlyBaseline(t, root, wildcard)
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "cannot contain wildcards") {
			t.Fatalf("run() error = %v, want wildcard rejection", err)
		}
	})

	t.Run("duplicate is rejected", func(t *testing.T) {
		root := packageTargetTestOnlyFixture(t, packagePath, true)
		writePackageTargetTestOnlyBaseline(t, root, entry, entry)
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
			t.Fatalf("run() error = %v, want duplicate rejection", err)
		}
	})

	t.Run("class is required and must be test-only", func(t *testing.T) {
		root := packageTargetTestOnlyFixture(t, packagePath, true)
		malformed := entry
		malformed.Class = ""
		writePackageTargetTestOnlyBaseline(t, root, malformed)
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "want explicit") {
			t.Fatalf("run() error = %v, want explicit class rejection", err)
		}
	})

	t.Run("stale entry is class-labeled and blocking", func(t *testing.T) {
		root := t.TempDir()
		writeGoPackage(t, root, "pkg/services/work", "package work\n")
		writeFixtureMoveLedger(t, root, nil)
		writePackageTargetTestOnlyBaseline(t, root, entry)
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil {
			t.Fatal("run() error = nil, want stale baseline failure")
		}
		for _, want := range []string{
			"stale test-only package-target baseline entry",
			"[class=test-only]",
			"dependency violation counts: production=0 test-only=1",
		} {
			if !strings.Contains(stderr.String(), want) {
				t.Fatalf("run() stderr = %q, want %q", stderr.String(), want)
			}
		}
	})
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

func packageTargetTestOnlyFixture(t *testing.T, packagePath string, production bool) string {
	t.Helper()
	root := t.TempDir()
	writeGoPackage(t, root, "pkg/services/work", "package work\n")
	if production {
		writeGoPackage(t, root, packagePath, "package testonly\n")
	}
	writeGoPackage(t, root, packagePath, "package testonly\n", "testonly_test.go")
	writeFixtureMoveLedger(t, root, []PackageMapping{{
		PackagePath: packagePath,
		Destination: "work/internal",
		Successor:   "pkg/services/work/internal",
	}})
	return root
}

func writePackageTargetTestOnlyBaseline(t *testing.T, repoRoot string, entries ...packageTargetTestOnlyBaselineEntry) {
	t.Helper()
	writeFixtureJSON(t, repoRoot, packageTargetTestOnlyBaselineRelativePath, packageTargetTestOnlyBaseline{
		Version: packageTargetTestOnlyBaselineVersion,
		Entries: entries,
	})
}
