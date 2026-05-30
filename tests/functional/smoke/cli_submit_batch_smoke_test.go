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
	submitBatchSmokeRequestID = "cli-submit-batch-smoke"
	submitBatchSmokeWorkName  = "smoke-task"
	submitBatchSmokeWorkType  = "task"
)

func TestSubmitBatch_RealCLIUpsertsToRunningFactory(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit batch smoke")
	}

	dir := support.ScaffoldFactory(t, submitBatchSmokeFactoryConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	batchPath := testutil.MustRepoPath(t, "tests/functional/smoke/testdata/cli_submit_batch_smoke.json")

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
		if waitErr := <-runWait; waitErr != nil {
			t.Fatalf("you submit batch: %v\nsubmit output:\n%s\nyou run: %v\nstdout:\n%s\nstderr:\n%s", err, submitOut, waitErr, runStdout.String(), runStderr.String())
		}
		t.Fatalf("you submit batch: %v\noutput:\n%s\nyou run stdout:\n%s\nyou run stderr:\n%s", err, submitOut, runStdout.String(), runStderr.String())
	}

	output := string(submitOut)
	for _, marker := range []string{
		"requestId: " + submitBatchSmokeRequestID,
		"traceId:",
		"work count: 1",
		submitBatchSmokeWorkName + " (" + submitBatchSmokeWorkType + ")",
		"you work show",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, output)
		}
	}

	if err := waitForSubmitBatchSmokeWorkAccepted(ctx, baseURL, 15*time.Second); err != nil {
		t.Fatalf("wait for upserted work: %v\nsubmit output:\n%s", err, output)
	}

	cancel()
	_ = <-runWait
}

func submitBatchSmokeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-submit-batch-smoke",
		"workTypes": []map[string]any{
			{
				"name":   submitBatchSmokeWorkType,
				"states": promptRunWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": submitBatchSmokeWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": submitBatchSmokeWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": submitBatchSmokeWorkType, "state": "failed"}},
			},
		},
	}
}

func waitForSmokeServerReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/work", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for GET /work on %s", baseURL)
}

func waitForSubmitBatchSmokeWorkAccepted(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/work", nil)
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

		var workList factoryapi.ListWorkResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&workList)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GET /work status = %d", resp.StatusCode)
		}

		for _, item := range workList.Results {
			if item.Name != submitBatchSmokeWorkName {
				continue
			}
			if stringPointerValue(item.WorkTypeName) == submitBatchSmokeWorkType {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for work %q (%s)", submitBatchSmokeWorkName, submitBatchSmokeWorkType)
}
