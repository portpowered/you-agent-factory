package claude

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	haikuGoldenRoot   = "testdata/haiku_stream_json"
	haikuGoldenResult = "HAIKU GOLDEN COMPLETE"
)

//go:embed testdata/haiku_stream_json/manifest.json testdata/haiku_stream_json/*.jsonl
var haikuGoldenFiles embed.FS

type haikuGoldenManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	Source        haikuGoldenSource `json:"source"`
	Cases         []haikuGoldenCase `json:"cases"`
}

type haikuGoldenSource struct {
	Command         string `json:"command"`
	ProviderVersion string `json:"providerVersion"`
	Prompt          string `json:"prompt"`
}

type haikuGoldenCase struct {
	Name          string `json:"name"`
	Selector      string `json:"selector"`
	ReportedModel string `json:"reportedModel"`
	SessionID     string `json:"sessionId"`
	StdoutFile    string `json:"stdoutFile"`
	SHA256        string `json:"sha256"`
}

// TestClaudeHaikuStreamJSONGoldens replays sanitized streams captured from
// three live Claude Haiku selectors through the customer process boundary.
func TestClaudeHaikuStreamJSONGoldens(t *testing.T) {
	manifest := loadHaikuGoldenManifest(t)
	for _, golden := range manifest.Cases {
		golden := golden
		t.Run(golden.Name, func(t *testing.T) {
			stdout := loadHaikuGoldenStdout(t, golden)
			assertHaikuGoldenNativeShape(t, stdout, golden)
			replayHaikuGolden(t, golden, stdout)
		})
	}
}

func loadHaikuGoldenManifest(t *testing.T) haikuGoldenManifest {
	t.Helper()
	raw, err := haikuGoldenFiles.ReadFile(haikuGoldenRoot + "/manifest.json")
	if err != nil {
		t.Fatalf("read Haiku golden manifest: %v", err)
	}
	var manifest haikuGoldenManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode Haiku golden manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.Cases) != 3 {
		t.Fatalf("Haiku golden manifest = %#v, want schema 1 with three cases", manifest)
	}
	for _, required := range []string{"-p", "--verbose", "--output-format stream-json", "--include-partial-messages"} {
		if !strings.Contains(manifest.Source.Command, required) {
			t.Fatalf("capture command %q missing %q", manifest.Source.Command, required)
		}
	}
	if manifest.Source.ProviderVersion == "" || manifest.Source.Prompt == "" {
		t.Fatalf("Haiku golden source metadata is incomplete: %#v", manifest.Source)
	}
	return manifest
}

func loadHaikuGoldenStdout(t *testing.T, golden haikuGoldenCase) []byte {
	t.Helper()
	if golden.Name == "" || golden.Selector == "" || golden.ReportedModel == "" ||
		golden.SessionID == "" || golden.StdoutFile == "" || golden.SHA256 == "" {
		t.Fatalf("Haiku golden case metadata is incomplete: %#v", golden)
	}
	raw, err := haikuGoldenFiles.ReadFile(haikuGoldenRoot + "/" + golden.StdoutFile)
	if err != nil {
		t.Fatalf("read Haiku golden %q: %v", golden.StdoutFile, err)
	}
	if err := support.ValidateProviderSessionFixtureContent(
		"claude-haiku-"+golden.Name,
		golden.StdoutFile,
		raw,
	); err != nil {
		t.Fatalf("validate Haiku golden %q sanitization: %v", golden.StdoutFile, err)
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != golden.SHA256 {
		t.Fatalf("Haiku golden %q sha256 = %s, want %s", golden.StdoutFile, got, golden.SHA256)
	}
	return raw
}

func assertHaikuGoldenNativeShape(t *testing.T, stdout []byte, golden haikuGoldenCase) {
	t.Helper()
	var sawModel, sawDelta, sawResult bool
	for index, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("Haiku golden %q line %d: %v", golden.Name, index+1, err)
		}
		encoded := string(line)
		sawModel = sawModel || strings.Contains(encoded, `"model":"`+golden.ReportedModel+`"`)
		sawDelta = sawDelta || strings.Contains(encoded, `"type":"text_delta"`) &&
			strings.Contains(encoded, haikuGoldenResult)
		sawResult = sawResult || record["type"] == "result" && record["result"] == haikuGoldenResult
	}
	if !sawModel || !sawDelta || !sawResult {
		t.Fatalf("Haiku golden %q shape model=%t delta=%t result=%t", golden.Name, sawModel, sawDelta, sawResult)
	}
}

func replayHaikuGolden(t *testing.T, golden haikuGoldenCase, stdout []byte) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		golden.Selector,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude Haiku golden replay"}`))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Claude command calls = %d, want 1", runner.CallCount())
	}
	support.AssertArgsContainSequence(t, runner.LastRequest().Args, []string{
		"--model", golden.Selector,
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	})
	assertHaikuGoldenInferenceResult(t, events, golden.SessionID)
}

func assertHaikuGoldenInferenceResult(t *testing.T, events []factoryapi.FactoryEvent, wantSessionID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode Haiku inference response: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded || payload.Response == nil ||
			strings.TrimSpace(*payload.Response) != haikuGoldenResult {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Id == nil ||
			*payload.ProviderSession.Id != wantSessionID {
			t.Fatalf("provider session = %#v, want id %q", payload.ProviderSession, wantSessionID)
		}
		return
	}
	t.Fatalf("Factory events omitted successful Haiku result %q", haikuGoldenResult)
}
