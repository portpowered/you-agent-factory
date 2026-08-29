package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const productionDefaultSelectionBaselinePath = "production-default-selection-baseline.json"
const productionDefaultSelectionBaselineStage = "wire-injection-full-blow"
const productionDefaultSelectionDeletionGate = "inject the exact external effect or select the policy-free leaf adapter in pkg/wire, then delete this exact entry"

type productionDefaultFinding struct {
	kind      string
	symbol    string
	filePath  string
	operation string
	line      int
	count     int
}

type productionDefaultBaseline struct {
	Version int                              `json:"version"`
	Entries []productionDefaultBaselineEntry `json:"entries"`
}

type productionDefaultBaselineEntry struct {
	Kind         string `json:"kind"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Operation    string `json:"operation"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

// productionDefaultSymbols are process-global effects whose selection must
// occur at the canonical Wire boundary. Operational code consumes an injected
// port; it must not silently select one of these production implementations.
var productionDefaultSymbols = map[string]string{
	"os.Args": "process", "os.Chdir": "filesystem", "os.Create": "filesystem",
	"os.CreateTemp": "filesystem", "os.DirFS": "filesystem", "os.Environ": "environment",
	"os.Executable": "process", "os.File": "filesystem", "os.Getenv": "environment",
	"os.Getwd": "environment", "os.LookupEnv": "environment", "os.Mkdir": "filesystem",
	"os.MkdirAll": "filesystem", "os.MkdirTemp": "filesystem", "os.Open": "filesystem",
	"os.OpenFile": "filesystem", "os.ReadDir": "filesystem", "os.ReadFile": "filesystem",
	"os.Remove": "filesystem", "os.RemoveAll": "filesystem", "os.Rename": "filesystem",
	"os.Stat": "filesystem", "os.Stderr": "process", "os.Stdin": "process",
	"os.Stdout": "process", "os.TempDir": "environment", "os.UserCacheDir": "environment",
	"os.UserConfigDir": "environment", "os.UserHomeDir": "environment", "os.WriteFile": "filesystem",
	"os.CopyFS":             "filesystem",
	"path/filepath.WalkDir": "filesystem", "path/filepath.EvalSymlinks": "filesystem",

	"database/sql.Open": "database",
	"runtime.GOOS":      "process",

	"os/exec.Command": "process", "os/exec.CommandContext": "process", "os/exec.LookPath": "process",

	"time.Now": "clock", "time.Since": "clock", "time.Until": "clock",

	"github.com/google/uuid.New": "identity", "github.com/google/uuid.NewString": "identity",
	"github.com/google/uuid.NewRandom": "identity", "github.com/google/uuid.NewRandomFromReader": "identity",
	"crypto/rand.Read": "random", "crypto/rand.Reader": "random",

	"math/rand.Float32": "random", "math/rand.Float64": "random", "math/rand.Int": "random",
	"math/rand.Int31": "random", "math/rand.Int31n": "random", "math/rand.Int63": "random",
	"math/rand.Int63n": "random", "math/rand.Intn": "random", "math/rand.New": "random",
	"math/rand.NewSource": "random", "math/rand.Perm": "random", "math/rand.Read": "random",
	"math/rand.Shuffle": "random", "math/rand.Uint32": "random", "math/rand.Uint64": "random",
	"math/rand/v2.Float32": "random", "math/rand/v2.Float64": "random", "math/rand/v2.Int": "random",
	"math/rand/v2.Int32": "random", "math/rand/v2.Int32N": "random", "math/rand/v2.Int64": "random",
	"math/rand/v2.Int64N": "random", "math/rand/v2.IntN": "random", "math/rand/v2.New": "random",
	"math/rand/v2.Perm": "random", "math/rand/v2.Shuffle": "random", "math/rand/v2.Uint": "random",
	"math/rand/v2.Uint32": "random", "math/rand/v2.Uint32N": "random", "math/rand/v2.Uint64": "random",
	"math/rand/v2.Uint64N": "random", "math/rand/v2.UintN": "random",

	"net/http.Client": "http-client", "net/http.DefaultClient": "http-client",
	"net/http.DefaultTransport": "http-client", "net/http.Get": "http-client",
	"net/http.Head": "http-client", "net/http.Post": "http-client", "net/http.PostForm": "http-client",
}

// platformAdapterSelectionSymbols are policy-free implementations that may be
// selected only by canonical Wire or by an outer _test.go edge. Normal
// compiled helpers must receive their owner-defined capabilities explicitly.
var platformAdapterSelectionSymbols = map[string]struct{}{
	repositoryImportPrefix + "pkg/platform/directoryreplace.Local":    {},
	repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal": {},
	repositoryImportPrefix + "pkg/platform/filesystem.Local":          {},
	repositoryImportPrefix + "pkg/platform/inboxgitkeep.NewLocal":     {},
	repositoryImportPrefix + "pkg/platform/locking.LocalFileSystem":   {},
	repositoryImportPrefix + "pkg/platform/locking.New":               {},
}

type productionDefaultAllowance struct {
	filePath   string
	operation  string
	symbol     string
	wireSymbol string
}

// These are exact policy-free leaf adapters. Each allowance remains valid only
// while canonical Wire source explicitly selects that adapter. This is not a
// package exemption: another function or file using the same standard-library
// effect is reported normally.
var productionDefaultAllowances = []productionDefaultAllowance{
	{filePath: "pkg/platform/clock/clock.go", operation: "Real.Now", symbol: "time.Now", wireSymbol: repositoryImportPrefix + "pkg/platform/clock.Real"},
	{filePath: "pkg/platform/browser/open.go", operation: "Host.Open", symbol: "os/exec.CommandContext", wireSymbol: repositoryImportPrefix + "pkg/platform/browser.NewHost"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.MkdirTemp", symbol: "os.MkdirTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.WriteFile", symbol: "os.WriteFile", wireSymbol: repositoryImportPrefix + "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.Stat", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.RemoveAll", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "Random.Read", symbol: "crypto/rand.Read", wireSymbol: repositoryImportPrefix + "pkg/platform/contentstaging.Random"},
	{filePath: "pkg/platform/random/crypto.go", operation: "CryptoSource.Int63n", symbol: "crypto/rand.Reader", wireSymbol: repositoryImportPrefix + "pkg/platform/random.CryptoSource"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Open", symbol: "os.Open", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Create", symbol: "os.Create", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.OpenFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Getwd", symbol: "os.Getwd", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Stat", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.ReadFile", symbol: "os.ReadFile", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.ReadDir", symbol: "os.ReadDir", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Remove", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.RemoveAll", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Rename", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.MkdirAll", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.MkdirTemp", symbol: "os.MkdirTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.CreateTemp", symbol: "os.CreateTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.WriteFile", symbol: "os.WriteFile", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.EvalSymlinks", symbol: "path/filepath.EvalSymlinks", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.WalkDir", symbol: "path/filepath.WalkDir", wireSymbol: repositoryImportPrefix + "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.MkdirTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.MkdirTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.CopyFS", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.DirFS", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "clearDirectoryContents", symbol: "os.ReadDir", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "clearDirectoryContents", symbol: "os.RemoveAll", wireSymbol: repositoryImportPrefix + "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.CreateTemp", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.AppendFile", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.AppendFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.ReadFile", symbol: "os.ReadFile", wireSymbol: repositoryImportPrefix + "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem.MkdirAll", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/locking.New"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem.OpenFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/locking.New"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/locking.New"},

	// Runtime metrics coordination is a policy-free host adapter selected by
	// Wire. Its stable OS-held locks are the external effect behind the public
	// RuntimeMetricsCoordination contract.
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "acquireRuntimeMetricsFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "openRuntimeMetricsLockFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "openRuntimeMetricsLockFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "package", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "prepareRuntimeMetricsCoordinationRoot", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_unix.go", operation: "tryLockRuntimeMetricsFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_unix.go", operation: "unlockRuntimeMetricsFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_windows.go", operation: "tryLockRuntimeMetricsFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_windows.go", operation: "unlockRuntimeMetricsFile", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/metrics.NewRuntimeMetricsCoordination"},

	// rollingfile is the shared policy-free writer used by the Wire-selected
	// runtime log, metrics, and ACP transcript openers. Keep these as exact
	// file/operation allowances so a new ambient effect elsewhere still fails.
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.currentTime", symbol: "time.Now", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.nextBackupPath", symbol: "os.Lstat", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openExistingOrNew", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openExistingOrNew", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.MkdirAll", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.Prepare", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.Prepare", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.readBackups", symbol: "os.ReadDir", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackClosedActiveFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackOpenActiveFile", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackOpenFile", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rotateForReservation", symbol: "os.Rename", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rotateForReservation", symbol: "os.Stat", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compress", symbol: "os.Open", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compress", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compressReader", symbol: "os.OpenFile", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compressReader", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "package", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "removeBackups", symbol: "os.Remove", wireSymbol: repositoryImportPrefix + "pkg/platform/logging.NewRuntimeLogOpener"},

	// These become allowed only after Wire explicitly selects the adapter. Until
	// then their ambient effects remain ordinary deletion-only findings.
	{filePath: "pkg/platform/process/command.go", operation: "ExecCommandRunner.Run", symbol: "os/exec.Command", wireSymbol: repositoryImportPrefix + "pkg/platform/process.ExecCommandRunner"},
	{filePath: "pkg/platform/process/executable.go", operation: "HostExecutableLocator.LookPath", symbol: "os/exec.LookPath", wireSymbol: repositoryImportPrefix + "pkg/platform/process.HostExecutableLocator"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "package", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "posixPTYAllocation.Master", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "posixPTYAllocation.Slave", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "POSIXHost.Start", symbol: "os/exec.Command", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "package", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "conPTYAllocation.InputPipe", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "conPTYAllocation.OutputPipe", symbol: "os.File", wireSymbol: repositoryImportPrefix + "pkg/platform/pty.NewHost"},
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func scanProductionDefaultSelections(repoRoot string) ([]productionDefaultFinding, error) {
	wireSelections, err := readWireProductionSelections(repoRoot)
	if err != nil {
		return nil, err
	}
	findingsByKey := map[string]productionDefaultFinding{}
	root := filepath.Join(repoRoot, "pkg")
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			relative, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			if relative == "pkg/root" || relative == "pkg/wire" ||
				strings.HasPrefix(relative, "pkg/root/") || strings.HasPrefix(relative, "pkg/wire/") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		filePath, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		filePath = filepath.ToSlash(filePath)
		fileSet := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fileSet, path, content, 0)
		if parseErr != nil {
			return parseErr
		}
		imports := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || spec.Name != nil && spec.Name.Name == "_" {
				continue
			}
			if spec.Name != nil && spec.Name.Name == "." {
				if isProductionDefaultControlledImport(importPath) {
					recordProductionDefaultFinding(
						findingsByKey, productionDefaultFinding{
							kind: "opaque-import", symbol: importPath, filePath: filePath,
							operation: "package", line: fileSet.Position(spec.Pos()).Line,
						},
					)
				}
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		for _, declaration := range parsed.Decls {
			operation := "package"
			if function, ok := declaration.(*ast.FuncDecl); ok {
				operation = productionOperationName(function)
			}
			ast.Inspect(declaration, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				qualified, _, imported := productionQualifiedSelector(selector, imports)
				if !imported {
					return true
				}
				kind, controlled := productionDefaultSymbols[qualified]
				if !controlled || productionDefaultAllowed(filePath, operation, qualified, wireSelections) {
					return true
				}
				recordProductionDefaultFinding(findingsByKey, productionDefaultFinding{
					kind: kind, symbol: qualified, filePath: filePath, operation: operation,
					line: fileSet.Position(selector.Pos()).Line,
				})
				return true
			})
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan production default selection: %w", err)
	}
	if err := scanPlatformAdapterSelections(repoRoot, findingsByKey); err != nil {
		return nil, err
	}
	findings := make([]productionDefaultFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, compareProductionDefaultFindings)
	return findings, nil
}

func scanPlatformAdapterSelections(
	repoRoot string,
	findings map[string]productionDefaultFinding,
) error {
	root := filepath.Join(repoRoot, "pkg")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if relative == "pkg/wire" || strings.HasPrefix(relative, "pkg/wire/") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, content, 0)
		if err != nil {
			return err
		}
		imports := make(map[string]string, len(parsed.Imports))
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		for _, declaration := range parsed.Decls {
			operation := "package"
			if function, ok := declaration.(*ast.FuncDecl); ok {
				operation = productionOperationName(function)
			}
			ast.Inspect(declaration, func(node ast.Node) bool {
				var selected ast.Expr
				switch expression := node.(type) {
				case *ast.CallExpr:
					selected = expression.Fun
				case *ast.CompositeLit:
					selected = expression.Type
				default:
					return true
				}
				qualified, position, imported := productionQualifiedSelector(selected, imports)
				if !imported {
					return true
				}
				if _, prohibited := platformAdapterSelectionSymbols[qualified]; !prohibited {
					return true
				}
				recordProductionDefaultFinding(findings, productionDefaultFinding{
					kind: "platform-adapter-selection", symbol: qualified,
					filePath: filePath, operation: operation, line: fileSet.Position(position).Line,
				})
				return true
			})
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("scan Platform adapter selections: %w", err)
	}
	return nil
}

