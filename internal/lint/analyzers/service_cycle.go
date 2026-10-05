package analyzers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/tools/go/analysis"
)

// ServiceCycle enforces the exact weighted, bidirectional service cycle ratchet.
var ServiceCycle = &analysis.Analyzer{
	Name: "servicecycle",
	Doc:  "enforce the exact weighted service cycle ceiling",
	Run: func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); ok && unit == "internal/lint/analyzers" {
			directory := filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)
			return ServiceCycleForDirectory(directory).Run(pass)
		}
		return nil, nil
	},
}

type serviceCyclePackage struct {
	Dir, ImportPath                   string
	GoFiles, CgoFiles, IgnoredGoFiles []string
	Error                             *struct{ Err string }
	DepsErrors                        []struct{ Err string }
}

type serviceCycleInput struct {
	Head, Metadata string
	Sources        map[string]string
}

// ServiceCycleForDirectory captures all external inputs before the issue-cache
// lookup. Git nominates directories; only the compiler names source files.
// Each plugin invocation owns its snapshot, including failures and deletions.
func ServiceCycleForDirectory(directory string) *analysis.Analyzer {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	run := func(root, executable string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Dir = root
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("service-cycle metadata %s: %w", executable, err)
		}
		return string(output), nil
	}
	input, err := collectServiceCycleInput(directory, run, os.ReadFile, os.Stat)
	return serviceCycleSnapshot(input, err)
}

func collectServiceCycleInput(directory string, run func(string, string, ...string) (string, error), read func(string) ([]byte, error), stat func(string) (os.FileInfo, error)) (serviceCycleInput, error) {
	input := serviceCycleInput{Sources: map[string]string{}}
	root, err := run(directory, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return input, err
	}
	root = strings.TrimSpace(root)
	head, err := read(filepath.Join(root, filepath.FromSlash(baselinePath)))
	if err != nil {
		return input, fmt.Errorf("read cycle baseline: %w", err)
	}
	input.Head = string(head)
	paths, err := run(root, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "pkg/services")
	if err != nil {
		return input, err
	}
	directories, err := serviceCycleDirectories(root, paths, stat)
	if err != nil {
		return input, err
	}
	// Keep explicit queries below Windows command-line limits. Chunks are a
	// single snapshot, and any failed chunk invalidates the whole result.
	for start := 0; start < len(directories); start += 64 {
		end := min(start+64, len(directories))
		args := append([]string{"list", "-e", "-tags=" + compilerOwnerTags, "-json"}, directories[start:end]...)
		metadata, failure := run(root, "go", args...)
		if failure != nil {
			return input, failure
		}
		input.Metadata += metadata + "\n"
	}
	files, err := serviceCycleFiles(input.Metadata)
	if err != nil {
		return input, err
	}
	owners := map[string]bool{}
	for _, file := range files {
		owners["./"+pathDirectory(file)] = true
	}
	for _, directory := range directories {
		if !owners[directory] {
			return input, fmt.Errorf("incomplete service-cycle compiler metadata: missing owner %s", directory)
		}
	}
	for _, file := range files {
		content, failure := read(filepath.Join(root, filepath.FromSlash(file)))
		if failure != nil {
			return input, fmt.Errorf("read service-cycle source %s: %w", file, failure)
		}
		input.Sources[file] = string(content)
	}
	return input, nil
}

func serviceCycleDirectories(root, paths string, stat func(string) (os.FileInfo, error)) ([]string, error) {
	seen := map[string]bool{}
	for _, name := range strings.Split(paths, "\x00") {
		if !serviceCycleSourcePath(name) {
			continue
		}
		if _, err := stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			if os.IsNotExist(err) {
				continue
			} // Git's index retains worktree deletions.
			return nil, fmt.Errorf("stat service source %s: %w", name, err)
		}
		seen["./"+pathDirectory(name)] = true
	}
	var result []string
	for directory := range seen {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result, nil
}

func serviceCycleSourcePath(name string) bool {
	if _, ok := serviceOwnerOf(name); !ok || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".git" || segment == "node_modules" || segment == "testdata" || segment == "vendor" || segment == ".." {
			return false
		}
	}
	return true
}

