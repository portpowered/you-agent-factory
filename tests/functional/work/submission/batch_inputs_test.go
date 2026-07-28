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

	batchInputsInlineRequestID = "work-batch-inputs-inline"
	batchInputsInlineWorkName  = "inline-shape-task"

	batchInputsFileRequestID = "work-batch-inputs-file"
	batchInputsFileWorkName  = "file-shape-task"

	batchInputsStdinRequestID = "work-batch-inputs-stdin"
	batchInputsStdinWorkName    = "stdin-shape-task"
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

type batchInputsSubmitJSON struct {
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId"`
	WorkCount int    `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func batchInputsFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-batch-inputs",
		"workTypes": []map[string]any{
			{
				"name": batchInputsWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": batchInputsWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": batchInputsWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": batchInputsWorkType, "state": "failed"}},
			},
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

	listed := support.ListDefaultSessionWork(t, baseURL)
	if !listedWorkContainsNameAndID(listed, workName, workID) {
		t.Fatalf(
			"public work list missing submitted work name=%q workId=%q: %#v",
			workName,
			workID,
			listed.Results,
		)
	}
}

func listedWorkContainsNameAndID(listed factoryapi.ListWorkResponse, workName, workID string) bool {
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			return true
		}
	}
	return false
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
