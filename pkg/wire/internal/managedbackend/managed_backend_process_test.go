package managedbackend

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestBackendRuntimeFailureWrapsWithoutLeakingCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("token=sensitive backend path=C:\\private\\backend.exe")
	wrapped := WrapBackendExtractFailure(runtimeSubcauseArchiveOpen, cause)
	if !errors.Is(wrapped, cause) {
		t.Fatal("backend extract wrapper did not preserve the original cause")
	}
	var classified interface {
		ModelRuntimeStage() string
		ModelRuntimeFailureClass() string
	}
	if !errors.As(wrapped, &classified) {
		t.Fatal("backend extract wrapper did not expose the private classifier")
	}
	if classified.ModelRuntimeStage() != "BACKEND_EXTRACT" || classified.ModelRuntimeFailureClass() != "EXTRACTION_FAILED" {
		t.Fatalf("classification = (%q, %q), want BACKEND_EXTRACT/EXTRACTION_FAILED", classified.ModelRuntimeStage(), classified.ModelRuntimeFailureClass())
	}
	var subcause interface{ ModelRuntimeFailureSubcause() string }
	if !errors.As(wrapped, &subcause) || subcause.ModelRuntimeFailureSubcause() != runtimeSubcauseArchiveOpen {
		got := ""
		if subcause != nil {
			got = subcause.ModelRuntimeFailureSubcause()
		}
		t.Fatalf("subcause = %q, want %s", got, runtimeSubcauseArchiveOpen)
	}
	if strings.Contains(wrapped.Error(), "sensitive") || strings.Contains(wrapped.Error(), "C:\\private") {
		t.Fatalf("backend wrapper leaked cause: %q", wrapped.Error())
	}
}

func TestResolveManagedBackendLaunchExtractsPinnedWindowsPackage(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for name, body := range map[string]string{
		"llama-cpp-cpu-all.exe": "backend executable",
		"libwinpthread-1.dll":   "runtime dependency",
	} {
		entry, createErr := zipWriter.Create(name)
		if createErr != nil {
			t.Fatalf("create archive entry %q: %v", name, createErr)
		}
		if _, writeErr := entry.Write([]byte(body)); writeErr != nil {
			t.Fatalf("write archive entry %q: %v", name, writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	launch, err := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-llamacpp",
		ModelPath:    filepath.Join(t.TempDir(), "model.gguf"),
		BackendFiles: []string{archivePath},
	})
	if err != nil {
		t.Fatalf("ResolveManagedBackendLaunch: %v", err)
	}
	if !strings.EqualFold(filepath.Base(launch.Command), "llama-cpp-cpu-all.exe") {
		t.Fatalf("managed backend command = %q, want pinned executable", launch.Command)
	}
	if launch.WorkDir == "" || launch.Endpoint == "" || len(launch.Args) != 1 || !strings.HasPrefix(launch.Args[0], "--addr=") {
		t.Fatalf("managed backend launch = %#v, want extracted workdir, endpoint, and address arg", launch)
	}
	if _, err := os.Stat(launch.Command); err != nil {
		t.Fatalf("extracted executable = %q: %v", launch.Command, err)
	}
	workDir := launch.WorkDir
	launch.Cleanup()
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("managed backend workspace after cleanup = %v, want removed", err)
	}
}

func TestResolveManagedBackendLaunchRejectsUnsafeArchivePath(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	entry, err := zipWriter.Create("../llama-cpp-cpu-all.exe")
	if err != nil {
		t.Fatalf("create unsafe archive entry: %v", err)
	}
	if _, err := entry.Write([]byte("backend executable")); err != nil {
		t.Fatalf("write unsafe archive entry: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	_, launchErr := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-llamacpp",
		BackendFiles: []string{archivePath},
	})
	if launchErr == nil {
		t.Fatal("unsafe archive launch error = nil, want bounded extract failure")
	}
	var classified runtimeFailureClassifier
	if !errors.As(launchErr, &classified) {
		t.Fatalf("unsafe archive launch error = %v, want private runtime classification", launchErr)
	}
	if classified.ModelRuntimeStage() != runtimeStageBackendExtract ||
		classified.ModelRuntimeFailureClass() != runtimeFailureExtraction {
		t.Fatalf("classification = (%q, %q), want BACKEND_EXTRACT/EXTRACTION_FAILED", classified.ModelRuntimeStage(), classified.ModelRuntimeFailureClass())
	}
	if strings.Contains(launchErr.Error(), "unsafe") || strings.Contains(launchErr.Error(), archivePath) {
		t.Fatalf("bounded extract failure leaked raw cause: %q", launchErr.Error())
	}
}

