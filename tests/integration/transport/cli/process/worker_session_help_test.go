package process_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

// The integration lane supplies the artifact; this test only observes OS exits
// and streams, leaving the behavior matrix to the public functional boundary.
func TestBuiltCLIWorkerSessionHelpAndUsage(t *testing.T) {
	t.Parallel()
	binary := quietShutdownArtifact(t)
	for _, cell := range []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"--server", "http://127.0.0.1:1", "worker-sessions", "--help"}, 0},
		{"usage", []string{"worker-sessions", "missing", "--server", "http://127.0.0.1:1"}, 1},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, cell.args...)
			cmd.Env = builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
			cmd.Dir = t.TempDir()
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != cell.code {
				t.Fatalf("exit=%d want=%d stderr=%q", code, cell.code, stderr.String())
			}
			if cell.code == 0 {
				if strings.Count(stdout.String(), "Usage:") != 1 || stderr.Len() != 0 {
					t.Fatalf("help streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			} else if stdout.Len() != 0 || !strings.Contains(stderr.String(), `unknown command "missing"`) || !strings.Contains(stderr.String(), "--help") {
				t.Fatalf("usage streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
