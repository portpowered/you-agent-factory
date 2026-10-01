package managedbackend

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestResolveManagedBackendLaunchBindsPinnedWindowsVibeVoiceLibrary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the pinned VibeVoice library correction is Windows-specific")
	}
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "backend.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for name, body := range map[string]string{
		"vibevoice-cpp.exe":     "backend executable",
		"libgovibevoicecpp.dll": "pinned VibeVoice library",
		"libgomp-1.dll":         "runtime dependency",
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
		t.Fatalf("close archive: %v", err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatalf("close archive file: %v", err)
	}

	launch, err := ResolveManagedBackendLaunch(context.Background(), serviceedges.HostProcessStartSpec{
		Backend:      "localai-vibevoice",
		BackendFiles: []string{archivePath},
	})
	if err != nil {
		t.Fatalf("ResolveManagedBackendLaunch: %v", err)
	}
	wantLibrary := filepath.Join(launch.WorkDir, "libgovibevoicecpp.dll")
	if _, err := os.Stat(wantLibrary); err != nil {
		t.Fatalf("pinned VibeVoice library = %q: %v", wantLibrary, err)
	}
	wantEnvironment := "VIBEVOICECPP_LIBRARY=" + wantLibrary
	if len(launch.Env) != 1 || !strings.EqualFold(launch.Env[0], wantEnvironment) {
		t.Fatalf("managed backend environment = %#v, want one private DLL binding", launch.Env)
	}
	if strings.Contains(strings.Join(launch.Args, " "), "libgovibevoicecpp-fallback.so") {
		t.Fatalf("managed backend args selected the Unix fallback library: %#v", launch.Args)
	}
	launch.Cleanup()
}

func TestResolveManagedBackendLaunchBindsWindowsQwenReferenceBackendLibrary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the Qwen DLL binding is Windows-specific")
	}
	t.Parallel()
	archivePath := filepath.Join(t.TempDir(), "qwen-backend.zip")
	writeQwenWindowsArchive(t, archivePath)
	launch, err := ResolveManagedBackendLaunch(t.Context(), serviceedges.HostProcessStartSpec{
		Backend: "localai-qwen3-tts-cpp", BackendFiles: []string{archivePath},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := launch.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	if filepath.Base(launch.Command) != "qwen3-tts-cpp.exe" || filepath.Dir(launch.Command) != launch.WorkDir {
		t.Fatalf("launch command/workdir = %q/%q, want extracted Qwen wrapper", launch.Command, launch.WorkDir)
	}
	library := filepath.Join(launch.WorkDir, "libgoqwen3ttscpp.dll")
	if _, err := os.Stat(library); err != nil {
		t.Fatal(err)
	}
	if len(launch.Env) != 1 || launch.Env[0] != "QWEN3TTS_LIBRARY="+library {
		t.Fatalf("launch environment = %v, want packaged Qwen DLL", launch.Env)
	}
	if len(launch.Args) != 1 || !strings.HasPrefix(launch.Args[0], "--addr=") {
		t.Fatalf("launch arguments = %v, want controlled local gRPC endpoint", launch.Args)
	}
}

func writeQwenWindowsArchive(t *testing.T, archivePath string) {
	t.Helper()
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archiveFile)
	for _, name := range []string{"qwen3-tts-cpp.exe", "libgoqwen3ttscpp.dll", "ggml-cuda.dll"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("controlled backend file")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}
}
