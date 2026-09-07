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
