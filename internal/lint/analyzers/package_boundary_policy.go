package analyzers

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
	"pkg/platform/process.NewParentOwnedStdio": {},
	"pkg/platform/directoryreplace.Local":      {},
	"pkg/platform/directoryreplace.NewLocal":   {},
	"pkg/platform/filesystem.Local":            {},
	"pkg/platform/inboxgitkeep.NewLocal":       {},
	"pkg/platform/locking.LocalFileSystem":     {},
	"pkg/platform/locking.New":                 {},
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
	{filePath: "pkg/platform/clock/clock.go", operation: "Real.Now", symbol: "time.Now", wireSymbol: "pkg/platform/clock.Real"},
	{filePath: "pkg/platform/browser/open.go", operation: "Host.Open", symbol: "os/exec.CommandContext", wireSymbol: "pkg/platform/browser.NewHost"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.MkdirTemp", symbol: "os.MkdirTemp", wireSymbol: "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.WriteFile", symbol: "os.WriteFile", wireSymbol: "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.Stat", symbol: "os.Stat", wireSymbol: "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "FileSystem.RemoveAll", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/contentstaging.FileSystem"},
	{filePath: "pkg/platform/contentstaging/adapters.go", operation: "Random.Read", symbol: "crypto/rand.Read", wireSymbol: "pkg/platform/contentstaging.Random"},
	{filePath: "pkg/platform/random/crypto.go", operation: "CryptoSource.Int63n", symbol: "crypto/rand.Reader", wireSymbol: "pkg/platform/random.CryptoSource"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Open", symbol: "os.Open", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Create", symbol: "os.Create", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.OpenFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Getwd", symbol: "os.Getwd", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Stat", symbol: "os.Stat", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.ReadFile", symbol: "os.ReadFile", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.ReadDir", symbol: "os.ReadDir", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Remove", symbol: "os.Remove", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.RemoveAll", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.Rename", symbol: "os.Rename", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.MkdirAll", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.MkdirTemp", symbol: "os.MkdirTemp", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.CreateTemp", symbol: "os.CreateTemp", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.WriteFile", symbol: "os.WriteFile", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.EvalSymlinks", symbol: "path/filepath.EvalSymlinks", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/filesystem/local.go", operation: "Local.WalkDir", symbol: "path/filepath.WalkDir", wireSymbol: "pkg/platform/filesystem.Local"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.MkdirTemp", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.Remove", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.Rename", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Commit", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Stat", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.MkdirTemp", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Remove", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.Rename", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "Local.Restore", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.CopyFS", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.DirFS", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "replaceWatchedDirectoryContents", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "clearDirectoryContents", symbol: "os.ReadDir", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/directoryreplace/replace.go", operation: "clearDirectoryContents", symbol: "os.RemoveAll", wireSymbol: "pkg/platform/directoryreplace.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.CreateTemp", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.Remove", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.WriteFile", symbol: "os.Rename", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.AppendFile", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.AppendFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/replay/storage.go", operation: "Local.ReadFile", symbol: "os.ReadFile", wireSymbol: "pkg/platform/replay.NewLocal"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem.MkdirAll", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/locking.New"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem.OpenFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/locking.New"},
	{filePath: "pkg/platform/locking/file.go", operation: "LocalFileSystem", symbol: "os.File", wireSymbol: "pkg/platform/locking.New"},

	// Runtime metrics coordination is a policy-free host adapter selected by
	// Wire. Its stable OS-held locks are the external effect behind the public
	// RuntimeMetricsCoordination contract.
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "acquireRuntimeMetricsFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "openRuntimeMetricsLockFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "openRuntimeMetricsLockFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "package", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination.go", operation: "prepareRuntimeMetricsCoordinationRoot", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_unix.go", operation: "tryLockRuntimeMetricsFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_unix.go", operation: "unlockRuntimeMetricsFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_windows.go", operation: "tryLockRuntimeMetricsFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},
	{filePath: "pkg/platform/metrics/runtime_metrics_coordination_windows.go", operation: "unlockRuntimeMetricsFile", symbol: "os.File", wireSymbol: "pkg/platform/metrics.NewRuntimeMetricsCoordination"},

	// rollingfile is the shared policy-free writer used by the Wire-selected
	// runtime log, metrics, and ACP transcript openers. Keep these as exact
	// file/operation allowances so a new ambient effect elsewhere still fails.
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.currentTime", symbol: "time.Now", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openExistingOrNew", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openExistingOrNew", symbol: "os.Stat", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.MkdirAll", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.Rename", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openNew", symbol: "os.Stat", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.openFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.Prepare", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.Prepare", symbol: "os.Stat", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.readBackups", symbol: "os.ReadDir", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackClosedActiveFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackOpenActiveFile", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackOpenFile", symbol: "os.Remove", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.Remove", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rollbackRotation", symbol: "os.Rename", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rotateForReservation", symbol: "os.Rename", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "Writer.rotateForReservation", symbol: "os.Stat", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compress", symbol: "os.Open", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compress", symbol: "os.Remove", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compressReader", symbol: "os.OpenFile", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "compressReader", symbol: "os.Remove", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "package", symbol: "os.File", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},
	{filePath: "pkg/platform/rollingfile/rollingfile.go", operation: "removeBackups", symbol: "os.Remove", wireSymbol: "pkg/platform/logging.NewRuntimeLogOpener"},

	// These become allowed only after Wire explicitly selects the adapter. Until
	// then their ambient effects remain ordinary deletion-only findings.
	{filePath: "pkg/platform/process/supervised_subprocess.go", operation: "NewParentOwnedStdio", symbol: "os.File", wireSymbol: "pkg/platform/process.NewParentOwnedStdio"},
	{filePath: "pkg/platform/process/supervised_subprocess.go", operation: "openParentOwnedStdio", symbol: "os.File", wireSymbol: "pkg/platform/process.NewParentOwnedStdio"},
	{filePath: "pkg/platform/process/supervised_subprocess.go", operation: "package", symbol: "os.File", wireSymbol: "pkg/platform/process.NewParentOwnedStdio"},
	{filePath: "pkg/platform/process/command.go", operation: "ExecCommandRunner.Run", symbol: "os/exec.Command", wireSymbol: "pkg/platform/process.ExecCommandRunner"},
	{filePath: "pkg/platform/process/managedchild/managed.go", operation: "Start", symbol: "os/exec.Command", wireSymbol: "pkg/platform/process/managedchild.Start"},
	{filePath: "pkg/platform/process/executable.go", operation: "HostExecutableLocator.LookPath", symbol: "os/exec.LookPath", wireSymbol: "pkg/platform/process.HostExecutableLocator"},
	{filePath: "pkg/platform/process/executable.go", operation: "HostExecutableLocator.CurrentExecutable", symbol: "os.Executable", wireSymbol: "pkg/platform/process.HostExecutableLocator"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "package", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "posixPTYAllocation.Master", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "posixPTYAllocation.Slave", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_unix.go", operation: "POSIXHost.Start", symbol: "os/exec.Command", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "package", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "conPTYAllocation.InputPipe", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
	{filePath: "pkg/platform/pty/platform_windows.go", operation: "conPTYAllocation.OutputPipe", symbol: "os.File", wireSymbol: "pkg/platform/pty.NewHost"},
}