func productionQualifiedSelector(
	node ast.Expr,
	imports map[string]string,
) (string, token.Pos, bool) {
	selector, ok := node.(*ast.SelectorExpr)
	if !ok {
		return "", token.NoPos, false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", token.NoPos, false
	}
	importPath, imported := imports[identifier.Name]
	if !imported {
		return "", token.NoPos, false
	}
	return importPath + "." + selector.Sel.Name, selector.Pos(), true
}

func recordProductionDefaultFinding(findings map[string]productionDefaultFinding, finding productionDefaultFinding) {
	key := productionDefaultKey(finding.filePath, finding.operation, finding.kind, finding.symbol)
	current := findings[key]
	if current.count == 0 {
		current = finding
	}
	current.count++
	findings[key] = current
}

func productionOperationName(function *ast.FuncDecl) string {
	if function == nil || function.Name == nil {
		return "function"
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := function.Recv.List[0].Type
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		receiver = pointer.X
	}
	if identifier, ok := receiver.(*ast.Ident); ok {
		return identifier.Name + "." + function.Name.Name
	}
	return function.Name.Name
}

func readWireProductionSelections(repoRoot string) (map[string]struct{}, error) {
	root := filepath.Join(repoRoot, "pkg", "wire")
	selections := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if shouldSkipRepositoryWalkDirectory(repoRoot, path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" ||
			strings.HasSuffix(entry.Name(), "_test.go") || entry.Name() == "wire_gen.go" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, payload, 0)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || spec.Name != nil && (spec.Name.Name == "_" || spec.Name.Name == ".") {
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			var selected ast.Expr
			switch expression := node.(type) {
			case *ast.CallExpr:
				selected = expression.Fun
			case *ast.CompositeLit:
				selected = expression.Type
			default:
				return true
			}
			selector, ok := selected.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if importPath, found := imports[identifier.Name]; found {
				selections[importPath+"."+selector.Sel.Name] = struct{}{}
			}
			return true
		})
		return nil
	})
	if os.IsNotExist(err) {
		return selections, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Wire production selections: %w", err)
	}
	return selections, nil
}

