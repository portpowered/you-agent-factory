package configcontractsmoke

import (
	"strings"
	"testing"
)

func TestFamiliesRegisterThreeDistinctProductionContracts(t *testing.T) {
	families := FamiliesWithParser(testGlobalParser)
	if diagnostics := ValidateFamilies(families, testGlobalParser); len(diagnostics) != 0 {
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
			diagnostics := ValidateFamilies(test.mutate(FamiliesWithParser(testGlobalParser)), testGlobalParser)
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
