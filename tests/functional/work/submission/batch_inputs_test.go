package submission_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	batchInputsWorkType = "task"
	batchInputsAltWorkType = "review"

	batchInputsInlineRequestID = "work-batch-inputs-inline"
	batchInputsInlineWorkName  = "inline-shape-task"

	batchInputsFileRequestID = "work-batch-inputs-file"
	batchInputsFileWorkName  = "file-shape-task"

	batchInputsStdinRequestID = "work-batch-inputs-stdin"
	batchInputsStdinWorkName  = "stdin-shape-task"

	batchInputsDefaultTypeRequestID = "work-batch-inputs-default-type"
	batchInputsDefaultTypeWorkName  = "default-type-task"

	batchInputsExplicitTypeRequestID = "work-batch-inputs-explicit-type"
	batchInputsExplicitTypeWorkName  = "explicit-type-review"

	batchInputsUnknownTypeRequestID = "work-batch-inputs-unknown-type"
	batchInputsUnknownTypeWorkName  = "unknown-type-task"
	batchInputsUnknownWorkType      = "nonexistent-type"

	batchInputsMixedBatchRequestID   = "work-batch-inputs-mixed-batch"
	batchInputsMixedValidWorkName    = "mixed-valid-task"
	batchInputsMixedInvalidWorkName  = "mixed-invalid-task"
)

// TestWorkBatchAcceptsInlineFileAndStdinShapes proves the public Work Request
// batch ingress accepts the same canonical FACTORY_REQUEST_BATCH document when
// provided inline, via a filesystem path, or via stdin, and that each ingress
// path yields customer-visible accept outcomes for the submitted works.
func TestWorkBatchAcceptsInlineFileAndStdinShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work batch input shape sweep")
	}

	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	canonicalBatch := func(requestID, workName string) string {
		return fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Batch inputs shape parity"}}]}`,
			requestID,
			workName,
			batchInputsWorkType,
		)
	}

	t.Run("inline", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsInlineRequestID, batchInputsInlineWorkName)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch inline: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsInlineRequestID, batchInputsInlineWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsInlineWorkName, submitted.Works[0].WorkID)
	})

	t.Run("file", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsFileRequestID, batchInputsFileWorkName)
		batchPath := filepath.Join(t.TempDir(), "batch-inputs-shape.json")
		if err := os.WriteFile(batchPath, []byte(batchJSON), 0o600); err != nil {
			t.Fatalf("write batch file: %v", err)
		}

		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchPath, nil)
		if err != nil {
			t.Fatalf("you submit batch file: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsFileRequestID, batchInputsFileWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsFileWorkName, submitted.Works[0].WorkID)
	})

	t.Run("stdin", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsStdinRequestID, batchInputsStdinWorkName)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, "-", strings.NewReader(batchJSON))
		if err != nil {
			t.Fatalf("you submit batch stdin: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsStdinRequestID, batchInputsStdinWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsStdinWorkName, submitted.Works[0].WorkID)
	})
}

// TestWorkBatchSelectsDefaultAndExplicitWorkTypes proves public Work Request
// batch ingress materializes the Factory default work type when a batch work
// entry omits workTypeName and honors an explicit workTypeName when provided.
func TestWorkBatchSelectsDefaultAndExplicitWorkTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work batch work-type selection sweep")
	}

	factoryDir := support.ScaffoldFactory(t, batchWorkTypeSelectionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("default", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"payload":{"title":"Default work type selection"}}]}`,
			batchInputsDefaultTypeRequestID,
			batchInputsDefaultTypeWorkName,
		)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch default work type: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchWorkListedWithWorkType(
			t,
			baseURL,
			batchInputsDefaultTypeWorkName,
			submitted.Works[0].WorkID,
			batchInputsWorkType,
		)
	})

	t.Run("explicit", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Explicit work type selection"}}]}`,
			batchInputsExplicitTypeRequestID,
			batchInputsExplicitTypeWorkName,
			batchInputsAltWorkType,
		)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch explicit work type: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchWorkListedWithWorkType(
			t,
			baseURL,
			batchInputsExplicitTypeWorkName,
			submitted.Works[0].WorkID,
			batchInputsAltWorkType,
		)
	})
}

// TestWorkBatchRejectsUnknownTypeWithoutPartialMutation proves public Work Request
// batch ingress rejects batches that name an unknown work type with a
// customer-visible failure and leaves durable Work unchanged for that request.
func TestWorkBatchRejectsUnknownTypeWithoutPartialMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work batch unknown-type rejection sweep")
	}

	factoryDir := support.ScaffoldFactory(t, batchWorkTypeSelectionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baselineListed := support.ListDefaultSessionWork(t, baseURL)
	baselineCount := len(baselineListed.Results)

	t.Run("unknown_type", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Unknown work type rejection"}}]}`,
			batchInputsUnknownTypeRequestID,
			batchInputsUnknownTypeWorkName,
			batchInputsUnknownWorkType,
		)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchJSON, nil)
		assertBatchSubmitRejected(t, output, err, batchInputsUnknownTypeRequestID)
		assertWorkNotListedByName(t, baseURL, batchInputsUnknownTypeWorkName)
		assertListedWorkCount(t, baseURL, baselineCount)
	})

	t.Run("mixed_batch_no_partial_submit", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Valid work in mixed batch"}},{"name":%q,"workTypeName":%q,"payload":{"title":"Invalid work in mixed batch"}}]}`,
			batchInputsMixedBatchRequestID,
			batchInputsMixedValidWorkName,
			batchInputsWorkType,
			batchInputsMixedInvalidWorkName,
			batchInputsUnknownWorkType,
		)
		output, err := runYouSubmitBatch(ctx, binaryPath, factoryDir, baseURL, batchJSON, nil)
		assertBatchSubmitRejected(t, output, err, batchInputsMixedBatchRequestID)
		assertWorkNotListedByName(t, baseURL, batchInputsMixedValidWorkName)
		assertWorkNotListedByName(t, baseURL, batchInputsMixedInvalidWorkName)
		assertListedWorkCount(t, baseURL, baselineCount)
	})
}

