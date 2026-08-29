package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestRunAtRootReportsSixOrderedWrites(t *testing.T) {
	sourceRoot := commandRepositoryRoot(t)
	root := t.TempDir()
	copyCommandFixture(t, sourceRoot, root)

	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	inventory, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	candidates, err := ownershipinventory.BuildSnapshotCandidates(root)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	freeze := ownershipinventory.BuildPathLeaseFreeze()

	var stdout bytes.Buffer
	if err := runAtRoot(root, &stdout); err != nil {
		t.Fatalf("runAtRoot() error = %v", err)
	}

	want := []string{
		fmt.Sprintf("wrote %s (%d packages)", ownershipinventory.InventoryRelativePath, len(inventory.Packages)),
		fmt.Sprintf("wrote %s (%d packets)", ownershipinventory.PathLeaseFreezeRelativePath, len(freeze.Packets)),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.OperatorSettingsRootGoInventoryRelativePath, len(candidates.OperatorSettingsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath, len(candidates.OperatorSettingsTopLevel.Children)),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.ProviderSessionsRootGoInventoryRelativePath, len(candidates.ProviderSessionsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath, len(candidates.ProviderSessionsTopLevel.Children)),
	}
	got := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Fatalf("runAtRoot() output = %q, want ordered lines %q", got, want)
	}

	for _, relativePath := range commandOutputPaths() {
		wantBytes, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read source artifact %s: %v", relativePath, err)
		}
		gotBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read generated artifact %s: %v", relativePath, err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Fatalf("generated artifact %s differs from source artifact", relativePath)
		}
	}
}

func TestOwnershipSnapshotGroupCharacterization(t *testing.T) {
	sourceRoot := commandRepositoryRoot(t)
	root := t.TempDir()
	copyCommandFixture(t, sourceRoot, root)

	wantPaths := []string{
		"docs/internal/baselines/ownership-inventory.json",
		"docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json",
		"docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json",
		"docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json",
		"docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json",
		"docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json",
	}
	outputs := ownershipSnapshotOutputs()
	if len(outputs) != len(wantPaths) {
		t.Fatalf("snapshot output count = %d, want %d", len(outputs), len(wantPaths))
	}
	wantIDs := []string{"S-03", "S-04", "S-05", "S-06", "S-07", "S-08"}
	for index, output := range outputs {
		if output.id != wantIDs[index] {
			t.Fatalf("snapshot output %d id = %q, want %q", index, output.id, wantIDs[index])
		}
		if output.relativePath != wantPaths[index] {
			t.Fatalf("snapshot output %d path = %q, want %q", index, output.relativePath, wantPaths[index])
		}
	}

	wantPayloads := make(map[string]snapshotFileState, len(outputs))
	for _, output := range outputs {
		payload, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(output.relativePath)))
		if err != nil {
			t.Fatalf("read committed %s: %v", output.relativePath, err)
		}
		if !hasExactlyOneTrailingNewline(payload) {
			t.Fatalf("committed %s does not have exactly one trailing newline", output.relativePath)
		}
		wantPayloads[output.relativePath] = snapshotFileState{payload: payload}
	}
	initial := seedSnapshotTargets(t, root, outputs)
	for relativePath, state := range initial {
		wantPayloads[relativePath] = snapshotFileState{
			payload: wantPayloads[relativePath].payload,
			mode:    state.mode,
		}
	}

	wantOutput := expectedRunOutput(t, root)
	stdout := &stateObservingWriter{
		observe: func() error {
			return assertSnapshotFileStates(root, wantPayloads)
		},
	}
	if err := runAtRoot(root, stdout); err != nil {
		t.Fatalf("runAtRoot() error = %v", err)
	}
	if !stdout.observed {
		t.Fatal("runAtRoot() did not write the success report")
	}
	if stdout.observationErr != nil {
		t.Fatalf("success report was written before all snapshot state was installed: %v", stdout.observationErr)
	}
	if got := strings.TrimSuffix(stdout.String(), "\n"); got != strings.Join(wantOutput, "\n") {
		t.Fatalf("runAtRoot() output = %q, want %q", got, strings.Join(wantOutput, "\n"))
	}
	if err := assertSnapshotFileStates(root, wantPayloads); err != nil {
		t.Fatalf("post-run snapshot state: %v", err)
	}
}