func TestManagedBackendArchiveFaultsHaveOneBoundedSubcause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		subcause string
	}{
		{name: "selection", subcause: runtimeSubcauseArchiveSelection},
		{name: "open", subcause: runtimeSubcauseArchiveOpen},
		{name: "entry validation", subcause: runtimeSubcauseEntryValidation},
		{name: "entry copy", subcause: runtimeSubcauseEntryCopy},
		{name: "executable discovery", subcause: runtimeSubcauseExecutableDiscovery},
		{name: "endpoint reservation", subcause: runtimeSubcauseEndpointReservation},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			archivePath := writeManagedBackendArchive(t, true)
			parent := t.TempDir()
			var extractionRoot string
			cause := errors.New("token=archive-secret path=C:\\private\\backend.zip")
			operations := managedBackendOperations{
				fail: func(subcause string) error {
					if subcause == testCase.subcause {
						return cause
					}
					return nil
				},
				mkdirTemp: func(_, pattern string) (string, error) {
					root, err := os.MkdirTemp(parent, pattern)
					extractionRoot = root
					return root, err
				},
			}
			_, err := resolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
				Backend: "localai-vibevoice", BackendFiles: []string{archivePath},
			}, operations)
			assertManagedBackendFailure(t, err, testCase.subcause, cause)
			if extractionRoot != "" {
				if _, statErr := os.Stat(extractionRoot); !os.IsNotExist(statErr) {
					t.Fatalf("failed extraction root stat = %v, want removed", statErr)
				}
			}
		})
	}

	t.Run("cleanup", func(t *testing.T) {
		t.Parallel()
		archivePath := writeManagedBackendArchive(t, true)
		parent := t.TempDir()
		var extractionRoot string
		var removeCalls atomic.Int32
		cause := errors.New("token=cleanup-secret path=C:\\private\\extract")
		launch, err := resolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
			Backend: "localai-vibevoice", BackendFiles: []string{archivePath},
		}, managedBackendOperations{
			mkdirTemp: func(_, pattern string) (string, error) {
				root, mkdirErr := os.MkdirTemp(parent, pattern)
				extractionRoot = root
				return root, mkdirErr
			},
			removeAll: func(root string) error {
				removeCalls.Add(1)
				return cause
			},
		})
		if err != nil {
			t.Fatalf("resolveManagedBackendLaunch: %v", err)
		}
		first := launch.Cleanup()
		second := launch.Cleanup()
		assertManagedBackendFailure(t, first, runtimeSubcauseCleanup, cause)
		if second == nil || second.Error() != first.Error() {
			t.Fatalf("repeated cleanup error = %v, want retained bounded error", second)
		}
		if removeCalls.Load() != 1 {
			t.Fatalf("cleanup calls = %d, want once", removeCalls.Load())
		}
		if extractionRoot == "" {
			t.Fatal("cleanup test did not record an owned extraction root")
		}
	})
}

func TestManagedBackendCleanupIsRaceSafeAndOwned(t *testing.T) {
	t.Parallel()

	archivePath := writeManagedBackendArchive(t, true)
	parent := t.TempDir()
	peer := filepath.Join(parent, "peer-sentinel.txt")
	if err := os.WriteFile(peer, []byte("peer"), 0o600); err != nil {
		t.Fatalf("write peer sentinel: %v", err)
	}
	var extractionRoot string
	var removeCalls atomic.Int32
	launch, err := resolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend: "localai-vibevoice", BackendFiles: []string{archivePath},
	}, managedBackendOperations{
		mkdirTemp: func(_, pattern string) (string, error) {
			root, mkdirErr := os.MkdirTemp(parent, pattern)
			extractionRoot = root
			return root, mkdirErr
		},
		removeAll: func(root string) error {
			removeCalls.Add(1)
			return os.RemoveAll(root)
		},
	})
	if err != nil {
		t.Fatalf("resolveManagedBackendLaunch: %v", err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if cleanupErr := launch.Cleanup(); cleanupErr != nil {
				t.Errorf("concurrent cleanup: %v", cleanupErr)
			}
		}()
	}
	wait.Wait()
	if removeCalls.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want once", removeCalls.Load())
	}
	if _, err := os.Stat(extractionRoot); !os.IsNotExist(err) {
		t.Fatalf("owned extraction root stat = %v, want removed", err)
	}
	if body, err := os.ReadFile(peer); err != nil || string(body) != "peer" {
		t.Fatalf("peer sentinel = (%q, %v), want unchanged", body, err)
	}
}

func assertManagedBackendFailure(t *testing.T, err error, wantSubcause string, cause error) {
	t.Helper()
	if err == nil {
		t.Fatalf("managed backend error = nil, want %s", wantSubcause)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("managed backend error = %v, want original cause identity", err)
	}
	var classifier interface {
		ModelRuntimeStage() string
		ModelRuntimeFailureClass() string
	}
	if !errors.As(err, &classifier) || classifier.ModelRuntimeStage() != runtimeStageBackendExtract ||
		classifier.ModelRuntimeFailureClass() != runtimeFailureExtraction {
		t.Fatalf("classification = %v, want BACKEND_EXTRACT/EXTRACTION_FAILED", err)
	}
	var subcause interface{ ModelRuntimeFailureSubcause() string }
	if !errors.As(err, &subcause) || subcause.ModelRuntimeFailureSubcause() != wantSubcause {
		t.Fatalf("subcause = %v, want %s", err, wantSubcause)
	}
	if strings.Contains(err.Error(), "archive-secret") || strings.Contains(err.Error(), "cleanup-secret") ||
		strings.Contains(err.Error(), `C:\private`) {
		t.Fatalf("bounded failure leaked raw cause: %q", err.Error())
	}
}

func writeManagedBackendArchive(t *testing.T, includeExecutable bool) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	entries := map[string]string{
		"libgomp-1.dll":         "gomp",
		"libgovibevoicecpp.dll": "vibevoice",
		"libwinpthread-1.dll":   "pthread",
	}
	if includeExecutable {
		entries["vibevoice-cpp.exe"] = "backend"
	}
	for name, body := range entries {
		entry, createErr := zipWriter.Create(name)
		if createErr != nil {
			t.Fatalf("create archive entry %q: %v", name, createErr)
		}
		if _, writeErr := entry.Write([]byte(body)); writeErr != nil {
			t.Fatalf("write archive entry %q: %v", name, writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return archivePath
}
