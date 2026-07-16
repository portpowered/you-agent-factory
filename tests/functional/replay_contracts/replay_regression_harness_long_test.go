//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayRegressionHarness_LoadsArtifactAndReportsSuccessfulReplay(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay CLI success sweep")

	artifactPath := support.RecordReplayFixture(t)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected replay fixture artifact to contain dispatches")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("expected replay fixture artifact to contain completions")
	}

	output, err := runReplayCLI(t, artifactPath)
	if err != nil {
		t.Fatalf("replay CLI failed: %v\n%s", err, output)
	}
}

func TestReplayRegressionHarness_ReportsCorruptedArtifactFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay CLI divergence sweep")

	artifact := testutil.LoadReplayArtifact(t, support.RecordReplayFixture(t))
	mutateFirstDispatchRequest(t, artifact)
	divergentPath := filepath.Join(t.TempDir(), "divergent-replay.json")
	writeReplayArtifact(t, divergentPath, artifact)

	output, err := runReplayCLI(t, divergentPath)
	if err == nil {
		t.Fatalf("corrupted replay CLI succeeded unexpectedly: %s", output)
	}
	if !strings.Contains(output, "replay divergence: category=dispatch_mismatch") {
		t.Fatalf("corrupted replay CLI output = %q, want stable dispatch-mismatch replay failure", output)
	}
}

func mutateFirstDispatchRequest(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	for i := range artifact.Events {
		event := testutil.GeneratedFactoryEvent(t, artifact.Events[i])
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch request event: %v", err)
		}
		payload.TransitionId = "unexpected-transition"
		var union factoryapi.FactoryEvent_Payload
		if err := union.FromDispatchRequestEventPayload(payload); err != nil {
			t.Fatalf("encode dispatch request event: %v", err)
		}
		event.Payload = union
		artifact.Events[i] = testutil.FactoryEvent(t, event)
		return
	}
	t.Fatal("artifact has no DISPATCH_REQUEST event")
}

func writeReplayArtifact(t *testing.T, path string, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal corrupted replay artifact: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write corrupted replay artifact: %v", err)
	}
}

func runReplayCLI(t *testing.T, artifactPath string) (string, error) {
	t.Helper()

	binaryPath := buildReplayCLIBinary(t)
	command := exec.Command(binaryPath, "run", "--replay", artifactPath)
	command.Dir = t.TempDir()
	command.Env = replayCLIEnvironment(t)
	output, err := command.CombinedOutput()
	return string(output), err
}

func buildReplayCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build replay CLI: %v\n%s", err, output)
	}
	return binaryPath
}

func replayCLIEnvironment(t *testing.T) []string {
	t.Helper()

	homeDir := t.TempDir()
	env := append([]string{}, os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}
