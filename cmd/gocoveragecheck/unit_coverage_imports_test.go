package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareUnitCoverageImportFilePreservesTestFreeCoveragePackages(t *testing.T) {
	packageDir := t.TempDir()
	testPackage := modulePath + "/pkg/coverageimports/testful"
	listings := []coveragePackageListing{
		{
			importPath:  modulePath + "/pkg/coverageimports/generated",
			directory:   filepath.Join(packageDir, "generated"),
			packageName: "generated",
			goFiles:     1,
		},
		{
			importPath:  testPackage,
			directory:   packageDir,
			packageName: "testful",
			goFiles:     1,
			testGoFiles: []string{"testful_test.go"},
			deps:        []string{modulePath + "/pkg/coverageimports/generated"},
		},
		{
			importPath:  modulePath + "/pkg/coverageimports/testless",
			directory:   filepath.Join(packageDir, "testless"),
			packageName: "testless",
			goFiles:     1,
		},
	}

	cleanup, err := prepareUnitCoverageImportFile([]string{testPackage}, listings)
	if err != nil {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v", err)
	}
	files, err := filepath.Glob(filepath.Join(packageDir, unitCoverageImportFilename))
	if err != nil {
		t.Fatalf("find temporary coverage import file: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("temporary coverage import files = %v, want one", files)
	}
	contents, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read temporary coverage import file: %v", err)
	}
	text := string(contents)
	if !strings.HasPrefix(text, "package testful\n\nimport (\n") {
		t.Fatalf("temporary coverage import file = %q, want target package declaration", text)
	}
	if strings.Contains(text, modulePath+"/pkg/coverageimports/generated") {
		t.Fatalf("temporary coverage import file = %q, redundantly imported selected test dependency", text)
	}
	for _, importPath := range []string{modulePath + "/pkg/coverageimports/testless"} {
		if !strings.Contains(text, "_ \""+importPath+"\"") {
			t.Fatalf("temporary coverage import file = %q, missing %s", text, importPath)
		}
	}
	if strings.Contains(text, testPackage) {
		t.Fatalf("temporary coverage import file imported selected test package: %q", text)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("temporary coverage import cleanup: %v", err)
	}
	if _, err := os.Stat(files[0]); !os.IsNotExist(err) {
		t.Fatalf("temporary coverage import file stat error after cleanup = %v, want not-exist", err)
	}
}

