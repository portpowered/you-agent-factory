package restart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This cell consumes the upstream compiled artifact and crosses real OS stderr
// and exit-status boundaries. The exhaustive cause matrix stays in lower layers.
func TestStartupLoadCauseWithoutDebug(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	home, repo := t.TempDir(), t.TempDir()
	path := filepath.Join(repo, "broken.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--resume", "./broken.json", "--quiet"}
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = repo
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("broken load exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("broken load polluted stdout: %q", stdout.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	var diagnostic factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &diagnostic); err != nil || diagnostic.Code == "" {
		t.Fatalf("broken load lost coded envelope: %v: %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unexpected end of JSON input") || strings.Count(stderr.String(), "cause[0]=") != 1 {
		t.Fatalf("broken load hid or repeated its cause: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "debug:") || strings.Contains(stderr.String(), home) || strings.Contains(stderr.String(), repo) {
		t.Fatalf("broken load leaked private startup details: %q", stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != "{" {
		t.Fatalf("broken load mutated the recording: bytes=%q error=%v", after, err)
	}
	t.Logf("INT-RESTART artifact SHA256=%s source=%s argv=%q exit=%d stdout=%q stderr=%q", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead, args, exit.ExitCode(), stdout.String(), stderr.String())
}
