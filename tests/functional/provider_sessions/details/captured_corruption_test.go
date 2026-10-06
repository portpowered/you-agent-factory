package details

import (
	"bytes"
	"encoding/json"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Public detail reads must distinguish unreadable retained history from absence.
func TestCodexCapturedDetailsFailureMatrix(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "corruption-source")
	support.ClearSeedInputs(t, dir)
	runner := capturedCodexRunner{testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: capturedCodexOutput("captured-direct")})}
	host := startCapturedCodexHost(t, home, dir, runner, nil)
	invokeCapturedCodex(t, host, home, dir, "direct", "")
	assertCapturedCodexDetail(t, awaitCapturedCodexDetail(t, host, "captured-direct"), "captured-direct")
	host.Close(t)
	for name, damage := range map[string]func([]byte) []byte{
		"corrupt": func(data []byte) []byte { return append(data, []byte("{broken-corrupt-record}\n")...) },
		"torn":    func(data []byte) []byte { return append(data, []byte("{\"uncommitted\":")...) },
		"incomplete": func(data []byte) []byte {
			lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
			for i, line := range lines {
				if bytes.Contains(line, []byte(`"SourceEventID":"terminal"`)) {
					// Preserve the captured association but remove its terminal fact.
					lines[i] = bytes.Replace(line, []byte(`"phase":"COMPLETED"`), []byte(`"phase":"UPDATED"`), 1)
					lines[i] = bytes.Replace(lines[i], []byte(`"status":"COMPLETED"`), []byte(`"status":"RUNNING"`), 1)
					return append(bytes.Join(lines, []byte{'\n'}), '\n')
				}
			}
			panic("fixture has no terminal")
		},
		"truncated": func(data []byte) []byte { return data[:len(data)-10] },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertDamagedCodexReload(t, dir, damage)
		})
	}
	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		faultHome, faultDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "capture-read-fault")
		support.ClearSeedInputs(t, faultDir)
		copyCapturedCodexStore(t, dir, faultDir)
		failed := startCapturedCodexHost(t, faultHome, faultDir, testutil.NewProviderCommandRunner(), func(string) ([]byte, error) {
			return nil, privateStorageFault()
		})
		body := getCodexProviderSessionDetailErrorBody(t, failed.URL(), "captured-direct", http.StatusInternalServerError)
		assertStorageFailureResponse(t, body, faultHome)
	})
}

func assertDamagedCodexReload(t *testing.T, dir string, damage func([]byte) []byte) {
	t.Helper()
	damagedHome, damagedDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-corrupt")
	support.ClearSeedInputs(t, damagedDir)
	copyCapturedCodexStore(t, dir, damagedDir)
	mutateCapturedCodexJournals(t, damagedDir, damage)
	damaged := startCapturedCodexHost(t, damagedHome, damagedDir, testutil.NewProviderCommandRunner(), nil)
	body := getCodexProviderSessionDetailErrorBody(t, damaged.URL(), "captured-direct", http.StatusInternalServerError)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeINTERNALERROR, damagedHome)
}

func mutateCapturedCodexJournals(t *testing.T, dir string, change func([]byte) []byte) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("journal fixture: %v %v", files, err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, change(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodexCapturedDetailsAmbiguousAndEmpty(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "captured-boundaries")
	support.ClearSeedInputs(t, dir)
	output := capturedCodexOutput("shared-provider-id")
	empty := []byte("{\"type\":\"thread.started\",\"thread_id\":\"empty-provider-id\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"empty\",\"type\":\"agent_message\",\"text\":\"fixture content\"}}\n")
	runner := capturedCodexRunner{testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: empty},
		platformprocess.CommandResult{Stdout: output},
		platformprocess.CommandResult{Stdout: output})}
	host := startCapturedCodexHost(t, home, dir, runner, nil)
	invokeCapturedCodex(t, host, home, dir, "empty-worker", "")
	awaitCapturedCodexDetail(t, host, "empty-provider-id")
	invokeCapturedCodex(t, host, home, dir, "first-worker", "")
	assertCapturedCodexDetail(t, awaitCapturedCodexDetail(t, host, "shared-provider-id"), "shared-provider-id")
	invokeCapturedCodex(t, host, home, dir, "second-worker", "")
	// Reload after joining capture removes transient ordering from the ambiguity assertion.
	host.Close(t)
	freshHome, freshDir := t.TempDir(), support.ScaffoldSingleStepFactory(t, "ambiguous-reload")
	support.ClearSeedInputs(t, freshDir)
	copyCapturedCodexStore(t, dir, freshDir)
	// Codex execution requires a final message. Model a valid retained capture
	// with no message content by clearing only text in a stopped, test-owned copy.
	mutateCapturedCodexJournals(t, freshDir, func(data []byte) []byte {
		lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
		for i, line := range lines {
			var record any
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatal(err)
			}
			clearCapturedFixtureText(record)
			lines[i], _ = json.Marshal(record)
		}
		return append(bytes.Join(lines, []byte{'\n'}), '\n')
	})
	fresh := startCapturedCodexHost(t, freshHome, freshDir, testutil.NewProviderCommandRunner(), nil)
	detail := awaitCapturedCodexDetail(t, fresh, "empty-provider-id")
	if len(detail.Transcript) != 0 || detail.Parse.TokenUsage != nil || detail.Parse.FunctionCalls == nil || detail.Parse.ParseErrors == nil || detail.Parse.Reasoning == nil || detail.Parse.Turns == nil || detail.Parse.UnknownEvents == nil {
		t.Fatalf("complete empty capture fabricated content or omitted required arrays: %+v", detail)
	}
	body := getCodexProviderSessionDetailErrorBody(t, fresh.URL(), "shared-provider-id", http.StatusInternalServerError)
	assertCapturedCodexFailure(t, body, factoryapi.ErrorResponseCodeINTERNALERROR, freshHome)
}

func clearCapturedFixtureText(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.EqualFold(key, "text") || strings.EqualFold(key, "textDelta") {
				value[key] = ""
			} else {
				clearCapturedFixtureText(child)
			}
		}
	case []any:
		for _, child := range value {
			clearCapturedFixtureText(child)
		}
	}
}
