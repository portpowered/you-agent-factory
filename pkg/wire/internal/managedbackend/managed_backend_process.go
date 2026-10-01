package managedbackend

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

type ManagedBackendLaunch struct {
	Command  string
	Args     []string
	Env      []string
	WorkDir  string
	Endpoint string
	Cleanup  func() error
}

// managedBackendOperations contains only the private native effects whose
// failures need focused classification. Production uses the standard
// implementations; component tests replace one operation at a time without
// adding a production-only test API.
type managedBackendOperations struct {
	fail      func(string) error
	mkdirTemp func(string, string) (string, error)
	removeAll func(string) error
	listen    func(string, string) (net.Listener, error)
}

func (operations managedBackendOperations) withDefaults() managedBackendOperations {
	if operations.mkdirTemp == nil {
		operations.mkdirTemp = os.MkdirTemp
	}
	if operations.removeAll == nil {
		operations.removeAll = os.RemoveAll
	}
	if operations.listen == nil {
		operations.listen = net.Listen
	}
	return operations
}

func (operations managedBackendOperations) failAt(subcause string) error {
	if operations.fail == nil {
		return nil
	}
	if err := operations.fail(subcause); err != nil {
		return WrapBackendExtractFailure(subcause, err)
	}
	return nil
}

func ResolveManagedBackendLaunch(
	ctx context.Context,
	spec serviceedges.HostProcessStartSpec,
) (ManagedBackendLaunch, error) {
	return resolveManagedBackendLaunch(ctx, spec, managedBackendOperations{})
}

func resolveManagedBackendLaunch(
	ctx context.Context,
	spec serviceedges.HostProcessStartSpec,
	operations managedBackendOperations,
) (ManagedBackendLaunch, error) {
	operations = operations.withDefaults()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ManagedBackendLaunch{}, err
	}
	command := strings.TrimSpace(spec.Command)
	if command != "" {
		endpoint := strings.TrimSpace(spec.HealthEndpoint)
		if endpoint == "" {
			return ManagedBackendLaunch{}, fmt.Errorf("supervised process health endpoint is required")
		}
		return ManagedBackendLaunch{
			Command: command, Args: append([]string(nil), spec.Args...),
			WorkDir: spec.WorkDir, Endpoint: endpoint,
			Cleanup: func() error { return nil },
		}, nil
	}

	backend := strings.TrimSpace(spec.Backend)
	if backend == "" || len(spec.BackendFiles) == 0 {
		return ManagedBackendLaunch{}, fmt.Errorf(
			"supervised process command is required when no packaged backend is supplied",
		)
	}
	root, executable, installed, cleanup, err := materializeManagedBackend(ctx, backend, spec.BackendFiles, operations)
	if err != nil {
		return ManagedBackendLaunch{}, err
	}
	endpoint, address, err := managedBackendEndpoint(spec.HealthEndpoint, operations)
	if err != nil {
		return ManagedBackendLaunch{}, cleanupManagedBackendFailure(err, cleanup)
	}
	args := append([]string(nil), spec.Args...)
	args = append(args, "--addr="+address)
	environment := managedBackendEnvironment(backend, root)
	if installed && backend == "localai-llamacpp" {
		grpcExecutable := filepath.Join(root, "llama-cpp-grpc")
		if executable != grpcExecutable {
			args = append([]string{grpcExecutable}, args...)
		}
		libraryPath := filepath.Join(root, "lib")
		previous := managedBackendEnvironmentValue(spec.Env, "LD_LIBRARY_PATH")
		if previous != "" {
			libraryPath += ":" + previous
		}
		environment = append(environment, "LD_LIBRARY_PATH="+libraryPath)
	}
	return ManagedBackendLaunch{
		Command: executable, Args: args,
		Env:     environment,
		WorkDir: root, Endpoint: endpoint, Cleanup: cleanup,
	}, nil
}

func managedBackendEnvironment(backend, root string) []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	// Windows backend wrappers need their packaged DLL rather than the Unix
	// fallback selected when the library environment variable is absent.
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "localai-vibevoice":
		return []string{"VIBEVOICECPP_LIBRARY=" + filepath.Join(root, "libgovibevoicecpp.dll")}
	case "localai-qwen3-tts-cpp":
		return []string{"QWEN3TTS_LIBRARY=" + filepath.Join(root, "libgoqwen3ttscpp.dll")}
	default:
		return nil
	}
}

