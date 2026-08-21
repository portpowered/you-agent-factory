package contracts_test

import (
	"path/filepath"
	"testing"
)

func TestCLIShapeStablePathCutoverIsConsistentAcrossPublishedContracts(t *testing.T) {
	t.Parallel()

	manifestPaths := []string{
		filepath.Join("cli", "commands.json"),
		filepath.Join("..", "packages", "api", "generated", "cli", "commands.json"),
	}
	for _, path := range manifestPaths {
		document := readJSON(t, path)
		commands := document.(map[string]any)["commands"].(map[string]any)

		assertCommandPath(t, commands, "you.factory.show", "you factory show")
		assertCommandPath(t, commands, "you.work.render", "you work render")
		for _, retired := range []string{
			"you.factory.query",
			"you.work.visualize",
			"you.session.dispatches",
			"you.worker-session.dispatches",
			"you.worker-sessions.dispatches",
		} {
			if _, found := commands[retired]; found {
				t.Fatalf("%s still contains retired command %q", path, retired)
			}
		}

		for _, id := range []string{"you.factory.show", "you.work.render"} {
			command := commands[id].(map[string]any)
			aliases, _ := command["aliases"].([]any)
			if len(aliases) != 0 {
				t.Fatalf("%s command %q aliases = %#v, want none", path, id, aliases)
			}
		}
	}
}

func TestCLIShapeStablePathCutoverKeepsCompatibilityLedgersEmpty(t *testing.T) {
	for _, test := range []struct {
		path string
		key  string
	}{
		{path: filepath.Join("cli", "deprecated-commands.json"), key: "commands"},
		{path: filepath.Join("cli", "deprecated.json"), key: "records"},
	} {
		document := readJSON(t, test.path).(map[string]any)
		entries := document[test.key].(map[string]any)
		if len(entries) != 0 {
			t.Fatalf("%s %s = %#v, want empty after breaking cutover", test.path, test.key, entries)
		}
	}
}

func assertCommandPath(t *testing.T, commands map[string]any, id, wantPath string) {
	t.Helper()
	command, ok := commands[id].(map[string]any)
	if !ok {
		t.Fatalf("manifest missing command %q", id)
	}
	if got := command["path"]; got != wantPath {
		t.Fatalf("command %q path = %v, want %q", id, got, wantPath)
	}
}
