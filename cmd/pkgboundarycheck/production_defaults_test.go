package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestGeneratedArtifactStoreUsesInjectedMechanicsOutsideApplicationWire(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve production-default test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	findings, err := scanProductionDefaultSelections(repositoryRoot)
	if err != nil {
		t.Fatalf("scanProductionDefaultSelections() error = %v", err)
	}
	for _, finding := range findings {
		if finding.filePath == "pkg/platform/generatedartifacts/store.go" {
			t.Fatalf("generated artifact store retains ambient %s in %s", finding.symbol, finding.operation)
		}
	}
	for _, allowance := range productionDefaultAllowances {
		if allowance.filePath == "pkg/platform/generatedartifacts/store.go" {
			t.Fatalf("generated artifact store ambient-effect allowance remains: %#v", allowance)
		}
	}
	selections, err := readWireProductionSelections(repositoryRoot)
	if err != nil {
		t.Fatalf("readWireProductionSelections() error = %v", err)
	}
	for _, symbol := range []string{
		repositoryImportPrefix + "pkg/platform/generatedartifacts.LocalStore",
		repositoryImportPrefix + "pkg/platform/generatedartifacts.NewLocalStore",
	} {
		if _, selected := selections[symbol]; selected {
			t.Fatalf("application Wire contains unreachable generated-artifact selection %q", symbol)
		}
	}
	wantSelections := map[string]int{
		"cmd/climanifestgen/main.go":   1,
		"cmd/clicontractsmoke/main.go": 1,
		"cmd/mcpdiscoverygen/main.go":  1,
	}
	gotSelections, err := generatedArtifactStoreSelections(repositoryRoot)
	if err != nil {
		t.Fatalf("generatedArtifactStoreSelections() error = %v", err)
	}
	if len(gotSelections) != len(wantSelections) {
		t.Fatalf("generated artifact store selections = %#v, want exact developer-tool roots %#v", gotSelections, wantSelections)
	}
	for path, want := range wantSelections {
		if gotSelections[path] != want {
			t.Fatalf("generated artifact store selections[%q] = %d, want %d; all=%#v", path, gotSelections[path], want, gotSelections)
		}
	}
}