func materializeManagedBackend(
	ctx context.Context,
	backend string,
	files []string,
	operations managedBackendOperations,
) (string, string, bool, func() error, error) {
	if err := operations.failAt(runtimeSubcauseArchiveSelection); err != nil {
		return "", "", false, func() error { return nil }, err
	}
	archivePath, directPath, installedDirectory, err := selectManagedBackendFile(backend, files)
	if err != nil {
		return "", "", false, func() error { return nil }, WrapBackendExtractFailure(runtimeSubcauseArchiveSelection, err)
	}
	if installedDirectory != "" {
		var script string
		var scriptErr error
		if backend == "localai-llamacpp" {
			script, scriptErr = installedLlamaCPPEntrypoint(installedDirectory)
		} else {
			script, scriptErr = installedBackendEntrypoint(installedDirectory)
		}
		if scriptErr != nil {
			return "", "", false, func() error { return nil }, WrapBackendExtractFailure(runtimeSubcauseExecutableDiscovery, scriptErr)
		}
		return installedDirectory, script, true, func() error { return nil }, nil
	}
	if directPath != "" {
		return filepath.Dir(directPath), directPath, false, func() error { return nil }, nil
	}

	root, err := operations.mkdirTemp("", "you-model-backend-")
	if err != nil {
		return "", "", false, func() error { return nil }, WrapBackendExtractFailure(
			runtimeSubcauseArchiveSelection,
			fmt.Errorf("prepare managed backend workspace: %w", err),
		)
	}
	cleanup := newManagedBackendCleanup(root, operations.removeAll)
	if err := extractManagedBackendArchive(ctx, archivePath, root, operations); err != nil {
		return "", "", false, cleanup, cleanupManagedBackendFailure(err, cleanup)
	}
	executable, err := findManagedBackendExecutable(root, backend, operations)
	if err != nil {
		return "", "", false, cleanup, cleanupManagedBackendFailure(
			WrapBackendExtractFailure(runtimeSubcauseExecutableDiscovery, err), cleanup,
		)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(executable, 0o755); err != nil {
			return "", "", false, cleanup, cleanupManagedBackendFailure(
				WrapBackendExtractFailure(runtimeSubcauseEntryCopy, fmt.Errorf("make managed backend executable: %w", err)),
				cleanup,
			)
		}
	}
	return filepath.Dir(executable), executable, false, cleanup, nil
}

func installedBackendEntrypoint(directory string) (string, error) {
	return installedExecutable(directory, "run.sh")
}

func installedLlamaCPPEntrypoint(directory string) (string, error) {
	executable, err := installedExecutable(directory, "llama-cpp-grpc")
	if err != nil {
		return "", err
	}
	libraryDirectory := filepath.Join(directory, "lib")
	if info, err := os.Lstat(libraryDirectory); err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("installed backend library directory must be a directory")
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect installed backend library directory: %w", err)
	}
	loader := filepath.Join(directory, "lib", "ld.so")
	if _, err := os.Lstat(loader); os.IsNotExist(err) {
		return executable, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect installed backend loader: %w", err)
	}
	if _, err := installedExecutable(directory, filepath.Join("lib", "ld.so")); err != nil {
		return "", fmt.Errorf("invalid installed backend loader: %w", err)
	}
	return loader, nil
}

func installedExecutable(directory, name string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("installed shell backend cannot run on Windows")
	}
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("installed backend directory must be absolute")
	}
	script := filepath.Join(directory, name)
	info, err := os.Lstat(script)
	if err != nil {
		return "", fmt.Errorf("inspect installed backend entrypoint: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("installed backend entrypoint must be a regular executable file")
	}
	return script, nil
}

func managedBackendEnvironmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	if len(environment) == 0 {
		return os.Getenv(key)
	}
	return ""
}

func newManagedBackendCleanup(root string, removeAll func(string) error) func() error {
	cleanup := &managedBackendCleanup{root: root, removeAll: removeAll}
	return cleanup.run
}

type managedBackendCleanup struct {
	once      sync.Once
	root      string
	removeAll func(string) error
	err       error
}

func (cleanup *managedBackendCleanup) run() error {
	if cleanup == nil {
		return nil
	}
	cleanup.once.Do(func() {
		if cleanup.removeAll == nil {
			cleanup.removeAll = os.RemoveAll
		}
		if err := cleanup.removeAll(cleanup.root); err != nil {
			cleanup.err = WrapBackendExtractFailure(runtimeSubcauseCleanup, err)
		}
	})
	return cleanup.err
}

func cleanupManagedBackendFailure(primary error, cleanup func() error) error {
	if cleanup == nil {
		return primary
	}
	cleanupErr := cleanup()
	if cleanupErr == nil {
		return primary
	}
	if primary == nil {
		return cleanupErr
	}
	return newBackendExtractFailure(runtimeSubcauseCleanup, errors.Join(primary, cleanupErr))
}

