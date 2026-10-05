package analyzers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// CompilerOwners judges exact debt whose compilation unit may have vanished.
// It queries only debt-owning directories through the compiler, never discovers
// sources itself. Platform-inactive files remain compiler-provided metadata.
var CompilerOwners = &analysis.Analyzer{
	Name: "compilerowners",
	Doc:  "reject exact debt with vanished compiler owners or source files",
	Run: func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); !ok || unit != "internal/lint/analyzers" {
			return nil, nil
		}
		directory := filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)
		return CompilerOwnersForDirectory(directory).Run(pass)
	},
}

const compilerOwnerTags = "integration,functionallong,backendconformance,factoryartifact,managed_process_integration"

// CompilerOwnersForDirectory captures metadata before golangci's issue-cache
// lookup. Deleting an owner must invalidate cached success in the surviving
// baseline-owner package even when none of its Go source changed.
func CompilerOwnersForDirectory(directory string) *analysis.Analyzer {
	command := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := command.CombinedOutput()
	var head, metadata string
	if err == nil {
		root := strings.TrimSpace(string(output))
		var data []byte
		data, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(baselinePath)))
		head = string(data)
		if err == nil {
			metadata, err = collectCompilerOwners(head, runtime.GOOS, func(goos string, directories []string) (string, error) {
				args := append([]string{"list", "-e", "-test", "-tags=" + compilerOwnerTags, "-json"}, directories...)
				cmd := exec.Command("go", args...)
				cmd.Dir = root
				cmd.Env = compilerOwnerEnvironment(goos)
				out, failure := cmd.Output()
				if failure != nil {
					return "", fmt.Errorf("compiler metadata (%s): %w", goos, failure)
				}
				return string(out), nil
			})
		}
	}
	return compilerOwnersSnapshot(head, metadata, err)
}

func compilerOwnerEnvironment(goos string) []string {
	var result []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GOOS=") {
			result = append(result, entry)
		}
	}
	return append(result, "GOOS="+goos)
}

func compilerOwnersSnapshot(head, metadata string, err error) *analysis.Analyzer {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v", head, metadata, err)))
	analyzer := analysis.Analyzer{Doc: "reject exact debt with vanished compiler owners or source files"}
	analyzer.Name = fmt.Sprintf("compilerowners_%x", digest[:12])
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); !ok || unit != "internal/lint/analyzers" {
			return nil, nil
		}
		failure := err
		if failure == nil {
			var orphaned []string
			orphaned, failure = orphanCompilerKeys(head, metadata, modulePrefix)
			if len(orphaned) > 0 {
				failure = fmt.Errorf("vanished compiler owner/source; delete stale baseline keys:\n%s", strings.Join(orphaned, "\n"))
			}
		}
		if failure != nil {
			pass.Reportf(pass.Files[0].Package, "compiler-ownership: %s", failure)
		}
		return nil, nil
	}
	return &analyzer
}

