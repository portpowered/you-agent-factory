package main

import (
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