func productionDefaultAllowed(
	filePath string,
	operation string,
	symbol string,
	wireSelections map[string]struct{},
) bool {
	for _, allowance := range productionDefaultAllowances {
		if allowance.filePath == filePath && allowance.operation == operation && allowance.symbol == symbol &&
			allowance.wireSymbol != "" {
			_, selected := wireSelections[allowance.wireSymbol]
			return selected
		}
	}
	return false
}

func isProductionDefaultControlledImport(importPath string) bool {
	for symbol := range productionDefaultSymbols {
		if strings.HasPrefix(symbol, importPath+".") {
			return true
		}
	}
	return false
}

func compareProductionDefaultFindings(left, right productionDefaultFinding) int {
	return strings.Compare(
		productionDefaultKey(left.filePath, left.operation, left.kind, left.symbol),
		productionDefaultKey(right.filePath, right.operation, right.kind, right.symbol),
	)
}

func productionDefaultKey(filePath, operation, kind, symbol string) string {
	return filepath.ToSlash(filePath) + "\x00" + operation + "\x00" + kind + "\x00" + symbol
}

func loadProductionDefaultBaseline(repoRoot string) (productionDefaultBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, productionDefaultSelectionBaselinePath))
	if os.IsNotExist(err) {
		return productionDefaultBaseline{}, nil
	}
	if err != nil {
		return productionDefaultBaseline{}, fmt.Errorf("read production default selection baseline: %w", err)
	}
	var baseline productionDefaultBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return productionDefaultBaseline{}, fmt.Errorf("decode production default selection baseline: %w", err)
	}
	if baseline.Version != 1 {
		return productionDefaultBaseline{}, fmt.Errorf("production default selection baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(productionDefaultSelectionBaselinePath, len(baseline.Entries)); err != nil {
		return productionDefaultBaseline{}, err
	}
	return baseline, nil
}

