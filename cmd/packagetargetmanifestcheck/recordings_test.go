package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRecordingsTopLevelChildrenMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsTopLevelChildren(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsTopLevelChildren() error = %v", err)
	}

	want := recordingsTopLevelInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live top-level children = %v, want committed inventory %v", live, want)
	}
}

func TestMapCommittedOwnerPackageRecordingsClassifiesExpectedTopLevelAsRetain(t *testing.T) {
	t.Parallel()

	cases := []string{
		"pkg/services/recordings",
		"pkg/services/recordings/wire",
		"pkg/services/recordings/transports/http",
		"pkg/services/recordings/internal/services/replay/wire",
		"pkg/services/recordings/internal/canonical",
	}

	for _, path := range cases {
		got, ok := mapCommittedOwnerPackage(path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", path)
		}
		if got.Disposition != DispositionRetain {
			t.Fatalf("mapCommittedOwnerPackage(%q) disposition = %q, want retain", path, got.Disposition)
		}
		if !strings.HasPrefix(got.Destination, "recordings") {
			t.Fatalf("mapCommittedOwnerPackage(%q) destination = %q, want recordings*", path, got.Destination)
		}
	}
}

func TestMapCommittedOwnerPackageRecordingsClassifiesUnexpectedTopLevelAsMove(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path        string
		destination string
	}{
		{
			path:        "pkg/services/recordings/artifacts",
			destination: "recordings/internal/services/artifacts_export",
		},
		{
			path:        "pkg/services/recordings/events/kinds",
			destination: "recordings/internal/services/canonical_ledger",
		},
		{
			path:        "pkg/services/recordings/projections/dashboard",
			destination: "recordings/internal/services/projection_query",
		},
		{
			path:        "pkg/services/recordings/replay/clocktests",
			destination: "recordings/internal/services/replay",
		},
		{
			path:        "pkg/services/recordings/service",
			destination: "recordings/internal",
		},
	}

	for _, tc := range cases {
		got, ok := mapCommittedOwnerPackage(tc.path)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", tc.path)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("mapCommittedOwnerPackage(%q) disposition = %q, want move", tc.path, got.Disposition)
		}
		if got.Destination != tc.destination {
			t.Fatalf("mapCommittedOwnerPackage(%q) destination = %q, want %q", tc.path, got.Destination, tc.destination)
		}
	}
}

func TestRecordingsTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	for _, child := range recordingsTopLevelUnexpected {
		rest := child
		if child == "service" {
			got, ok := mapLegacyServiceImplementationPackage("recordings", "pkg/services/recordings/"+child, rest)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage() ok = false for %q", child)
			}
			if got.Disposition != DispositionMove || got.Destination != "recordings/internal" {
				t.Fatalf("service move mapping = %#v, want move→recordings/internal", got)
			}
			continue
		}

		destination, ok := nestedOwnerMoveDestination("recordings", rest)
		if !ok {
			t.Fatalf("nestedOwnerMoveDestination(recordings, %q) ok = false", rest)
		}
		if destination == "recordings" {
			t.Fatalf("unexpected top-level child %q maps to owner root retain destination", child)
		}
	}
}

func TestRecordingsCanonicalRetainRestMatchesTopLevelInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	live, err := listRecordingsTopLevelChildren(repoRoot)
	if err != nil {
		t.Fatalf("listRecordingsTopLevelChildren() error = %v", err)
	}

	for _, name := range live {
		if slices.Contains(recordingsTopLevelExpectedRetain, name) {
			if !recordingsCanonicalRetainRest(name) {
				t.Fatalf("recordingsCanonicalRetainRest(%q) = false, want true", name)
			}
			continue
		}
		if slices.Contains(recordingsTopLevelUnexpected, name) {
			if recordingsUnexpectedTopLevelRest(name) != true {
				t.Fatalf("recordingsUnexpectedTopLevelRest(%q) = false, want true", name)
			}
			continue
		}
		t.Fatalf("live top-level child %q is missing from committed inventory", name)
	}
}

func TestCommittedManifestRecordingsRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, row := range manifest.Packages {
		if row.PackagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(row.PackagePath, ownerPrefix)
		if recordingsCanonicalRetainRest(rest) {
			continue
		}
		if row.Disposition == DispositionRetain && row.Destination == "recordings" {
			t.Fatalf("committed manifest row retain→recordings for %q", row.PackagePath)
		}
		if row.Disposition != DispositionMove {
			t.Fatalf("committed manifest row %q disposition = %q, want move", row.PackagePath, row.Disposition)
		}
		if row.Destination == "recordings" {
			t.Fatalf("committed manifest row %q move destination = owner root, want nested plan path", row.PackagePath)
		}
	}
}

func TestRecordingsInventoryRejectsRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, packagePath := range manifest.Inventory {
		if packagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		if recordingsCanonicalRetainRest(rest) {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if got.Disposition == DispositionRetain && got.Destination == "recordings" {
			t.Fatalf("unexpected retain→recordings for inventory path %q", packagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("inventory path %q disposition = %q, want move", packagePath, got.Disposition)
		}
	}
}

func TestRecordingsCommittedManifestAlignMoveDestinations(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	const ownerPrefix = "pkg/services/recordings/"
	for _, row := range manifest.Packages {
		if !strings.HasPrefix(row.PackagePath, ownerPrefix) {
			continue
		}
		if row.Disposition != DispositionMove {
			continue
		}

		got, ok := mapCommittedOwnerPackage(row.PackagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", row.PackagePath)
		}
		if got.Disposition != DispositionMove {
			t.Fatalf("mapCommittedOwnerPackage(%q) disposition = %q, want move", row.PackagePath, got.Disposition)
		}
		if got.Destination != row.Destination {
			t.Fatalf("dual-ledger drift for %q: manifest destination %q, generator has %q",
				row.PackagePath, row.Destination, got.Destination)
		}
	}
}
