package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	workMoveSmokeRequestID = "cli-work-move-smoke"
	workMoveSmokeWorkName  = "smoke-move-task"
	workMoveSmokeWorkType  = "task"
)

func TestWorkMove_RealCLIMovesSubmittedWork(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI work move smoke")
	}

	dir := support.ScaffoldFactory(t, workMoveSmokeFactoryConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	batchPath := testutil.MustRepoPath(t, "tests/functional/smoke/testdata/cli_work_move_smoke.json")

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	runCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--continuously",
		"--quiet",
		mockWorkersPath,
	)
	runCmd.Dir = dir

	var runStdout, runStderr strings.Builder
	runCmd.Stdout = &runStdout
	runCmd.Stderr = &runStderr

	if err := runCmd.Start(); err != nil {
		t.Fatalf("start you run: %v", err)
	}

	runWait := make(chan error, 1)
	go func() {
		runWait <- runCmd.Wait()
	}()

	if err := waitForSmokeServerReady(ctx, baseURL, 20*time.Second); err != nil {
		if waitErr := <-runWait; waitErr != nil {
			t.Fatalf("you run: %v\nstdout:\n%s\nstderr:\n%s", waitErr, runStdout.String(), runStderr.String())
		}
		t.Fatalf("wait for factory API: %v\nstdout:\n%s\nstderr:\n%s", err, runStdout.String(), runStderr.String())
	}

	submitCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", baseURL,
		"submit", "batch",
		batchPath,
	)
	submitCmd.Dir = dir
	submitOut, err := submitCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}

	workID, err := waitForWorkMoveSmokeWorkID(ctx, baseURL, 15*time.Second)
	if err != nil {
		t.Fatalf("resolve work id: %v\nsubmit output:\n%s", err, submitOut)
	}

	moveCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", baseURL,
		"work", "move", workID, "complete",
	)
	moveCmd.Dir = dir
	moveOut, err := moveCmd.CombinedOutput()
	if err != nil {
		if waitErr := <-runWait; waitErr != nil {
			t.Fatalf("you work move: %v\noutput:\n%s\nyou run: %v", err, moveOut, waitErr)
		}
		t.Fatalf("you work move: %v\noutput:\n%s", err, moveOut)
	}

	output := string(moveOut)
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Previous state:\tinit",
		"New state:\tcomplete",
		"Session ID:\t~default",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("work move output missing %q:\n%s", marker, output)
		}
	}

	if err := waitForWorkMoveSmokeWorkAtState(ctx, baseURL, workID, "complete", 15*time.Second); err != nil {
		t.Fatalf("verify moved work: %v\nmove output:\n%s", err, output)
	}

	cancel()
	_ = <-runWait
}

func workMoveSmokeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-work-move-smoke",
		"workTypes": []map[string]any{
			{
				"name":   workMoveSmokeWorkType,
				"states": promptRunWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
	}
}

func waitForWorkMoveSmokeWorkID(ctx context.Context, baseURL string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/work", nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(25 * time.Millisecond):
				continue
			}
		}

		var workList factoryapi.ListWorkResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&workList)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return "", decodeErr
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GET /work status = %d", resp.StatusCode)
		}

		for _, item := range workList.Results {
			if item.Name != workMoveSmokeWorkName {
				continue
			}
			if stringPointerValue(item.WorkTypeName) != workMoveSmokeWorkType {
				continue
			}
			if item.State == nil || item.State.Name != "init" {
				continue
			}
			workID := stringPointerValue(item.WorkId)
			if workID == "" {
				return "", fmt.Errorf("work %q missing workId", workMoveSmokeWorkName)
			}
			return workID, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return "", fmt.Errorf("timed out waiting for work %q (%s) in init", workMoveSmokeWorkName, workMoveSmokeWorkType)
}

func waitForWorkMoveSmokeWorkAtState(ctx context.Context, baseURL, workID, stateName string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/work/"+workID, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(25 * time.Millisecond):
				continue
			}
		}

		var work factoryapi.Work
		decodeErr := json.NewDecoder(resp.Body).Decode(&work)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET /work/%s status = %d", workID, resp.StatusCode)
		}
		if work.State != nil && work.State.Name == stateName {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for work %q at state %q", workID, stateName)
}
