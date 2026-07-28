package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestRecordingsTopLevelChildrenMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListRecordingsTopLevelChildren(root)
	if err != nil {
		t.Fatalf("ListRecordingsTopLevelChildren() error = %v", err)
	}

	want := ownershipinventory.RecordingsTopLevelInventory()
	if !slices.Equal(live, want) {
		t.Fatalf("live top-level children = %v, want committed inventory %v", live, want)
	}
}

func TestRecordingsTopLevelExpectedRetainChildren(t *testing.T) {
	t.Parallel()

	want := []string{"internal", "transports", "wire"}
	if !slices.Equal(ownershipinventory.RecordingsTopLevelExpectedRetain, want) {
		t.Fatalf("RecordingsTopLevelExpectedRetain = %v, want %v", ownershipinventory.RecordingsTopLevelExpectedRetain, want)
	}

	for _, name := range ownershipinventory.RecordingsTopLevelExpectedRetain {
		kind, ok := ownershipinventory.ClassifyRecordingsTopLevelChild(name)
		if !ok {
			t.Fatalf("ClassifyRecordingsTopLevelChild(%q) ok = false", name)
		}
		if kind != "expected_retain" {
			t.Fatalf("ClassifyRecordingsTopLevelChild(%q) = %q, want expected_retain", name, kind)
		}
	}
}

func TestRecordingsTopLevelUnexpectedChildren(t *testing.T) {
	t.Parallel()

	want := []string{"artifacts", "events", "projections", "replay", "service"}
	if !slices.Equal(ownershipinventory.RecordingsTopLevelUnexpected, want) {
		t.Fatalf("RecordingsTopLevelUnexpected = %v, want %v", ownershipinventory.RecordingsTopLevelUnexpected, want)
	}

	for _, name := range ownershipinventory.RecordingsTopLevelUnexpected {
		kind, ok := ownershipinventory.ClassifyRecordingsTopLevelChild(name)
		if !ok {
			t.Fatalf("ClassifyRecordingsTopLevelChild(%q) ok = false", name)
		}
		if kind != "unexpected_move" {
			t.Fatalf("ClassifyRecordingsTopLevelChild(%q) = %q, want unexpected_move", name, kind)
		}
	}
}

func TestIsRecordingsCanonicalRetainRest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rest string
		want bool
	}{
		{rest: "wire", want: true},
		{rest: "wire/cli", want: true},
		{rest: "transports/http", want: true},
		{rest: "internal/services/replay/wire", want: true},
		{rest: "internal/canonical", want: true},
		{rest: "artifacts", want: false},
		{rest: "events/kinds", want: false},
		{rest: "service", want: false},
	}

	for _, tc := range cases {
		got := ownershipinventory.IsRecordingsCanonicalRetainRest(tc.rest)
		if got != tc.want {
			t.Fatalf("IsRecordingsCanonicalRetainRest(%q) = %v, want %v", tc.rest, got, tc.want)
		}
	}
}

func TestIsRecordingsUnexpectedTopLevelRest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rest string
		want bool
	}{
		{rest: "artifacts", want: true},
		{rest: "events/kinds", want: true},
		{rest: "projections/dashboard", want: true},
		{rest: "replay/clocktests", want: true},
		{rest: "service", want: true},
		{rest: "wire", want: false},
		{rest: "internal/services/replay", want: false},
	}

	for _, tc := range cases {
		got := ownershipinventory.IsRecordingsUnexpectedTopLevelRest(tc.rest)
		if got != tc.want {
			t.Fatalf("IsRecordingsUnexpectedTopLevelRest(%q) = %v, want %v", tc.rest, got, tc.want)
		}
	}
}

func TestRecordingsTopLevelClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	live, err := ownershipinventory.ListRecordingsTopLevelChildren(root)
	if err != nil {
		t.Fatalf("ListRecordingsTopLevelChildren() error = %v", err)
	}

	for _, name := range live {
		kind, ok := ownershipinventory.ClassifyRecordingsTopLevelChild(name)
		if !ok {
			t.Fatalf("live top-level child %q is not classified", name)
		}
		if kind != "expected_retain" && kind != "unexpected_move" {
			t.Fatalf("live top-level child %q has unknown kind %q", name, kind)
		}
	}

	const ownerPrefix = "pkg/services/recordings/"
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	for _, packagePath := range packages {
		if packagePath == "pkg/services/recordings" {
			continue
		}
		if !strings.HasPrefix(packagePath, ownerPrefix) {
			continue
		}
		rest := strings.TrimPrefix(packagePath, ownerPrefix)
		canonical := ownershipinventory.IsRecordingsCanonicalRetainRest(rest)
		unexpected := ownershipinventory.IsRecordingsUnexpectedTopLevelRest(rest)
		if canonical && unexpected {
			t.Fatalf("inventory path %q is both canonical retain and unexpected", packagePath)
		}
		if !canonical && !unexpected {
			t.Fatalf("inventory path %q is neither canonical retain nor unexpected top-level sibling", packagePath)
		}
	}
}