func generatedArtifactStoreSelections(repositoryRoot string) (map[string]int, error) {
	selections := map[string]int{}
	for _, root := range []string{"cmd", "pkg"} {
		err := filepath.WalkDir(filepath.Join(repositoryRoot, root), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			aliases := map[string]struct{}{}
			for _, spec := range parsed.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil || importPath != repositoryImportPrefix+"pkg/platform/generatedartifacts" {
					continue
				}
				alias := "generatedartifacts"
				if spec.Name != nil {
					alias = spec.Name.Name
				}
				aliases[alias] = struct{}{}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				if literal, ok := node.(*ast.CompositeLit); ok {
					selector, selected := literal.Type.(*ast.SelectorExpr)
					identifier, qualified := selectorExpressionIdentifier(selector)
					if selected && qualified && selector.Sel.Name == "LocalStore" {
						if _, imported := aliases[identifier.Name]; imported {
							relative, relErr := filepath.Rel(repositoryRoot, path)
							if relErr == nil {
								selections["direct-composite:"+filepath.ToSlash(relative)]++
							}
						}
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "NewLocalStore" {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				if _, selected := aliases[identifier.Name]; selected {
					relative, relErr := filepath.Rel(repositoryRoot, path)
					if relErr == nil {
						selections[filepath.ToSlash(relative)]++
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return selections, nil
}

func selectorExpressionIdentifier(selector *ast.SelectorExpr) (*ast.Ident, bool) {
	if selector == nil {
		return nil, false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return identifier, ok
}

func TestRunBlocksHiddenProductionDefaultsAcrossServicesAndAdapters(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/models/defaults.go", `package models

import (
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type leafClock struct{}
func (leafClock) Now() time.Time { return time.Now() }

func hiddenDefaults() {
	_, _ = os.UserHomeDir()
	_, _ = os.Getwd()
	_ = os.TempDir()
	_ = os.Getenv("MODEL")
	_ = time.Since(time.Now())
	_ = time.Until(time.Now())
	_ = uuid.NewString()
	_ = &http.Client{}
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want production default selection rejected")
	}
	for _, want := range []string{
		"hidden production environment default: os.UserHomeDir",
		"hidden production environment default: os.Getwd",
		"hidden production environment default: os.TempDir",
		"hidden production environment default: os.Getenv",
		"hidden production clock default: time.Now in leafClock.Now",
		"hidden production clock default: time.Since",
		"hidden production clock default: time.Until",
		"hidden production identity default: github.com/google/uuid.NewString",
		"hidden production http-client default: net/http.Client",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestRunBlocksProviderSessionAmbientEffects(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor/defaults.go", `package cursor

import (
	"database/sql"
	"path/filepath"
	"runtime"
)

func discover(root string) error {
	if err := filepath.WalkDir(root, nil); err != nil {
		return err
	}
	_, _ = filepath.EvalSymlinks(root)
	_, _ = sql.Open("sqlite", root)
	_ = runtime.GOOS
	return nil
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want Provider Sessions ambient effects rejected")
	}
	for _, want := range []string{
		"hidden production filesystem default: path/filepath.WalkDir in discover",
		"hidden production filesystem default: path/filepath.EvalSymlinks in discover",
		"hidden production database default: database/sql.Open in discover",
		"hidden production process default: runtime.GOOS in discover",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("run() stderr = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestProviderSessionAmbientEffectsUseExactDeletionOnlyBaselineKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kind   string
		symbol string
		source string
	}{
		{name: "walk directory", kind: "filesystem", symbol: "path/filepath.WalkDir", source: `package cursor
import "path/filepath"
func discover(root string) { _ = filepath.WalkDir(root, nil) }
`},
		{name: "evaluate symlinks", kind: "filesystem", symbol: "path/filepath.EvalSymlinks", source: `package cursor
import "path/filepath"
func discover(root string) { _, _ = filepath.EvalSymlinks(root) }
`},
		{name: "open database", kind: "database", symbol: "database/sql.Open", source: `package cursor
import "database/sql"
func discover(root string) { _, _ = sql.Open("sqlite", root) }
`},
		{name: "select operating system", kind: "process", symbol: "runtime.GOOS", source: `package cursor
import "runtime"
func discover(root string) { _ = runtime.GOOS }
`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			const filePath = "pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor/defaults.go"
			writeGoSourceFile(t, repoRoot, filePath, test.source)
			writeProductionDefaultTestBaseline(t, repoRoot, productionDefaultBaselineEntry{
				Kind: test.kind, Symbol: test.symbol, FilePath: filePath,
				Operation: "discover", Count: 1, Stage: productionDefaultSelectionBaselineStage,
				DeletionGate: productionDefaultSelectionDeletionGate,
			})

			stderr := &bytes.Buffer{}
			if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
				t.Fatalf("run() error = %v, want exact deletion-only baseline accepted; stderr=%q", err, stderr.String())
			}

			writeGoSourceFile(t, repoRoot, filePath, "package cursor\nfunc discover(root string) {}\n")
			stderr.Reset()
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil || !strings.Contains(stderr.String(), "stale production default selection baseline entry") {
				t.Fatalf("run() error = %v, stderr=%q, want exact stale-baseline rejection", err, stderr.String())
			}
		})
	}
}

func TestRunAllowsOnlyExactPolicyFreeLeafAdapterSelectedByWire(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/clock/clock.go", `package clock

import "time"

type Real struct{}
func (Real) Now() time.Time { return time.Now() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/platform/filesystem/local.go", `package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
)

type Local struct{}
func (Local) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (Local) Remove(path string) error { return os.Remove(path) }
func (Local) RemoveAll(path string) error { return os.RemoveAll(path) }
func (Local) Rename(from, to string) error { return os.Rename(from, to) }
func (Local) MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }
func (Local) WriteFile(path string, data []byte, mode os.FileMode) error { return os.WriteFile(path, data, mode) }
func (Local) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
func (Local) WalkDir(path string, fn fs.WalkDirFunc) error { return filepath.WalkDir(path, fn) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/platform/process/command.go", `package process

import (
	"context"
	"os/exec"
)

type ExecCommandRunner struct{}
type CommandRequest struct { Command string; Args []string }
type CommandResult struct{}
func (ExecCommandRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	_ = exec.Command(request.Command, request.Args...)
	return CommandResult{}, nil
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/profiles.go", `package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func provideClock() platformclock.Real { return platformclock.Real{} }
var filesystem = platformfilesystem.Local{}
var process = platformprocess.ExecCommandRunner{}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want exact Wire-selected leaf adapter allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunAllowsWireSelectedHostSpecificPlatformLeafAdapters(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/browser/open.go", `package browser

import (
	"context"
	"os/exec"
)

type Host struct{ operatingSystem string }
func NewHost(operatingSystem string) Host { return Host{operatingSystem: operatingSystem} }
func (host Host) Open(ctx context.Context, url string) error {
	cmd := exec.CommandContext(ctx, "browser", url)
	return cmd.Start()
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/platform/replay/storage.go", `package replay

import "os"

type Local struct{ operatingSystem string }
func NewLocal(operatingSystem string) Local { return Local{operatingSystem: operatingSystem} }
func (Local) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(path, 0o755); err != nil { return err }
	tmp, err := os.CreateTemp(path, "replay")
	if err != nil { return err }
	_ = os.Remove(tmp.Name())
	return os.Rename(tmp.Name(), path)
}
func (Local) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/platform.go", `package wire

import (
	"runtime"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
)

var browser = platformbrowser.NewHost(runtime.GOOS)
var replay = platformreplay.NewLocal(runtime.GOOS)
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want Wire-selected host adapters allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunDoesNotAcceptCommentAsWireSelectionProof(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/clock/clock.go", `package clock

import "time"

type Real struct{}
func (Real) Now() time.Time { return time.Now() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/profiles.go", `package wire

// platformclock.Real is intentionally only a comment, not a selection.
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "hidden production clock default: time.Now in Real.Now") {
		t.Fatalf("run() error = %v, stderr = %q, want comment-only marker rejected", err, stderr.String())
	}
}

func TestRunRejectsLeafAdapterWhenWireDoesNotSelectIt(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/clock/clock.go", `package clock

import "time"

type Real struct{}
func (Real) Now() time.Time { return time.Now() }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/profiles.go", "package wire\n")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil || !strings.Contains(stderr.String(), "hidden production clock default: time.Now in Real.Now") {
		t.Fatalf("run() error = %v, stderr = %q, want unselected leaf adapter rejected", err, stderr.String())
	}
}

func TestExecutableLocatorAllowanceRequiresParsedWireSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wireSource  string
		wantFinding bool
	}{
		{
			name: "selected composite literal",
			wireSource: `package wire
import platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
var executableLocator = platformprocess.HostExecutableLocator{}
`,
		},
		{
			name:        "unselected",
			wireSource:  "package wire\n",
			wantFinding: true,
		},
		{
			name: "comment only",
			wireSource: `package wire
// platformprocess.HostExecutableLocator{} is documentation, not a selection.
`,
			wantFinding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, "pkg/platform/process/executable.go", `package process
import "os/exec"
type HostExecutableLocator struct{}
func (HostExecutableLocator) LookPath(file string) (string, error) { return exec.LookPath(file) }
`)
			writeGoSourceFile(t, repoRoot, "pkg/wire/process.go", test.wireSource)

			findings, err := scanProductionDefaultSelections(repoRoot)
			if err != nil {
				t.Fatalf("scanProductionDefaultSelections() error = %v", err)
			}
			matched := false
			for _, finding := range findings {
				if finding.filePath == "pkg/platform/process/executable.go" &&
					finding.operation == "HostExecutableLocator.LookPath" &&
					finding.symbol == "os/exec.LookPath" {
					matched = true
				}
			}
			if matched != test.wantFinding {
				t.Fatalf("exact executable-locator finding = %v, want %v; all = %#v", matched, test.wantFinding, findings)
			}
		})
	}
}

func TestTemporaryFileSystemAllowanceRequiresParsedWireSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wireSource  string
		wantFinding bool
	}{
		{
			name: "selected composite literal",
			wireSource: `package wire
import platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
var temporaryFiles = platformfilesystem.Local{}
`,
		},
		{
			name:        "unselected",
			wireSource:  "package wire\n",
			wantFinding: true,
		},
		{
			name: "comment only",
			wireSource: `package wire
// platformfilesystem.Local{} is documentation, not a selection.
`,
			wantFinding: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, "pkg/platform/filesystem/local.go", `package filesystem
import "os"
type TemporaryFile interface{}
type Local struct{}
func (Local) CreateTemp(dir, pattern string) (TemporaryFile, error) { return os.CreateTemp(dir, pattern) }
`)
			writeGoSourceFile(t, repoRoot, "pkg/wire/filesystem.go", test.wireSource)

			findings, err := scanProductionDefaultSelections(repoRoot)
			if err != nil {
				t.Fatalf("scanProductionDefaultSelections() error = %v", err)
			}
			matched := false
			for _, finding := range findings {
				if finding.filePath == "pkg/platform/filesystem/local.go" &&
					finding.operation == "Local.CreateTemp" && finding.symbol == "os.CreateTemp" {
					matched = true
				}
			}
			if matched != test.wantFinding {
				t.Fatalf("exact temporary-filesystem finding = %v, want %v; all = %#v", matched, test.wantFinding, findings)
			}
		})
	}
}

func TestRunRecognizesDirectoryReplacementConstructorAsExactWireSelection(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/directoryreplace/replace.go", `package directoryreplace

import "os"

type Local struct{}
func NewLocal(string) Local { return Local{} }
func (Local) Commit(path string) error { return os.Remove(path) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/factorydefinitions/persistence.go", `package factorydefinitions

import directoryreplace "github.com/portpowered/infinite-you/pkg/platform/directoryreplace"

func provideReplacement() directoryreplace.Local {
	return directoryreplace.NewLocal("linux")
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want constructor-selected directory adapter allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunEnforcesDirectoryReplacementSelectionBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filePath    string
		packageName string
		selection   string
		wantError   bool
		wantSymbol  string
	}{
		{
			name:        "constructor in production helper is blocked",
			filePath:    "pkg/services/factory_definitions/internal/testcomposition/composition.go",
			packageName: "testcomposition",
			selection:   `directoryreplace.NewLocal("linux")`,
			wantError:   true,
			wantSymbol:  repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal",
		},
		{
			name:        "composite literal in production helper is blocked",
			filePath:    "pkg/services/factory_definitions/internal/testcomposition/composition.go",
			packageName: "testcomposition",
			selection:   "directoryreplace.Local{}",
			wantError:   true,
			wantSymbol:  repositoryImportPrefix + "pkg/platform/directoryreplace.Local",
		},
		{
			name:        "composite literal in Wire is allowed",
			filePath:    "pkg/wire/factory_definitions.go",
			packageName: "wire",
			selection:   "directoryreplace.Local{}",
		},
		{
			name:        "outer test selection is allowed",
			filePath:    "pkg/services/factory_definitions/internal/testcomposition/composition_test.go",
			packageName: "testcomposition",
			selection:   "directoryreplace.Local{}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, "pkg/platform/directoryreplace/replace.go", `package directoryreplace

type Local struct{}
func NewLocal(string) Local { return Local{} }
`)
			writeGoSourceFile(t, repoRoot, test.filePath, directoryReplacementSelectionSource(test.packageName, test.selection))

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if !test.wantError {
				if err != nil {
					t.Fatalf("run() error = %v, want selection allowed; stderr=%q", err, stderr.String())
				}
				return
			}
			if err == nil {
				t.Fatal("run() error = nil, want production directory-replacement selection rejected")
			}
			wantDiagnostic := "hidden production platform-adapter-selection default: " + test.wantSymbol + " in package (" + test.filePath
			if !strings.Contains(stderr.String(), wantDiagnostic) {
				t.Fatalf("run() stderr = %q, want diagnostic %q", stderr.String(), wantDiagnostic)
			}
		})
	}
}

func directoryReplacementSelectionSource(packageName, selection string) string {
	return "package " + packageName + "\n\n" +
		"import directoryreplace \"" + repositoryImportPrefix + "pkg/platform/directoryreplace\"\n\n" +
		"var replacement = " + selection + "\n"
}

func TestPlatformAdapterSelectionIsRestrictedToWireAndOuterTests(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	selectionSource := `package testcomposition
import (
	directoryreplace "github.com/portpowered/infinite-you/pkg/platform/directoryreplace"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/inboxgitkeep"
)
var files = platformfilesystem.Local{}
var inbox = inboxgitkeep.NewLocal(files)
var replacement = directoryreplace.NewLocal("linux")
var zeroReplacement = directoryreplace.Local{}
`
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_definitions/internal/testcomposition/composition.go", selectionSource)
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_definitions/internal/testcomposition/composition_test.go", selectionSource)
	writeGoSourceFile(t, repoRoot, "pkg/wire/factory_definitions.go", selectionSource)

	findings, err := scanProductionDefaultSelections(repoRoot)
	if err != nil {
		t.Fatalf("scanProductionDefaultSelections() error = %v", err)
	}
	var selected []productionDefaultFinding
	for _, finding := range findings {
		if finding.kind == "platform-adapter-selection" {
			selected = append(selected, finding)
		}
	}
	if len(selected) != 4 {
		t.Fatalf("Platform adapter selections = %#v, want four normal-helper findings", selected)
	}
	for _, finding := range selected {
		if finding.filePath != "pkg/services/factory_definitions/internal/testcomposition/composition.go" {
			t.Fatalf("Platform adapter selection escaped normal helper: %#v", finding)
		}
	}
}

func TestRunRejectsAuthoredLayoutFilesystemLeafWhenWireDoesNotSelectIt(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/filesystem/local.go", `package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
)

type Local struct{}
func (Local) RemoveAll(path string) error { return os.RemoveAll(path) }
func (Local) MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }
func (Local) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (Local) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
func (Local) WalkDir(path string, fn fs.WalkDirFunc) error { return filepath.WalkDir(path, fn) }
`)
	writeGoSourceFile(t, repoRoot, "pkg/wire/factory_definitions.go", "package wire\n")

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil ||
		!strings.Contains(stderr.String(), "hidden production filesystem default: os.RemoveAll in Local.RemoveAll") ||
		!strings.Contains(stderr.String(), "hidden production filesystem default: os.MkdirTemp in Local.MkdirTemp") ||
		!strings.Contains(stderr.String(), "hidden production filesystem default: os.WriteFile in Local.WriteFile") ||
		!strings.Contains(stderr.String(), "hidden production filesystem default: path/filepath.EvalSymlinks in Local.EvalSymlinks") ||
		!strings.Contains(stderr.String(), "hidden production filesystem default: path/filepath.WalkDir in Local.WalkDir") {
		t.Fatalf("run() error = %v, stderr = %q, want unselected authored-layout filesystem leaf rejected", err, stderr.String())
	}
}

func TestProductionDefaultBaselineRejectsCountIncreaseAndStaleEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantStale  bool
		wantCount2 bool
	}{
		{name: "count increase", source: "package models\nimport \"time\"\nfunc now() { _, _ = time.Now(), time.Now() }\n", wantCount2: true},
		{name: "stale", source: "package models\nfunc now() {}\n", wantStale: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, "pkg/services/models/default.go", test.source)
			writeProductionDefaultTestBaseline(t, repoRoot, productionDefaultBaselineEntry{
				Kind: "clock", Symbol: "time.Now", FilePath: "pkg/services/models/default.go",
				Operation: "now", Count: 1, Stage: productionDefaultSelectionBaselineStage,
				DeletionGate: productionDefaultSelectionDeletionGate,
			})

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want deletion-only baseline violation")
			}
			if test.wantCount2 && !strings.Contains(stderr.String(), "2 occurrence(s)") {
				t.Fatalf("stderr = %q, want count increase diagnostic", stderr.String())
			}
			if test.wantStale && !strings.Contains(stderr.String(), "stale production default selection baseline entry") {
				t.Fatalf("stderr = %q, want stale entry diagnostic", stderr.String())
			}
		})
	}
}

func TestProductionDefaultBaselineRejectsEmptyFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/models/model.go", "package models\n")
	payload, err := json.Marshal(productionDefaultBaseline{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, productionDefaultSelectionBaselinePath), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	err = run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "is empty; delete the file to record zero debt") {
		t.Fatalf("run() error = %v, want empty baseline rejected", err)
	}
}

func writeProductionDefaultTestBaseline(
	t *testing.T,
	repoRoot string,
	entries ...productionDefaultBaselineEntry,
) {
	t.Helper()
	payload, err := json.Marshal(productionDefaultBaseline{Version: 1, Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, productionDefaultSelectionBaselinePath), payload, 0o644); err != nil {
		t.Fatal(err)
	}
}