func selectManagedBackendFile(backend string, files []string) (string, string, string, error) {
	archivePath := ""
	installedDirectory := ""
	for _, rawPath := range files {
		candidate := strings.TrimSpace(rawPath)
		if candidate == "" {
			continue
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return "", "", "", fmt.Errorf("inspect managed backend artifact: %w", err)
		}
		if info.IsDir() {
			if len(files) != 1 {
				return "", "", "", fmt.Errorf("installed backend directory must be the only backend file")
			}
			installedDirectory = candidate
			continue
		}
		if !info.Mode().IsRegular() {
			return "", "", "", fmt.Errorf("managed backend artifact must be a regular file or directory")
		}
		if isManagedBackendArchive(candidate) {
			if archivePath != "" {
				return "", "", "", fmt.Errorf("multiple packaged backend archives supplied for %q", backend)
			}
			archivePath = candidate
			continue
		}
		if directPath := managedBackendExecutableIfNamed(candidate, backend); directPath != "" {
			return "", directPath, "", nil
		}
	}
	if installedDirectory != "" {
		return "", "", installedDirectory, nil
	}
	if archivePath == "" {
		return "", "", "", fmt.Errorf("managed backend executable or archive is unavailable for %q", backend)
	}
	return archivePath, "", "", nil
}

func isManagedBackendArchive(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar")
}

func managedBackendExecutableIfNamed(name, backend string) string {
	base := filepath.Base(name)
	executable := managedBackendExecutableName(backend)
	if strings.EqualFold(base, executable) || strings.EqualFold(base, executable+".exe") {
		return name
	}
	return ""
}

func managedBackendExecutableName(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "localai-llamacpp":
		return "llama-cpp-cpu-all"
	case "localai-whisper":
		return "whisper"
	case "localai-vibevoice":
		return "vibevoice-cpp"
	case "localai-qwen3-tts-cpp":
		return "qwen3-tts-cpp"
	default:
		return ""
	}
}

func findManagedBackendExecutable(root, backend string, operations managedBackendOperations) (string, error) {
	if err := operations.failAt(runtimeSubcauseExecutableDiscovery); err != nil {
		return "", err
	}
	wanted := managedBackendExecutableName(backend)
	if wanted == "" {
		return "", fmt.Errorf("managed backend executable is unknown for %q", backend)
	}
	var found string
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.EqualFold(name, wanted) || strings.EqualFold(name, wanted+".exe") {
			found = current
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", WrapBackendExtractFailure(runtimeSubcauseExecutableDiscovery,
			fmt.Errorf("find managed backend executable: %w", err))
	}
	if found == "" {
		return "", WrapBackendExtractFailure(runtimeSubcauseExecutableDiscovery,
			fmt.Errorf("managed backend executable %q is missing from archive", wanted))
	}
	return found, nil
}

func managedBackendEndpoint(raw string, operations managedBackendOperations) (string, string, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		if err := operations.failAt(runtimeSubcauseEndpointReservation); err != nil {
			return "", "", err
		}
		listener, err := operations.listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", "", WrapBackendExtractFailure(runtimeSubcauseEndpointReservation,
				fmt.Errorf("reserve managed backend endpoint: %w", err))
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			return "", "", WrapBackendExtractFailure(runtimeSubcauseEndpointReservation,
				fmt.Errorf("release managed backend endpoint: %w", err))
		}
		return "grpc://" + address, address, nil
	}
	address := endpoint
	for _, prefix := range []string{"grpc://", "tcp://"} {
		if strings.HasPrefix(strings.ToLower(address), prefix) {
			address = strings.TrimSpace(address[len(prefix):])
			break
		}
	}
	if address == "" {
		return "", "", WrapBackendExtractFailure(runtimeSubcauseEndpointReservation,
			fmt.Errorf("managed backend endpoint is invalid"))
	}
	return endpoint, address, nil
}

func extractManagedBackendArchive(
	ctx context.Context,
	archivePath, root string,
	operations managedBackendOperations,
) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractManagedBackendZip(ctx, archivePath, root, operations)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractManagedBackendTarGz(ctx, archivePath, root, operations)
	case strings.HasSuffix(lower, ".tar"):
		return extractManagedBackendTar(ctx, archivePath, root, operations)
	default:
		return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
			fmt.Errorf("managed backend archive format is unsupported: %q", archivePath))
	}
}