func serviceCycleFiles(metadata string) ([]string, error) {
	decoder := json.NewDecoder(strings.NewReader(metadata))
	seen := map[string]bool{}
	for {
		var unit serviceCyclePackage
		if err := decoder.Decode(&unit); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("invalid service-cycle compiler metadata: %w", err)
		}
		if len(unit.DepsErrors) > 0 {
			return nil, fmt.Errorf("incomplete service-cycle metadata: %s", unit.DepsErrors[0].Err)
		}
		if unit.Error != nil && (!strings.Contains(unit.Error.Err, "build constraints exclude all Go files") || len(unit.IgnoredGoFiles) == 0) {
			return nil, fmt.Errorf("incomplete service-cycle metadata: %s", unit.Error.Err)
		}
		relative, ok := strings.CutPrefix(unit.ImportPath, modulePrefix)
		if !ok || !under(relative, "pkg/services") {
			return nil, fmt.Errorf("unexpected service-cycle compiler owner: %s", unit.ImportPath)
		}
		files := append(append(append([]string{}, unit.GoFiles...), unit.CgoFiles...), unit.IgnoredGoFiles...)
		if len(files) == 0 {
			return nil, fmt.Errorf("incomplete service-cycle owner %s: no named sources", unit.ImportPath)
		}
		for _, file := range files {
			if filepath.Base(file) != file {
				return nil, fmt.Errorf("invalid compiler source name: %s", file)
			}
			name := relative + "/" + file
			if serviceCycleSourcePath(name) {
				seen[name] = true
			}
		}
	}
	var result []string
	for file := range seen {
		result = append(result, file)
	}
	sort.Strings(result)
	return result, nil
}

func serviceCycleGraph(input serviceCycleInput) (*serviceGraph, error) {
	files, err := serviceCycleFiles(input.Metadata)
	if err != nil {
		return nil, err
	}
	graph := &serviceGraph{weights: map[serviceEdge]int{}, carriers: map[serviceEdge][]string{}}
	known := map[string]bool{}
	for _, file := range files {
		owner, _ := serviceOwnerOf(file)
		known[owner] = true
	}
	for owner := range known {
		graph.services = append(graph.services, owner)
	}
	sort.Strings(graph.services)
	for _, file := range files {
		content, present := input.Sources[file]
		if !present {
			return nil, fmt.Errorf("missing compiler-named source: %s", file)
		}
		parsed, failure := parser.ParseFile(token.NewFileSet(), file, content, parser.ImportsOnly)
		if failure != nil {
			return nil, fmt.Errorf("parse service-cycle source %s: %w", file, failure)
		}
		owner, _ := serviceOwnerOf(file)
		for _, spec := range parsed.Imports {
			path, failure := strconv.Unquote(spec.Path.Value)
			if failure != nil {
				return nil, fmt.Errorf("parse import %s: %w", file, failure)
			}
			target, ok := importedServiceOf(path)
			if ok && target != owner && known[target] {
				graph.recordImport(serviceEdge{owner, target}, pathDirectory(file))
			}
		}
	}
	graph.sortCarriers()
	return graph, nil
}

func serviceCycleSnapshot(input serviceCycleInput, failure error) *analysis.Analyzer {
	input.Sources = maps.Clone(input.Sources)
	data, _ := json.Marshal(input) // JSON sorts map keys; source deletions change the digest.
	digest := sha256.Sum256(append(data, []byte(fmt.Sprintf("\x00%s\x00%v", compilerOwnerTags, failure))...))
	analyzer := analysis.Analyzer{Doc: "enforce the exact weighted service cycle ceiling"}
	analyzer.Name = fmt.Sprintf("servicecycle_%x", digest[:12])
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); !ok || unit != "internal/lint/analyzers" {
			return nil, nil
		}
		err := failure
		if err == nil {
			var graph *serviceGraph
			graph, err = serviceCycleGraph(input)
			if err == nil {
				err = serviceCycleDiagnostic(graph, input.Head)
			}
		}
		if err != nil {
			pass.Reportf(pass.Files[0].Package, "service-cycle-weight: %s", err)
		}
		return nil, nil
	}
	return &analyzer
}
