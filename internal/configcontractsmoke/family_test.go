package configcontractsmoke

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFamiliesRegisterThreeDistinctProductionContracts(t *testing.T) {
	families := Families()
	if diagnostics := ValidateFamilies(families); len(diagnostics) != 0 {
		t.Fatalf("ValidateFamilies() diagnostics = %v", diagnostics)
	}
	if len(families) != 3 {
		t.Fatalf("Families() count = %d, want 3", len(families))
	}

	validDocuments := map[FamilyID][]byte{
		FamilyGlobal:     []byte(`{"defaults":{},"workerPresets":[]}`),
		FamilyMockWorker: []byte(`{"mockWorkers":[]}`),
		FamilyFactory:    []byte(`{"name":"registered-factory"}`),
	}
	for _, family := range families {
		if err := family.Parse(validDocuments[family.ID]); err != nil {
			t.Errorf("configuration family %q production parser rejected valid input: %v", family.ID, err)
		}
	}
}

func TestValidateFamiliesNamesMissingDuplicateAndCrossWiredPaths(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func([]Family) []Family
		wantCode string
		wantID   FamilyID
		wantPath string
	}{
		{
			name:     "missing",
			mutate:   func(families []Family) []Family { return families[1:] },
			wantCode: "config.family.missing", wantID: FamilyGlobal,
			wantPath: "contracts/config/you-config.schema.json",
		},
		{
			name:     "duplicate",
			mutate:   func(families []Family) []Family { return append(families, families[1]) },
			wantCode: "config.family.duplicate", wantID: FamilyMockWorker,
			wantPath: "contracts/config/mock-workers.schema.json",
		},
		{
			name: "cross-wired parser",
			mutate: func(families []Family) []Family {
				families[0].parser = families[1].parser
				return families
			},
			wantCode: "config.family.cross_wired", wantID: FamilyGlobal,
			wantPath: "contracts/config/you-config.schema.json",
		},
		{
			name: "cross-wired export",
			mutate: func(families []Family) []Family {
				families[2].ExportPath = families[0].ExportPath
				return families
			},
			wantCode: "config.family.cross_wired", wantID: FamilyFactory,
			wantPath: "packages/api/generated/schemas/you-config.schema.json",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := ValidateFamilies(test.mutate(Families()))
			if len(diagnostics) != 1 {
				t.Fatalf("ValidateFamilies() diagnostics = %v, want one", diagnostics)
			}
			diagnostic := diagnostics[0]
			if diagnostic.Code != test.wantCode || diagnostic.Family != test.wantID || diagnostic.Path != test.wantPath {
				t.Fatalf("diagnostic = %#v, want code=%q family=%q path=%q", diagnostic, test.wantCode, test.wantID, test.wantPath)
			}
			if message := diagnostic.Error(); !strings.Contains(message, string(test.wantID)) || !strings.Contains(message, test.wantPath) {
				t.Fatalf("diagnostic %q does not name family and path", message)
			}
		})
	}
}

func TestPublishedManifestExposesDistinctActiveConfigurationExports(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	payload, err := os.ReadFile(filepath.Join(repositoryRoot, "packages", "api", "generated", "manifest.json"))
	if err != nil {
		t.Fatalf("read package manifest: %v", err)
	}
	var manifest struct {
		Exports map[string]struct {
			Path         string `json:"path"`
			Family       string `json:"family"`
			ArtifactHash string `json:"artifactHash"`
			Lifecycle    struct {
				State string `json:"state"`
			} `json:"lifecycle"`
		} `json:"exports"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode package manifest: %v", err)
	}

	seen := make(map[string]FamilyID, 3)
	for _, family := range Families() {
		manifestPath := strings.TrimPrefix(family.ExportPath, "packages/api/")
		id := manifestID(manifestPath)
		export, ok := manifest.Exports[id]
		if !ok {
			t.Errorf("configuration family %q export %q is missing", family.ID, family.ExportPath)
			continue
		}
		if export.Path != manifestPath || export.Family != "config" || export.Lifecycle.State != "active" {
			t.Errorf("configuration family %q export = %#v", family.ID, export)
		}
		if prior, duplicate := seen[export.Path]; duplicate {
			t.Errorf("configuration families %q and %q share export %q", prior, family.ID, export.Path)
		}
		seen[export.Path] = family.ID
		artifact, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(family.ExportPath)))
		if err != nil {
			t.Errorf("read configuration family %q export: %v", family.ID, err)
			continue
		}
		digest := sha256.Sum256(artifact)
		if got := hex.EncodeToString(digest[:]); export.ArtifactHash != got {
			t.Errorf("configuration family %q artifactHash = %q, want %q", family.ID, export.ArtifactHash, got)
		}
	}
}

func manifestID(path string) string {
	withoutExtension := strings.TrimSuffix(strings.TrimSuffix(path, filepath.Ext(path)), ".schema")
	replacer := strings.NewReplacer("/", ".", "_", "-", "@", "")
	return strings.Trim(replacer.Replace(strings.ToLower(withoutExtension)), ".")
}