type batchInputsSubmitJSON struct {
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId"`
	WorkCount int    `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func batchWorkTypeStates() []map[string]any {
	return []map[string]any{
		{"name": "init", "type": "INITIAL"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
}

func batchWorkTypeWorkstation(name, workType string) map[string]any {
	return map[string]any{
		"name":      name,
		"worker":    "mock-worker",
		"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
		"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
		"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
	}
}

func batchInputsFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-batch-inputs",
		"workTypes": []map[string]any{
			{
				"name":   batchInputsWorkType,
				"states": batchWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			batchWorkTypeWorkstation("process-task", batchInputsWorkType),
		},
	}
}

func batchWorkTypeSelectionFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-batch-work-type-selection",
		"workTypes": []map[string]any{
			{
				"name":             batchInputsWorkType,
				"handlingBehavior": []string{"DEFAULT"},
				"states":           batchWorkTypeStates(),
			},
			{
				"name":   batchInputsAltWorkType,
				"states": batchWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			batchWorkTypeWorkstation("process-task", batchInputsWorkType),
			batchWorkTypeWorkstation("process-review", batchInputsAltWorkType),
		},
	}
}

func buildYouCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func runYouSubmitBatch(
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	batchSource string,
	stdin io.Reader,
) ([]byte, error) {
	args := []string{"--server", serverURL, "--json", "submit", "batch", batchSource}
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = workingDir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	return cmd.CombinedOutput()
}

func decodeBatchSubmitJSON(t *testing.T, output []byte) batchInputsSubmitJSON {
	t.Helper()

	var submitted batchInputsSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(output), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing accepted work identity: %#v", submitted)
	}
	if strings.TrimSpace(submitted.RequestID) == "" || strings.TrimSpace(submitted.TraceID) == "" {
		t.Fatalf("submit batch response missing request or trace identity: %#v", submitted)
	}
	return submitted
}

func assertBatchSubmitAcknowledgment(t *testing.T, output []byte, requestID, workName string) {
	t.Helper()

	text := string(output)
	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		`"traceId":`,
		`"workCount":1`,
		workName,
		batchInputsWorkType,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, text)
		}
	}
}

func assertBatchWorkListedAfterSubmit(t *testing.T, baseURL, workName, workID string) {
	t.Helper()
	assertBatchWorkListedWithWorkType(t, baseURL, workName, workID, "")
}

func assertBatchWorkListedWithWorkType(t *testing.T, baseURL, workName, workID, workType string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	item, ok := findListedWorkByNameAndID(listed, workName, workID)
	if !ok {
		t.Fatalf(
			"public work list missing submitted work name=%q workId=%q: %#v",
			workName,
			workID,
			listed.Results,
		)
	}
	if workType == "" {
		return
	}
	if support.StringPointerValue(item.WorkTypeName) != workType {
		t.Fatalf(
			"public work list workTypeName = %q, want %q for name=%q workId=%q: %#v",
			support.StringPointerValue(item.WorkTypeName),
			workType,
			workName,
			workID,
			item,
		)
	}
}

func findListedWorkByNameAndID(listed factoryapi.ListWorkResponse, workName, workID string) (factoryapi.Work, bool) {
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func listedWorkContainsNameAndID(listed factoryapi.ListWorkResponse, workName, workID string) bool {
	_, ok := findListedWorkByNameAndID(listed, workName, workID)
	return ok
}

func assertBatchSubmitRejected(t *testing.T, output []byte, err error, requestID string) {
	t.Helper()

	if err == nil {
		t.Fatalf("you submit batch unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you submit batch error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("you submit batch exit code = %d, want 1", exitErr.ExitCode())
	}

	text := string(output)
	for _, marker := range []string{
		"batch submission failed (400)",
		"code=BAD_REQUEST",
		"family=BAD_REQUEST",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch rejection output missing %q:\n%s", marker, text)
		}
	}

	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		"requestId: " + requestID,
		`"traceId":`,
		"traceId:",
		`"workCount":`,
		"work count:",
	} {
		if strings.Contains(text, marker) {
			t.Fatalf("submit batch rejection output must not contain success acknowledgment marker %q:\n%s", marker, text)
		}
	}
}

func assertWorkNotListedByName(t *testing.T, baseURL, workName string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	for _, item := range listed.Results {
		if item.Name == workName {
			t.Fatalf(
				"public work list unexpectedly contains rejected work name=%q: %#v",
				workName,
				item,
			)
		}
	}
}

func assertListedWorkCount(t *testing.T, baseURL string, want int) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	if got := len(listed.Results); got != want {
		t.Fatalf("public work list count = %d, want %d after rejected batch: %#v", got, want, listed.Results)
	}
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func jsonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", value)
	}
	return string(encoded)
}