func partitionProductionDefaultFindings(
	findings []productionDefaultFinding,
	baseline productionDefaultBaseline,
) ([]productionDefaultFinding, []productionDefaultBaselineEntry, error) {
	baselineByKey := make(map[string]productionDefaultBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateProductionDefaultBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := productionDefaultKey(entry.FilePath, entry.Operation, entry.Kind, entry.Symbol)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate production default selection baseline entry: %s %s %s %s", entry.FilePath, entry.Operation, entry.Kind, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []productionDefaultFinding
	seen := map[string]struct{}{}
	for _, finding := range findings {
		key := productionDefaultKey(finding.filePath, finding.operation, finding.kind, finding.symbol)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.count {
			blocking = append(blocking, finding)
		}
	}
	var stale []productionDefaultBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right productionDefaultBaselineEntry) int {
		return strings.Compare(
			productionDefaultKey(left.FilePath, left.Operation, left.Kind, left.Symbol),
			productionDefaultKey(right.FilePath, right.Operation, right.Kind, right.Symbol),
		)
	})
	return blocking, stale, nil
}

func validateProductionDefaultBaselineEntry(entry productionDefaultBaselineEntry) error {
	if entry.Kind == "" || entry.Symbol == "" || entry.FilePath == "" || entry.Operation == "" ||
		entry.Count < 1 || entry.Stage != productionDefaultSelectionBaselineStage ||
		entry.DeletionGate != productionDefaultSelectionDeletionGate {
		return fmt.Errorf("production default selection baseline entry is incomplete or has an unrecognized deletion gate: %#v", entry)
	}
	filePath := filepath.ToSlash(entry.FilePath)
	if !strings.HasPrefix(filePath, "pkg/") || strings.HasPrefix(filePath, "pkg/root/") ||
		strings.HasPrefix(filePath, "pkg/wire/") || strings.HasSuffix(filePath, "_test.go") {
		return fmt.Errorf("production default selection baseline entry is outside the controlled production graph: %s", entry.FilePath)
	}
	if entry.Kind == "opaque-import" {
		if !isProductionDefaultControlledImport(entry.Symbol) {
			return fmt.Errorf("production default selection baseline entry names an uncontrolled opaque import: %s", entry.Symbol)
		}
		return nil
	}
	if entry.Kind == "platform-adapter-selection" {
		if _, recognized := platformAdapterSelectionSymbols[entry.Symbol]; !recognized {
			return fmt.Errorf("production default selection baseline entry names an unknown Platform adapter: %s", entry.Symbol)
		}
		return nil
	}
	kind, recognized := productionDefaultSymbols[entry.Symbol]
	if !recognized || kind != entry.Kind {
		return fmt.Errorf("production default selection baseline entry names an unrecognized rule: %s %s", entry.Kind, entry.Symbol)
	}
	return nil
}

func createProductionDefaultBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	path := filepath.Join(repoRoot, productionDefaultSelectionBaselinePath)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create production default selection baseline: %w", err)
	}
	findings, err := scanProductionDefaultSelections(repoRoot)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if len(findings) == 0 {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("refuse to create empty %s; absence is the zero-debt state", productionDefaultSelectionBaselinePath)
	}
	baseline := productionDefaultBaseline{Version: 1}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, productionDefaultBaselineEntry{
			Kind: finding.kind, Symbol: finding.symbol, FilePath: finding.filePath,
			Operation: finding.operation, Count: finding.count,
			Stage: productionDefaultSelectionBaselineStage, DeletionGate: productionDefaultSelectionDeletionGate,
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode production default selection baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close production default selection baseline: %w", err)
	}
	fmt.Fprintf(stdoutWriter, "[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n", productionDefaultSelectionBaselinePath, len(baseline.Entries))
	return nil
}

func writeProductionDefaultFindings(writer io.Writer, findings []productionDefaultFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] hidden production %s default: %s in %s (%s:%d, %d occurrence(s))\n", finding.kind, finding.symbol, finding.operation, finding.filePath, finding.line, finding.count)
		fmt.Fprintln(writer, "  remediation: inject the exact effect contract and select its implementation in pkg/wire; only Wire-constructed policy-free leaf adapters may call the ambient implementation.")
	}
}

func writeStaleProductionDefaultBaselineEntries(writer io.Writer, entries []productionDefaultBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] stale production default selection baseline entry: %s %s %s %s\n", entry.FilePath, entry.Operation, entry.Kind, entry.Symbol)
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", productionDefaultSelectionBaselinePath)
	}
}

func writeProductionDefaultBaselineSummary(writer io.Writer, count int) {
	if count > 0 {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active production default-selection migration baseline: %d exact file/operation/kind/symbol edge(s)\n", count)
		fmt.Fprintln(writer, "  deletion gate: inject each effect and select its implementation in Wire, then delete its exact baseline entry.")
	}
}
