package ownershipinventory_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestSnapshotBuildersMatchCommittedArtifacts(t *testing.T) {
	root := repositoryRoot(t)

	got, err := ownershipinventory.BuildSnapshotCandidates(root)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	wantOperatorSettingsRootGo, err := ownershipinventory.LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsRootGoInventory() error = %v", err)
	}
	wantOperatorSettingsTopLevel, err := ownershipinventory.LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadOperatorSettingsTopLevelInventory() error = %v", err)
	}
	wantProviderSessionsRootGo, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
	}
	wantProviderSessionsTopLevel, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}

	if !reflect.DeepEqual(got.OperatorSettingsRootGo, wantOperatorSettingsRootGo) {
		t.Fatalf("Operator Settings root-Go candidate differs from committed artifact: got=%#v want=%#v", got.OperatorSettingsRootGo, wantOperatorSettingsRootGo)
	}
	if !reflect.DeepEqual(got.OperatorSettingsTopLevel, wantOperatorSettingsTopLevel) {
		t.Fatalf("Operator Settings top-level candidate differs from committed artifact: got=%#v want=%#v", got.OperatorSettingsTopLevel, wantOperatorSettingsTopLevel)
	}
	if !reflect.DeepEqual(got.ProviderSessionsRootGo, wantProviderSessionsRootGo) {
		t.Fatalf("Provider Sessions root-Go candidate differs from committed artifact: got=%#v want=%#v", got.ProviderSessionsRootGo, wantProviderSessionsRootGo)
	}
	if !reflect.DeepEqual(got.ProviderSessionsTopLevel, wantProviderSessionsTopLevel) {
		t.Fatalf("Provider Sessions top-level candidate differs from committed artifact: got=%#v want=%#v", got.ProviderSessionsTopLevel, wantProviderSessionsTopLevel)
	}
	if got.OperatorSettingsRootGo.Clusters == nil || got.OperatorSettingsTopLevel.UnexpectedPublicSiblings == nil || got.ProviderSessionsTopLevel.UnexpectedPublicSiblingsBeyondService == nil {
		t.Fatal("empty candidate collections must serialize as [] rather than null")
	}
}

func TestWriteSnapshotCandidatesProducesCommittedCanonicalBytes(t *testing.T) {
	sourceRoot := repositoryRoot(t)
	destinationRoot := t.TempDir()
	candidates, err := ownershipinventory.BuildSnapshotCandidates(sourceRoot)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	if err := ownershipinventory.WriteSnapshotCandidates(destinationRoot, candidates); err != nil {
		t.Fatalf("WriteSnapshotCandidates() error = %v", err)
	}

	paths := []string{
		ownershipinventory.OperatorSettingsRootGoInventoryRelativePath,
		ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath,
		ownershipinventory.ProviderSessionsRootGoInventoryRelativePath,
		ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath,
	}
	for _, relativePath := range paths {
		want, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read committed %s: %v", relativePath, err)
		}
		got, err := os.ReadFile(filepath.Join(destinationRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read generated %s: %v", relativePath, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("generated %s is not byte-identical to committed artifact", relativePath)
		}
		if !bytes.HasSuffix(got, []byte("\n")) || bytes.HasSuffix(got[:len(got)-1], []byte("\n")) {
			t.Fatalf("generated %s must have exactly one trailing newline", relativePath)
		}
	}
}

func TestWriteSnapshotCandidatesRejectsInvalidCandidateBeforeReplacingAnyArtifact(t *testing.T) {
	sourceRoot := repositoryRoot(t)
	destinationRoot := t.TempDir()
	candidates, err := ownershipinventory.BuildSnapshotCandidates(sourceRoot)
	if err != nil {
		t.Fatalf("BuildSnapshotCandidates() error = %v", err)
	}
	candidates.OperatorSettingsRootGo.Files[0].Classification = "invalid_classification"

	sentinel := []byte("sentinel\n")
	paths := []string{
		ownershipinventory.OperatorSettingsRootGoInventoryRelativePath,
		ownershipinventory.OperatorSettingsTopLevelInventoryRelativePath,
		ownershipinventory.ProviderSessionsRootGoInventoryRelativePath,
		ownershipinventory.ProviderSessionsTopLevelInventoryRelativePath,
	}
	for _, relativePath := range paths {
		path := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relativePath, err)
		}
		if err := os.WriteFile(path, sentinel, 0o644); err != nil {
			t.Fatalf("write sentinel %s: %v", relativePath, err)
		}
	}

	err = ownershipinventory.WriteSnapshotCandidates(destinationRoot, candidates)
	if err == nil {
		t.Fatal("WriteSnapshotCandidates() error = nil, want invalid-candidate failure")
	}
	if !strings.Contains(err.Error(), "invalid_classification") && !strings.Contains(err.Error(), "unknown classification") {
		t.Fatalf("WriteSnapshotCandidates() error = %v, want classification invariant", err)
	}
	for _, relativePath := range paths {
		path := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read sentinel %s: %v", relativePath, readErr)
		}
		if !bytes.Equal(got, sentinel) {
			t.Fatalf("invalid candidate replaced %s", relativePath)
		}
	}
}

func TestSnapshotBuildersRejectUnclassifiedLiveUnits(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		build func(string) error
	}{
		{
			name: "operator settings root file",
			path: "pkg/services/operator_settings/surprise.go",
			build: func(root string) error {
				_, err := ownershipinventory.BuildOperatorSettingsRootGoInventory(root)
				return err
			},
		},
		{
			name: "operator settings top-level directory",
			path: "pkg/services/operator_settings/surprise",
			build: func(root string) error {
				_, err := ownershipinventory.BuildOperatorSettingsTopLevelInventory(root)
				return err
			},
		},
		{
			name: "provider sessions root file",
			path: "pkg/services/provider_sessions/surprise.go",
			build: func(root string) error {
				_, err := ownershipinventory.BuildProviderSessionsRootGoInventory(root)
				return err
			},
		},
		{
			name: "provider sessions top-level directory",
			path: "pkg/services/provider_sessions/surprise",
			build: func(root string) error {
				_, err := ownershipinventory.BuildProviderSessionsTopLevelInventory(root)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(test.path))
			if strings.HasSuffix(test.path, ".go") {
				writeFile(t, path, "package fixture\n")
			} else {
				mkdirAll(t, path)
			}
			err := test.build(root)
			if err == nil {
				t.Fatal("builder error = nil, want unclassified-unit failure")
			}
			if !strings.Contains(err.Error(), "surprise") || !strings.Contains(err.Error(), "unclassified") {
				t.Fatalf("builder error = %v, want named unclassified unit", err)
			}
		})
	}
}
