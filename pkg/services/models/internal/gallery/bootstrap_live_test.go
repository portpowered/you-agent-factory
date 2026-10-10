package gallery

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalAIBinaryBootstrapLive(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("YOU_LOCALAI_LIVE_BOOTSTRAP") != "1" {
		t.Skip("opt in on Linux with YOU_LOCALAI_LIVE_BOOTSTRAP=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	path, err := downloadLocalAIBinary(ctx, http.DefaultClient, t.TempDir(), localAIReleaseURL)
	if err != nil {
		t.Fatalf("download verified LocalAI binary: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("downloaded LocalAI binary = %q, info = %#v, error = %v", path, info, err)
	}
	version, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(version)), "v") {
		t.Fatalf("downloaded LocalAI version check: output = %q, error = %v", version, err)
	}
}