func extractManagedBackendZip(
	ctx context.Context,
	archivePath, root string,
	operations managedBackendOperations,
) error {
	if err := operations.failAt(runtimeSubcauseArchiveOpen); err != nil {
		return err
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
			fmt.Errorf("open managed backend archive: %w", err))
	}
	defer archive.Close()
	for _, file := range archive.File {
		if err := writeManagedBackendEntry(ctx, root, file.Name, file.FileInfo().IsDir(), file.Mode(), func() (io.ReadCloser, error) {
			return file.Open()
		}, operations); err != nil {
			return err
		}
	}
	return nil
}

func extractManagedBackendTarGz(
	ctx context.Context,
	archivePath, root string,
	operations managedBackendOperations,
) error {
	if err := operations.failAt(runtimeSubcauseArchiveOpen); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
			fmt.Errorf("open managed backend archive: %w", err))
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
			fmt.Errorf("open managed backend gzip: %w", err))
	}
	defer reader.Close()
	return extractManagedBackendTarReader(ctx, tar.NewReader(reader), root, operations)
}

func extractManagedBackendTar(
	ctx context.Context,
	archivePath, root string,
	operations managedBackendOperations,
) error {
	if err := operations.failAt(runtimeSubcauseArchiveOpen); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
			fmt.Errorf("open managed backend archive: %w", err))
	}
	defer file.Close()
	return extractManagedBackendTarReader(ctx, tar.NewReader(file), root, operations)
}

func extractManagedBackendTarReader(
	ctx context.Context,
	reader *tar.Reader,
	root string,
	operations managedBackendOperations,
) error {
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return WrapBackendExtractFailure(runtimeSubcauseArchiveOpen,
				fmt.Errorf("read managed backend archive: %w", err))
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := writeManagedBackendEntry(ctx, root, header.Name, true, header.FileInfo().Mode(), nil, operations); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeManagedBackendEntry(ctx, root, header.Name, false, header.FileInfo().Mode(), func() (io.ReadCloser, error) {
				return io.NopCloser(reader), nil
			}, operations); err != nil {
				return err
			}
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		default:
			return WrapBackendExtractFailure(runtimeSubcauseEntryValidation,
				fmt.Errorf("managed backend archive contains unsupported entry %q", header.Name))
		}
	}
}

func writeManagedBackendEntry(
	ctx context.Context,
	root, name string,
	directory bool,
	mode os.FileMode,
	open func() (io.ReadCloser, error),
	operations managedBackendOperations,
) error {
	if err := operations.failAt(runtimeSubcauseEntryValidation); err != nil {
		return err
	}
	relative, err := safeManagedArchivePath(name)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryValidation, err)
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if directory {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
				fmt.Errorf("create managed backend directory: %w", err))
		}
		return nil
	}
	if open == nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryValidation,
			fmt.Errorf("managed backend file %q has no content", name))
	}
	if mode&os.ModeSymlink != 0 {
		return WrapBackendExtractFailure(runtimeSubcauseEntryValidation,
			fmt.Errorf("managed backend archive contains symlink %q", name))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
			fmt.Errorf("create managed backend file directory: %w", err))
	}
	input, err := open()
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
			fmt.Errorf("open managed backend file %q: %w", name, err))
	}
	defer input.Close()
	fileMode := mode.Perm()
	if fileMode == 0 {
		fileMode = 0o644
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
			fmt.Errorf("create managed backend file %q: %w", name, err))
	}
	if err := operations.failAt(runtimeSubcauseEntryCopy); err != nil {
		_ = output.Close()
		return err
	}
	if err := copyManagedBackendFile(ctx, output, input); err != nil {
		_ = output.Close()
		return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
			fmt.Errorf("extract managed backend file %q: %w", name, err))
	}
	if err := output.Close(); err != nil {
		return WrapBackendExtractFailure(runtimeSubcauseEntryCopy,
			fmt.Errorf("close managed backend file %q: %w", name, err))
	}
	return nil
}

func copyManagedBackendFile(ctx context.Context, output io.Writer, input io.Reader) error {
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := input.Read(buffer)
		if read > 0 {
			written := 0
			for written < read {
				count, writeErr := output.Write(buffer[written:read])
				written += count
				if writeErr != nil {
					return writeErr
				}
				if count == 0 {
					return io.ErrShortWrite
				}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func safeManagedArchivePath(name string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	clean := path.Clean(normalized)
	if normalized == "" || clean == "." || clean == ".." ||
		strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") ||
		(len(clean) >= 2 && clean[1] == ':') {
		return "", fmt.Errorf("managed backend archive path is unsafe: %q", name)
	}
	return clean, nil
}
