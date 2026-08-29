package claude

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	cases := prepareHaikuGoldenReplayCases(t, manifest)
	router := newHaikuGoldenCommandRouter(t, cases)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                cases[0].factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: router,
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	seenSessions := make(map[string]struct{}, len(cases))
	t.Cleanup(func() { assertHaikuGoldenTopology(t, router, cases, seenSessions) })
	runHaikuGoldenCases(t, server, router, cases, seenSessions)
}

func runHaikuGoldenCases(
	t *testing.T,
	server *support.FunctionalAPIServer,
	router *haikuGoldenCommandRouter,
	cases []haikuGoldenReplayCase,
	seenSessions map[string]struct{},
) {
	t.Helper()
	for _, replayCase := range cases {
		replayCase := replayCase
		t.Run(replayCase.golden.Name, func(t *testing.T) {
			replayHaikuGoldenCase(t, server, router, replayCase, seenSessions)
		})
	}
}

func replayHaikuGoldenCase(
	t *testing.T,
	server *support.FunctionalAPIServer,
	router *haikuGoldenCommandRouter,
	replayCase haikuGoldenReplayCase,
	seenSessions map[string]struct{},
) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, server.URL(), replayCase.factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatal("explicit Claude golden session has no id")
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("golden session id = %q, want a non-default explicit session", sessionID)
	}
	if _, exists := seenSessions[sessionID]; exists {
		t.Fatalf("duplicate explicit golden session id %q", sessionID)
	}
	seenSessions[sessionID] = struct{}{}
	closed := false
	t.Cleanup(func() {
		if !closed {
			support.CloseFactorySessionAt(t, server.URL(), sessionID)
		}
	})

	name := "claude-haiku-" + replayCase.golden.Name
	support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "claude Haiku golden replay"},
	})
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 20*time.Second)
	assertSuccessfulHaikuGoldenWork(t, server, router, replayCase, sessionID)
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	closed = true
}

func assertSuccessfulHaikuGoldenWork(
	t *testing.T,
	server *support.FunctionalAPIServer,
	router *haikuGoldenCommandRouter,
	replayCase haikuGoldenReplayCase,
	sessionID string,
) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, sessionWorkURL(server.URL(), sessionID))
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	request, ok := router.RequestFor(replayCase.factoryDir)
	if !ok {
		t.Fatalf("no Claude command request recorded for golden %q", replayCase.golden.Name)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{
		"--model", replayCase.golden.Selector,
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	})
	assertHaikuGoldenInferenceResult(
		t,
		support.GetFactoryEventsForSessionAt(t, server.URL(), sessionID),
		replayCase.golden.SessionID,
	)
}

func assertHaikuGoldenTopology(
	t *testing.T,
	router *haikuGoldenCommandRouter,
	cases []haikuGoldenReplayCase,
	seenSessions map[string]struct{},
) {
	t.Helper()
	if len(seenSessions) != len(cases) {
		t.Errorf("explicit Claude golden sessions = %d, want %d", len(seenSessions), len(cases))
	}
	requests := router.Requests()
	if len(requests) != len(cases) {
		t.Errorf("shared Claude golden command calls = %d, want %d", len(requests), len(cases))
	}
	for index, replayCase := range cases {
		if index >= len(requests) {
			break
		}
		if got := normalizeHaikuGoldenRouteDirectory(requests[index].WorkDir); got != replayCase.factoryDir {
			t.Errorf("golden request %d work dir = %q, want pre-start route for case %q", index, got, replayCase.golden.Name)
		}
	}
	for _, replayCase := range cases {
		if got := router.CallsFor(replayCase.factoryDir); got != 1 {
			t.Errorf("golden route %q calls = %d, want 1", replayCase.golden.Name, got)
		}
	}
	router.Close()
	if got := router.RouteCount(); got != 0 {
		t.Errorf("closed golden route count = %d, want 0", got)
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
	if err := validateHaikuGoldenCase(golden); err != nil {
		t.Fatal(err)
	}
	raw, err := haikuGoldenFiles.ReadFile(haikuGoldenRoot + "/" + golden.StdoutFile)
	if err != nil {
		t.Fatalf("read Haiku golden %q: %v", golden.StdoutFile, err)
	}
	if err := validateHaikuGoldenStdout(golden, raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertHaikuGoldenNativeShape(t *testing.T, stdout []byte, golden haikuGoldenCase) {
	t.Helper()
	if err := validateHaikuGoldenNativeShape(stdout, golden); err != nil {
		t.Fatal(err)
	}
}

func validateHaikuGoldenCase(golden haikuGoldenCase) error {
	if golden.Name == "" || golden.Selector == "" || golden.ReportedModel == "" ||
		golden.SessionID == "" || golden.StdoutFile == "" || golden.SHA256 == "" {
		return fmt.Errorf("Haiku golden case metadata is incomplete: %#v", golden)
	}
	return nil
}

func validateHaikuGoldenStdout(golden haikuGoldenCase, raw []byte) error {
	if err := support.ValidateProviderSessionFixtureContent(
		"claude-haiku-"+golden.Name,
		golden.StdoutFile,
		raw,
	); err != nil {
		return fmt.Errorf("validate Haiku golden %q sanitization: %w", golden.StdoutFile, err)
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != golden.SHA256 {
		return fmt.Errorf("Haiku golden %q sha256 = %s, want %s", golden.StdoutFile, got, golden.SHA256)
	}
	return nil
}

func validateHaikuGoldenNativeShape(stdout []byte, golden haikuGoldenCase) error {
	var sawModel, sawDelta, sawResult bool
	for index, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("Haiku golden %q line %d: %w", golden.Name, index+1, err)
		}
		encoded := string(line)
		sawModel = sawModel || strings.Contains(encoded, `"model":"`+golden.ReportedModel+`"`)
		sawDelta = sawDelta || strings.Contains(encoded, `"type":"text_delta"`) &&
			strings.Contains(encoded, haikuGoldenResult)
		sawResult = sawResult || record["type"] == "result" && record["result"] == haikuGoldenResult
	}
	if !sawModel || !sawDelta || !sawResult {
		return fmt.Errorf("Haiku golden %q shape model=%t delta=%t result=%t", golden.Name, sawModel, sawDelta, sawResult)
	}
	return nil
}

func assertHaikuGoldenInferenceResult(t *testing.T, events []factoryapi.FactoryEvent, wantSessionID string) {
	t.Helper()
	if err := findHaikuGoldenInferenceResult(events, wantSessionID); err != nil {
		t.Fatal(err)
	}
}

func findHaikuGoldenInferenceResult(events []factoryapi.FactoryEvent, wantSessionID string) error {
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			return fmt.Errorf("decode Haiku inference response: %w", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded || payload.Response == nil ||
			strings.TrimSpace(*payload.Response) != haikuGoldenResult {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Id == nil ||
			*payload.ProviderSession.Id != wantSessionID {
			return fmt.Errorf("provider session = %#v, want id %q", payload.ProviderSession, wantSessionID)
		}
		return nil
	}
	return fmt.Errorf("Factory events omitted successful Haiku result %q", haikuGoldenResult)
}
