package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

func runBoardPersistenceCLI(
	ctx context.Context,
	binaryPath, factoryDir, homeDir, baseURL string,
	args ...string,
) ([]byte, error) {
	commandArgs := append([]string{"--server", baseURL}, args...)
	command := exec.CommandContext(ctx, binaryPath, commandArgs...)
	command.Dir = factoryDir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	return command.CombinedOutput()
}

func submitBatchThroughCLI(
	t *testing.T,
	ctx context.Context,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir, batchJSON, requestID string,
	wantWorkCount int,
) {
	t.Helper()
	output, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "submit", "batch", batchJSON)
	if err != nil {
		t.Fatalf("you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	var acknowledgement struct {
		RequestID string `json:"requestId"`
		WorkCount int    `json:"workCount"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &acknowledgement); err != nil {
		t.Fatalf("decode you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	if acknowledgement.RequestID != requestID || acknowledgement.WorkCount != wantWorkCount {
		t.Fatalf("you submit batch acknowledgement = %#v, want requestId %q and workCount %d", acknowledgement, requestID, wantWorkCount)
	}
}