func TestRunAtRootRejectsUnclassifiedLiveUnitBeforeSuccessOutput(t *testing.T) {
	sourceRoot := commandRepositoryRoot(t)
	root := t.TempDir()
	copyCommandFixture(t, sourceRoot, root)
	outputs := ownershipSnapshotOutputs()
	initial := seedSnapshotTargets(t, root, outputs)

	unclassified := filepath.Join(root, filepath.FromSlash("pkg/services/operator_settings/surprise.go"))
	if err := os.WriteFile(unclassified, []byte("package operator_settings\n"), 0o644); err != nil {
		t.Fatalf("write unclassified unit: %v", err)
	}

	var stdout bytes.Buffer
	err := runAtRoot(root, &stdout)
	if err == nil {
		t.Fatal("runAtRoot() error = nil, want unclassified-unit failure")
	}
	if !strings.Contains(err.Error(), "surprise.go") || !strings.Contains(err.Error(), "unclassified") {
		t.Fatalf("runAtRoot() error = %v, want named unclassified unit", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runAtRoot() wrote success output on candidate failure: %q", stdout.String())
	}
	if err := assertSnapshotFileStates(root, initial); err != nil {
		t.Fatalf("snapshot state changed before candidate failure returned: %v", err)
	}
}

func TestRunFailsOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout bytes.Buffer
	err := run(&stdout)
	if err == nil {
		t.Fatal("run() error = nil, want repository-root failure")
	}
	if !strings.Contains(err.Error(), "find repository root") {
		t.Fatalf("run() error = %v, want root-discovery stage", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote stdout on root-discovery failure: %q", stdout.String())
	}
}

func commandRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func commandOutputPaths() []string {
	outputs := ownershipSnapshotOutputs()
	paths := make([]string, 0, len(outputs))
	for _, output := range outputs {
		paths = append(paths, output.relativePath)
	}
	return paths
}

type snapshotOutput struct {
	id           string
	relativePath string
}

func ownershipSnapshotOutputs() []snapshotOutput {
	return []snapshotOutput{
		{id: "S-03", relativePath: ownershipinventory.InventoryRelativePath},
		{id: "S-04", relativePath: ownershipinventory.PathLeaseFreezeRelativePath},
		{id: "S-05", relativePath: ownershipinventory.OperatorSettingsRootGoInventoryRelativePath},
		{id: "S-06", relativePath: ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath},
		{id: "S-07", relativePath: ownershipinventory.ProviderSessionsRootGoInventoryRelativePath},
		{id: "S-08", relativePath: ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath},
	}
}

type snapshotFileState struct {
	payload []byte
	mode    os.FileMode
}

func seedSnapshotTargets(t *testing.T, root string, outputs []snapshotOutput) map[string]snapshotFileState {
	t.Helper()
	seedModes := []os.FileMode{0o600, 0o620, 0o640, 0o660, 0o604, 0o624}
	states := make(map[string]snapshotFileState, len(outputs))
	for index, output := range outputs {
		path := filepath.Join(root, filepath.FromSlash(output.relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", output.relativePath, err)
		}
		payload := []byte("sentinel-" + output.id + "\n")
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("write sentinel %s: %v", output.relativePath, err)
		}
		if err := os.Chmod(path, seedModes[index]); err != nil {
			t.Fatalf("chmod sentinel %s: %v", output.relativePath, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat sentinel %s: %v", output.relativePath, err)
		}
		states[output.relativePath] = snapshotFileState{payload: payload, mode: info.Mode().Perm()}
	}
	return states
}

func assertSnapshotFileStates(root string, want map[string]snapshotFileState) error {
	for relativePath, expected := range want {
		path := filepath.Join(root, filepath.FromSlash(relativePath))
		got, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", relativePath, err)
		}
		if !bytes.Equal(got, expected.payload) {
			return fmt.Errorf("%s bytes = %q, want %q", relativePath, got, expected.payload)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", relativePath, err)
		}
		if info.Mode().Perm() != expected.mode {
			return fmt.Errorf("%s mode = %o, want %o", relativePath, info.Mode().Perm(), expected.mode)
		}
	}
	return nil
}

func expectedRunOutput(t *testing.T, root string) []string {
	t.Helper()
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}
	inventory, err := ownershipinventory.BuildInventory(root, packages)
	if err != nil {
		t.Fatalf("BuildInventory() error = %v", err)
	}
	candidates, err := ownershipinventory.BuildSnapshotCandidates(root)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	return []string{
		fmt.Sprintf("wrote %s (%d packages)", ownershipinventory.InventoryRelativePath, len(inventory.Packages)),
		fmt.Sprintf("wrote %s (%d packets)", ownershipinventory.PathLeaseFreezeRelativePath, len(freeze.Packets)),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.OperatorSettingsRootGoInventoryRelativePath, len(candidates.OperatorSettingsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath, len(candidates.OperatorSettingsTopLevel.Children)),
		fmt.Sprintf("wrote %s (%d files)", ownershipinventory.ProviderSessionsRootGoInventoryRelativePath, len(candidates.ProviderSessionsRootGo.Files)),
		fmt.Sprintf("wrote %s (%d directories)", ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath, len(candidates.ProviderSessionsTopLevel.Children)),
	}
}

func hasExactlyOneTrailingNewline(payload []byte) bool {
	return len(payload) > 0 && payload[len(payload)-1] == '\n' && (len(payload) == 1 || payload[len(payload)-2] != '\n')
}

type stateObservingWriter struct {
	bytes.Buffer
	observe        func() error
	observed       bool
	observationErr error
}

func (writer *stateObservingWriter) Write(payload []byte) (int, error) {
	if !writer.observed {
		writer.observed = true
		writer.observationErr = writer.observe()
	}
	return writer.Buffer.Write(payload)
}

func copyCommandFixture(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()
	for _, relativePath := range []string{
		"pkg",
		"docs/internal/baselines",
		"docs/internal/projects/packaged-service-structure",
	} {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relativePath))
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
		if err := os.CopyFS(destinationPath, os.DirFS(sourcePath)); err != nil {
			t.Fatalf("copy command fixture %s: %v", relativePath, err)
		}
	}
}
