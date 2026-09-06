package managedbackend

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

type ManagedBackendLaunch struct {
	Command  string
	Args     []string
	WorkDir  string
	Endpoint string
	Cleanup  func()
}

func ResolveManagedBackendLaunch(
	ctx context.Context,
	spec serviceedges.HostProcessStartSpec,
) (ManagedBackendLaunch, error) {
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
			Cleanup: func() {},
		}, nil
	}

	backend := strings.TrimSpace(spec.Backend)
	if backend == "" || len(spec.BackendFiles) == 0 {
		return ManagedBackendLaunch{}, fmt.Errorf(
			"supervised process command is required when no packaged backend is supplied",
		)
	}
	root, executable, cleanup, err := materializeManagedBackend(ctx, backend, spec.BackendFiles)
	if err != nil {
		return ManagedBackendLaunch{}, WrapBackendExtractFailure(err)
	}
	endpoint, address, err := managedBackendEndpoint(spec.HealthEndpoint)
	if err != nil {
		cleanup()
		return ManagedBackendLaunch{}, WrapBackendExtractFailure(err)
	}
	args := append([]string(nil), spec.Args...)
	args = append(args, "--addr="+address)
	return ManagedBackendLaunch{
		Command: executable, Args: args,
		WorkDir: root, Endpoint: endpoint, Cleanup: cleanup,
	}, nil
}

func materializeManagedBackend(
	ctx context.Context,
	backend string,
	files []string,
) (string, string, func(), error) {
	archivePath, directPath, err := selectManagedBackendFile(backend, files)
	if err != nil {
		return "", "", func() {}, err
	}
	if directPath != "" {
		return filepath.Dir(directPath), directPath, func() {}, nil
	}

	root, err := os.MkdirTemp("", "you-model-backend-")
	if err != nil {
		return "", "", func() {}, fmt.Errorf("prepare managed backend workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	if err := extractManagedBackendArchive(ctx, archivePath, root); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	executable, err := findManagedBackendExecutable(root, backend)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(executable, 0o755); err != nil {
			cleanup()
			return "", "", func() {}, fmt.Errorf("make managed backend executable: %w", err)
		}
	}
	return filepath.Dir(executable), executable, cleanup, nil
}

func selectManagedBackendFile(backend string, files []string) (string, string, error) {
	archivePath := ""
	for _, rawPath := range files {
		candidate := strings.TrimSpace(rawPath)
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil {
			return "", "", fmt.Errorf("inspect managed backend artifact: %w", err)
		}
		if info.IsDir() {
			return "", "", fmt.Errorf("managed backend artifact %q is a directory", candidate)
		}
		if isManagedBackendArchive(candidate) {
			if archivePath != "" {
				return "", "", fmt.Errorf("multiple packaged backend archives supplied for %q", backend)
			}
			archivePath = candidate
			continue
		}
		if directPath := managedBackendExecutableIfNamed(candidate, backend); directPath != "" {
			return "", directPath, nil
		}
	}
	if archivePath == "" {
		return "", "", fmt.Errorf("managed backend executable or archive is unavailable for %q", backend)
	}
	return archivePath, "", nil
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
	default:
		return ""
	}
}

func findManagedBackendExecutable(root, backend string) (string, error) {
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
		return "", fmt.Errorf("find managed backend executable: %w", err)
	}
	if found == "" {
		return "", fmt.Errorf("managed backend executable %q is missing from archive", wanted)
	}
	return found, nil
}

func managedBackendEndpoint(raw string) (string, string, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", "", fmt.Errorf("reserve managed backend endpoint: %w", err)
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			return "", "", fmt.Errorf("release managed backend endpoint: %w", err)
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
		return "", "", fmt.Errorf("managed backend endpoint is invalid")
	}
	return endpoint, address, nil
}

func extractManagedBackendArchive(ctx context.Context, archivePath, root string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractManagedBackendZip(ctx, archivePath, root)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractManagedBackendTarGz(ctx, archivePath, root)
	case strings.HasSuffix(lower, ".tar"):
		return extractManagedBackendTar(ctx, archivePath, root)
	default:
		return fmt.Errorf("managed backend archive format is unsupported: %q", archivePath)
	}
}

func extractManagedBackendZip(ctx context.Context, archivePath, root string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open managed backend archive: %w", err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if err := writeManagedBackendEntry(ctx, root, file.Name, file.FileInfo().IsDir(), file.Mode(), func() (io.ReadCloser, error) {
			return file.Open()
		}); err != nil {
			return err
		}
	}
	return nil
}

func extractManagedBackendTarGz(ctx context.Context, archivePath, root string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open managed backend archive: %w", err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open managed backend gzip: %w", err)
	}
	defer reader.Close()
	return extractManagedBackendTarReader(ctx, tar.NewReader(reader), root)
}

func extractManagedBackendTar(ctx context.Context, archivePath, root string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open managed backend archive: %w", err)
	}
	defer file.Close()
	return extractManagedBackendTarReader(ctx, tar.NewReader(file), root)
}

func extractManagedBackendTarReader(ctx context.Context, reader *tar.Reader, root string) error {
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read managed backend archive: %w", err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := writeManagedBackendEntry(ctx, root, header.Name, true, header.FileInfo().Mode(), nil); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeManagedBackendEntry(ctx, root, header.Name, false, header.FileInfo().Mode(), func() (io.ReadCloser, error) {
				return io.NopCloser(reader), nil
			}); err != nil {
				return err
			}
		case tar.TypeXHeader, tar.TypeXGlobalHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		default:
			return fmt.Errorf("managed backend archive contains unsupported entry %q", header.Name)
		}
	}
}

func writeManagedBackendEntry(
	ctx context.Context,
	root, name string,
	directory bool,
	mode os.FileMode,
	open func() (io.ReadCloser, error),
) error {
	relative, err := safeManagedArchivePath(name)
	if err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if directory {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create managed backend directory: %w", err)
		}
		return nil
	}
	if open == nil {
		return fmt.Errorf("managed backend file %q has no content", name)
	}
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("managed backend archive contains symlink %q", name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create managed backend file directory: %w", err)
	}
	input, err := open()
	if err != nil {
		return fmt.Errorf("open managed backend file %q: %w", name, err)
	}
	defer input.Close()
	fileMode := mode.Perm()
	if fileMode == 0 {
		fileMode = 0o644
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("create managed backend file %q: %w", name, err)
	}
	if err := copyManagedBackendFile(ctx, output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("extract managed backend file %q: %w", name, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close managed backend file %q: %w", name, err)
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
