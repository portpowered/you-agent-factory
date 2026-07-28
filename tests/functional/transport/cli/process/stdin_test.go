package process_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestRunReadsPromptFromStdin proves you run - consumes piped stdin and
// delivers the exact prompt bytes to the selected worker through the public
// built you CLI process boundary.
func TestRunReadsPromptFromStdin(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	factoryPath := writeStdinRunFactory(t, session.WorkDir)
	mockWorkersPath := writeStdinRunDefaultMockWorkers(t)
	prompt := fmt.Sprintf("functional-stdin-run-café-résumé-%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"--quiet",
		"-",
	)

	result, err := session.RunWithStdin(ctx, prompt, args...)
	if err != nil {
		t.Fatalf(
			"you run - via stdin: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			result.Stdout,
			result.Stderr,
		)
	}
	if got := result.Stdout; got != prompt {
		t.Fatalf("worker-bound primary result = %q, want exact stdin prompt %q", got, prompt)
	}
}