func compilerOwnedKeys(text string) ([]string, error) {
	seen := map[string]bool{}
	var keys []string
	for _, line := range strings.Split(text, "\n") {
		key := strings.TrimSpace(line)
		if key == "" || strings.HasPrefix(key, "#") {
			continue
		}
		parts := strings.Split(key, "|")
		count := 3
		if parts[0] == "petri-public" {
			count = 4
		}
		if len(parts) != count || seen[key] {
			return nil, fmt.Errorf("malformed or duplicate baseline key: %s", key)
		}
		for _, part := range parts {
			if part == "" || strings.TrimSpace(part) != part {
				return nil, fmt.Errorf("malformed baseline key: %s", key)
			}
		}
		seen[key] = true
		if constructionSiteRules[parts[0]] {
			if err := validateConstructionSiteKey(parts); err != nil {
				return nil, err
			}
		}
		if strings.HasPrefix(parts[0], "testsleep-") {
			if err := validateTimingOwnerKey(parts); err != nil {
				return nil, err
			}
		}
		if ownsCompilerMetadata(parts[0]) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func ownsCompilerMetadata(rule string) bool {
	for _, prefix := range []string{"testsleep-", "service-root-", "functional-test-", "transport-recorded-site", "deprecated-runtime-api-"} {
		if strings.HasPrefix(rule, prefix) {
			return true
		}
	}
	return rule == "service-container-go-file" || rule == "test-cross-owner-policy" || rule == "test-transport-owner-policy" || rule == "petri-reference" || constructionSiteRules[rule]
}

func validateTimingOwnerKey(parts []string) error {
	kind := strings.TrimSuffix(strings.TrimPrefix(parts[0], "testsleep-"), "-test")
	site := strings.Split(parts[2], "::")
	if (kind != "sleep" && kind != "deadline" && kind != "elapsed") || len(site) != 4 ||
		!strings.HasSuffix(site[0], ".go") || site[1] == "" || site[2] != kind || !positiveDecimal(site[3]) {
		return fmt.Errorf("malformed timing baseline key: %s", strings.Join(parts, "|"))
	}
	return nil
}

func positiveDecimal(text string) bool {
	if text == "" || text[0] < '1' || text[0] > '9' {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compilerTargetFiles(key string) []string {
	parts := strings.Split(key, "|")
	if parts[0] == "service-root-unexpected-directory" || parts[2] == "<none>" {
		return nil
	}
	var files []string
	for _, target := range strings.Split(parts[2], ",") {
		files = append(files, strings.SplitN(strings.SplitN(strings.SplitN(target, "::", 2)[0], "#", 2)[0], ":", 2)[0])
	}
	return files
}

type compilerOwnerPackage struct {
	ImportPath                                         string
	GoFiles, TestGoFiles, XTestGoFiles, IgnoredGoFiles []string
	Error                                              *struct{ Err string }
	DepsErrors                                         []struct{ Err string }
}

func decodeCompilerOwners(text, prefix string) (map[string]compilerOwnerPackage, error) {
	units := map[string]compilerOwnerPackage{}
	decoder := json.NewDecoder(strings.NewReader(text))
	for decoder.More() {
		var unit compilerOwnerPackage
		if err := decoder.Decode(&unit); err != nil {
			return nil, fmt.Errorf("invalid compiler metadata: %w", err)
		}
		if len(unit.DepsErrors) > 0 {
			return nil, fmt.Errorf("incomplete compiler metadata: %s", unit.DepsErrors[0].Err)
		}
		if unit.Error != nil {
			// -e includes missing/platform-inactive packages. They contribute no
			// ownership; other compiler failures cannot establish a clean graph.
			message := unit.Error.Err
			if !strings.Contains(message, "build constraints exclude all Go files") && !strings.Contains(message, "no Go files") && !strings.Contains(message, "directory not found") && !strings.Contains(message, "cannot find package") {
				return nil, fmt.Errorf("incomplete compiler metadata: %s", message)
			}
			continue
		}
		identity := strings.SplitN(unit.ImportPath, " [", 2)[0]
		if !strings.HasPrefix(identity, prefix) || strings.HasSuffix(identity, ".test") {
			continue
		}
		key := strings.TrimPrefix(identity, prefix)
		previous := units[key]
		unit.GoFiles = append(unit.GoFiles, previous.GoFiles...)
		unit.TestGoFiles = append(unit.TestGoFiles, previous.TestGoFiles...)
		unit.XTestGoFiles = append(unit.XTestGoFiles, previous.XTestGoFiles...)
		unit.IgnoredGoFiles = append(unit.IgnoredGoFiles, previous.IgnoredGoFiles...)
		units[key] = unit
	}
	return units, nil
}

func orphanCompilerKeys(head, metadata, prefix string) ([]string, error) {
	keys, err := compilerOwnedKeys(head)
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	units, err := decodeCompilerOwners(metadata, prefix)
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("compiler metadata contains no repository units")
	}
	var orphaned []string
	for _, key := range keys {
		parts := strings.Split(key, "|")
		unit, exists := units[parts[1]]
		if !exists || !compilerUnitOwnsKey(unit, key) {
			orphaned = append(orphaned, key)
		}
	}
	return orphaned, nil
}

func compilerUnitOwnsKey(unit compilerOwnerPackage, key string) bool {
	parts := strings.Split(key, "|")
	files := append(append(append(append([]string{}, unit.GoFiles...), unit.TestGoFiles...), unit.XTestGoFiles...), unit.IgnoredGoFiles...)
	hasTests := strings.Contains(unit.ImportPath, " [") || strings.HasSuffix(parts[1], "_test")
	names := map[string]bool{}
	for _, file := range files {
		hasTests = hasTests || strings.HasSuffix(file, "_test.go")
		names[filepath.Base(file)] = true
	}
	if strings.HasPrefix(parts[0], "testsleep-") && strings.HasSuffix(parts[0], "-test") && !hasTests {
		return false
	}
	for _, target := range compilerTargetFiles(key) {
		if !names[filepath.Base(target)] {
			return false
		}
	}
	return true
}

func collectCompilerOwners(head, host string, query func(string, []string) (string, error)) (string, error) {
	keys, err := compilerOwnedKeys(head)
	if err != nil {
		return "", err
	}
	var metadata string
	seen := map[string]bool{}
	for _, goos := range []string{host, "linux", "windows", "darwin"} {
		if len(keys) == 0 {
			break
		}
		if seen[goos] {
			continue
		}
		seen[goos] = true
		dirs := map[string]bool{}
		for _, key := range keys {
			files := compilerTargetFiles(key)
			directory := strings.TrimSuffix(strings.Split(key, "|")[1], "_test")
			if len(files) > 0 {
				directory = filepath.ToSlash(filepath.Dir(files[0]))
			}
			dirs["./"+directory] = true
		}
		var directories []string
		for dir := range dirs {
			directories = append(directories, dir)
		}
		sort.Strings(directories)
		text, failure := query(goos, directories)
		if failure != nil {
			return "", failure
		}
		metadata += text + "\n"
		units, failure := decodeCompilerOwners(metadata, modulePrefix)
		if failure != nil {
			return "", failure
		}
		if len(units) == 0 {
			continue
		}
		keys, err = orphanCompilerKeys(head, metadata, modulePrefix)
		if err != nil {
			return "", err
		}
	}
	return metadata, nil
}
