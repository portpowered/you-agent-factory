package commandidentity_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
)

func TestWalk_ProductionRootInventoriesEveryReachableCommandOnce(t *testing.T) {
	observation := productionCLIObservation(t)
	inventory := observation.Snapshot.Commands

	if inventory.RootPath != "you" {
		t.Fatalf("RootPath = %q, want you", inventory.RootPath)
	}

	wantPaths := productionCommandPaths(t, observation.Snapshot.CommandTree)
	gotPaths := commandPathsFromInventory(t, inventory.Commands)

	if len(gotPaths) != len(wantPaths) {
		t.Fatalf("command count = %d, want %d reachable production commands", len(gotPaths), len(wantPaths))
	}

	if !sort.StringsAreSorted(gotPaths) {
		t.Fatalf("inventory paths are not sorted: %#v", gotPaths)
	}

	wantSet := make(map[string]struct{}, len(wantPaths))
	for _, path := range wantPaths {
		wantSet[path] = struct{}{}
	}
	for _, path := range gotPaths {
		if _, ok := wantSet[path]; !ok {
			t.Fatalf("unexpected command path in inventory: %q", path)
		}
		delete(wantSet, path)
	}
	if len(wantSet) != 0 {
		missing := make([]string, 0, len(wantSet))
		for path := range wantSet {
			missing = append(missing, path)
		}
		sort.Strings(missing)
		t.Fatalf("missing command paths from inventory: %v", missing)
	}
}

func TestWalk_ProductionRootRepresentativeCommandsRetainIdentity(t *testing.T) {
	inventory := productionCLIObservation(t).Snapshot.Commands

	byPath := indexCommandsByPath(t, inventory.Commands)

	cases := []struct {
		path       string
		visibility string
		runnable   bool
	}{
		{path: "you", visibility: "visible", runnable: true},
		{path: "you run", visibility: "visible", runnable: true},
		{path: "you submit batch", visibility: "visible", runnable: true},
		{path: "you session show", visibility: "visible", runnable: true},
		{path: "you mcp serve", visibility: "visible", runnable: true},
	}
	if _, ok := byPath["you workflow"]; ok {
		t.Fatal("production command tree must not expose the removed workflow alias family")
	}

	for _, tc := range cases {
		record, ok := byPath[tc.path]
		if !ok {
			t.Fatalf("missing representative production command path %q", tc.path)
		}
		if record.Visibility != tc.visibility {
			t.Fatalf("%s visibility = %q, want %q", tc.path, record.Visibility, tc.visibility)
		}
		if record.Runnable != tc.runnable {
			t.Fatalf("%s runnable = %t, want %t", tc.path, record.Runnable, tc.runnable)
		}
	}
}

func productionCommandPaths(t *testing.T, commandTree string) []string {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(commandTree, "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		path, _, _ := strings.Cut(line, "\t")
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func commandPathsFromInventory(t *testing.T, commands []commandidentity.CommandRecord) []string {
	t.Helper()

	paths := make([]string, len(commands))
	for i, record := range commands {
		paths[i] = record.Path
	}
	return paths
}
