package climanifestparity_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestProductionCLIRootSessionFamily_RepresentativeCutover(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	assertHandwrittenRootSessionConstructors(t, repoRoot)
	assertGeneratorInfrastructurePresent(t, repoRoot)
	assertProductionRepresentativeCutoverWired(t, repoRoot)
	assertProductionFactoryConfigInitCutoverWired(t, repoRoot)
	assertProductionWorkCutoverWired(t, repoRoot)
	assertNoForbiddenCLIGeneratorMarkers(t, repoRoot)
}

func assertHandwrittenRootSessionConstructors(t *testing.T, repoRoot string) {
	t.Helper()
	rootGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root.go")
	rootWorkGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_work.go")
	rootWorkflowGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_workflow.go")
	for _, path := range []string{rootGo, rootWorkGo} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(contents)
		switch filepath.Base(path) {
		case "root.go":
			if !strings.Contains(text, "func NewRootCommand(") {
				t.Fatalf("%s must keep handwritten NewRootCommand constructor", path)
			}
		case "root_work.go":
			if !strings.Contains(text, "func newLegacyRootCommandWithOptions(") {
				t.Fatalf("%s must keep legacy rollback constructor", path)
			}
		}
	}
	sessionConstructorFiles := []string{rootWorkGo, rootWorkflowGo}
	foundSessionShow := false
	for _, path := range sessionConstructorFiles {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(contents), "func newSessionShowCommand(") {
			foundSessionShow = true
			break
		}
	}
	if !foundSessionShow {
		t.Fatalf("handwritten newSessionShowCommand must remain in %s or %s for rollback", rootWorkGo, rootWorkflowGo)
	}
}

func assertGeneratorInfrastructurePresent(t *testing.T, repoRoot string) {
	t.Helper()
	requiredPaths := []string{
		filepath.Join(repoRoot, "pkg", "transports", "cli", "climanifestgen"),
		filepath.Join(repoRoot, "pkg", "transports", "cli", "climanifestcobra"),
		filepath.Join(repoRoot, "pkg", "transports", "cli", "generated"),
		filepath.Join(repoRoot, "pkg", "transports", "cli", "commandregistry"),
	}
	for _, path := range requiredPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generator infrastructure path must exist: %s", path)
		}
	}
}

func assertProductionRepresentativeCutoverWired(t *testing.T, repoRoot string) {
	t.Helper()
	rootWorkGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_work.go")
	contents, err := os.ReadFile(rootWorkGo)
	if err != nil {
		t.Fatalf("read %s: %v", rootWorkGo, err)
	}
	text := string(contents)
	requiredMarkers := []string{
		"useGeneratedRepresentativeFamily = true",
		"newRootCommandWithGeneratedRepresentativeFamily",
		"climanifestcobra.NewRepresentativeFamilyComponents",
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(text, marker) {
			t.Fatalf("%s must wire production representative-family cutover via %q", rootWorkGo, marker)
		}
	}
}

func assertProductionFactoryConfigInitCutoverWired(t *testing.T, repoRoot string) {
	t.Helper()
	rootWorkGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_work.go")
	rootFactoryGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_factory.go")
	for _, path := range []string{rootWorkGo, rootFactoryGo} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(contents)
		switch filepath.Base(path) {
		case "root_work.go":
			if !strings.Contains(text, "productionFactoryConfigInitCommands") {
				t.Fatalf("%s must wire production factory/config/init cutover via %q", path, "productionFactoryConfigInitCommands")
			}
		case "root_factory.go":
			requiredMarkers := []string{
				"useGeneratedFactoryConfigInitFamily = true",
				"productionFactoryConfigInitCommands",
				"climanifestcobra.NewFactoryConfigInitFamilyComponents",
				"NewLegacyFactoryConfigInitFamilyCommands",
				"func newFactoryCommand(",
			}
			for _, marker := range requiredMarkers {
				if !strings.Contains(text, marker) {
					t.Fatalf("%s must wire production factory/config/init cutover via %q", path, marker)
				}
			}
		}
	}
}

func assertProductionWorkCutoverWired(t *testing.T, repoRoot string) {
	t.Helper()
	rootWorkGo := filepath.Join(repoRoot, "pkg", "transports", "cli", "root_work.go")
	contents, err := os.ReadFile(rootWorkGo)
	if err != nil {
		t.Fatalf("read %s: %v", rootWorkGo, err)
	}
	text := string(contents)
	requiredMarkers := []string{
		"useGeneratedWorkFamily = true",
		"productionWorkCommand",
		"climanifestcobra.NewWorkFamilyCommand",
		"productionWorkCommand(globals, diagnostics)",
		"func newWorkCommand(",
		"NewLegacyWorkFamilyCommand",
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(text, marker) {
			t.Fatalf("%s must wire production work-family cutover via %q", rootWorkGo, marker)
		}
	}
}

func assertNoForbiddenCLIGeneratorMarkers(t *testing.T, repoRoot string) {
	t.Helper()
	cliRoot := filepath.Join(repoRoot, "pkg", "transports", "cli")
	forbiddenMarkers := []string{
		"// Code generated by cli-command-manifest",
		"// Code generated by climanifestgen",
		"var commandHandlers = map[string]",
		"commandHandlersByStableID",
	}
	err := filepath.WalkDir(cliRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "climanifest", "climanifestparity", "climanifestgen", "climanifestcobra", "cliinputs", "commandidentity", "commandregistry", "baseline", "generated":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(contents)
		for _, marker := range forbiddenMarkers {
			if strings.Contains(text, marker) {
				t.Errorf("%s contains forbidden generator/cutover marker %q", path, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk CLI production tree: %v", err)
	}
}