func TestPrepareUnitCoverageImportFileSkipsSelectedTestDependencies(t *testing.T) {
	packageDir := t.TempDir()
	testPackage := modulePath + "/pkg/coverageimports/testful"
	dependency := modulePath + "/pkg/coverageimports/already-imported"
	cleanup, err := prepareUnitCoverageImportFile(
		[]string{testPackage},
		[]coveragePackageListing{
			{importPath: testPackage, directory: packageDir, packageName: "testful", goFiles: 1, testGoFiles: []string{"testful_test.go"}, deps: []string{dependency}},
			{importPath: dependency, goFiles: 1},
		},
	)
	if err != nil {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("temporary coverage import cleanup: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(packageDir, unitCoverageImportFilename))
	if err != nil {
		t.Fatalf("find temporary coverage import file: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("temporary coverage import files = %v, want none for an already-imported package", files)
	}
}

func TestPrepareUnitCoverageImportFileFailsClosedOnMissingMetadata(t *testing.T) {
	_, err := prepareUnitCoverageImportFile(
		[]string{modulePath + "/pkg/coverageimports/testful"},
		[]coveragePackageListing{
			{
				importPath:  modulePath + "/pkg/coverageimports/testful",
				packageName: "testful",
				goFiles:     1,
				testGoFiles: []string{"testful_test.go"},
			},
			{
				importPath: modulePath + "/pkg/coverageimports/testless",
				goFiles:    1,
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "Dir and Name are required") {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v, want incomplete target metadata diagnostic", err)
	}
}

func TestPrepareUnitCoverageImportFileRespectsInternalVisibility(t *testing.T) {
	root := t.TempDir()
	generalDir := filepath.Join(root, "general")
	internalDir := filepath.Join(root, "internal")
	for _, directory := range []string{generalDir, internalDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create carrier directory %q: %v", directory, err)
		}
	}

	generalCarrier := modulePath + "/pkg/coverageimports/carrier"
	internalCarrier := modulePath + "/pkg/services/automations"
	generalTestFree := modulePath + "/pkg/coverageimports/testless"
	internalTestFree := modulePath + "/pkg/services/automations/internal/cron"
	cleanup, err := prepareUnitCoverageImportFile(
		[]string{generalCarrier, internalCarrier},
		[]coveragePackageListing{
			{importPath: generalCarrier, directory: generalDir, packageName: "carrier", goFiles: 1, testGoFiles: []string{"carrier_test.go"}},
			{importPath: internalCarrier, directory: internalDir, packageName: "automations", goFiles: 1, testGoFiles: []string{"automations_test.go"}},
			{importPath: generalTestFree, goFiles: 1},
			{importPath: internalTestFree, goFiles: 1},
		},
	)
	if err != nil {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Errorf("temporary coverage import cleanup: %v", cleanupErr)
		}
	})

	generalFiles, err := filepath.Glob(filepath.Join(generalDir, unitCoverageImportFilename))
	if err != nil {
		t.Fatalf("find general carrier file: %v", err)
	}
	internalFiles, err := filepath.Glob(filepath.Join(internalDir, unitCoverageImportFilename))
	if err != nil {
		t.Fatalf("find internal carrier file: %v", err)
	}
	if len(generalFiles) != 1 || len(internalFiles) != 1 {
		t.Fatalf("temporary carrier files = general %v, internal %v; want one per legal carrier", generalFiles, internalFiles)
	}

	generalSource, err := os.ReadFile(generalFiles[0])
	if err != nil {
		t.Fatalf("read general carrier file: %v", err)
	}
	if !strings.Contains(string(generalSource), "_ \""+generalTestFree+"\"") || strings.Contains(string(generalSource), internalTestFree) {
		t.Fatalf("general carrier source = %q, want only the general test-free package", generalSource)
	}
	internalSource, err := os.ReadFile(internalFiles[0])
	if err != nil {
		t.Fatalf("read internal carrier file: %v", err)
	}
	if !strings.Contains(string(internalSource), "_ \""+internalTestFree+"\"") || strings.Contains(string(internalSource), generalTestFree) {
		t.Fatalf("internal carrier source = %q, want only the internal test-free package", internalSource)
	}
}

func TestCanUnitCoverageImport(t *testing.T) {
	tests := []struct {
		name     string
		importer string
		imported string
		want     bool
	}{
		{
			name:     "external package",
			importer: modulePath + "/pkg/coverageimports/carrier",
			imported: modulePath + "/pkg/coverageimports/testless",
			want:     true,
		},
		{
			name:     "internal parent",
			importer: modulePath + "/pkg/services/automations",
			imported: modulePath + "/pkg/services/automations/internal/cron",
			want:     true,
		},
		{
			name:     "internal descendant",
			importer: modulePath + "/pkg/services/automations/wire",
			imported: modulePath + "/pkg/services/automations/internal/cron",
			want:     true,
		},
		{
			name:     "internal sibling",
			importer: modulePath + "/pkg/services/work",
			imported: modulePath + "/pkg/services/automations/internal/cron",
			want:     false,
		},
		{
			name:     "self",
			importer: modulePath + "/pkg/coverageimports/carrier",
			imported: modulePath + "/pkg/coverageimports/carrier",
			want:     false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canUnitCoverageImport(test.importer, test.imported); got != test.want {
				t.Fatalf("canUnitCoverageImport(%q, %q) = %t, want %t", test.importer, test.imported, got, test.want)
			}
		})
	}
}

func TestChooseUnitCoverageImportCarrierSkipsDependencyCycle(t *testing.T) {
	testFree := modulePath + "/pkg/services/factory_definitions/internal/services/catalog"
	closest := modulePath + "/pkg/services/factory_definitions/internal/services/catalog/wire"
	fallback := modulePath + "/pkg/services/factory_definitions"
	carriers := []unitCoverageImportCarrier{
		{listing: coveragePackageListing{importPath: closest}},
		{listing: coveragePackageListing{importPath: fallback}},
	}

	index, ok := chooseUnitCoverageImportCarrier(testFree, []string{closest}, carriers)
	if !ok || carriers[index].listing.importPath != fallback {
		t.Fatalf("chooseUnitCoverageImportCarrier() = (%d, %t), want fallback carrier %q", index, ok, fallback)
	}
}

func TestChooseUnitCoverageImportCarrierPrefersLoadedEqualPrefixCarrier(t *testing.T) {
	testFree := modulePath + "/pkg/services/automations/internal/services/cron"
	loaded := modulePath + "/pkg/services/automations/internal/services/cron/loaded"
	empty := modulePath + "/pkg/services/automations/internal/services/cron/empty"
	carriers := []unitCoverageImportCarrier{
		{listing: coveragePackageListing{importPath: empty}},
		{listing: coveragePackageListing{importPath: loaded}, imports: []string{"example.com/already-grouped"}},
	}

	index, ok := chooseUnitCoverageImportCarrier(testFree, nil, carriers)
	if !ok || carriers[index].listing.importPath != loaded {
		t.Fatalf("chooseUnitCoverageImportCarrier() = (%d, %t), want loaded carrier %q", index, ok, loaded)
	}
}
