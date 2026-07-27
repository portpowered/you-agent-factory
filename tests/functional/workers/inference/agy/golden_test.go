package agy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agyFinalOnlySuccessGoldenCase = "final-only-success"

// TestAgyGoldenFinalOnlySuccess replays a sanitized Agy final-only-success
// transcript through the customer process boundary and proves public final-only
// success without fabricated streaming deltas or structured snapshot events.
//golden: docs/temp/functional/provider-sessions/agy/final-only-success/manifest.json
func TestAgyGoldenFinalOnlySuccess(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("agy", agyFinalOnlySuccessGoldenCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "agy-final-only-success" {
		t.Fatalf("manifest.ID = %q, want agy-final-only-success", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf("manifest.fidelityClass = %q, want %q", loaded.Manifest.FidelityClass, support.ProviderSessionFidelityFinalOnly)
	}

	var request struct {
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" || request.SessionID == "" {
		t.Fatalf("request.json = %#v, want model and session_id", request)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAgy, request.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy golden final-only success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeAgyFixtureExecutable(t)
	ptyHost := newAgyGoldenFixturePTYHost(loaded.Stdout.Raw, exitCode)
	clock := platformclock.NewDeterministic(time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC), time.Millisecond)

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			AgyPTYHost:               ptyHost,
			AgyPTYClock:              clock,
			WorkersExecutableLocator: fixedExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:   identityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if ptyHost.callCount() != 1 {
		t.Fatalf("agy PTY host starts = %d, want one invocation", ptyHost.callCount())
	}

	inferencePayload, dispatchOutput := agyGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Provider == nil {
		t.Fatal("inference response missing provider session metadata")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Provider); got != string(modelprovider.ProviderAgy) {
		t.Fatalf("provider session provider = %q, want %q", got, modelprovider.ProviderAgy)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	assertAgyFinalOnlyPublicResponseEvents(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:   observeAgyProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeAgyResponseEventGoldens(responseEvents),
		InvocationResult: observeAgyInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

type fixedExecutableLocator struct {
	path string
}

func (l fixedExecutableLocator) LookPath(file string) (string, error) {
	if file == "agy" {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func identityResolveSymlinks(path string) (string, error) {
	return path, nil
}

func writeAgyFixtureExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("agy-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write agy fixture executable: %v", err)
	}
	return path
}

type agyGoldenFixturePTYHost struct {
	stdout   []byte
	exitCode int
	starts   int
}

func newAgyGoldenFixturePTYHost(stdout []byte, exitCode int) *agyGoldenFixturePTYHost {
	return &agyGoldenFixturePTYHost{
		stdout:   append([]byte(nil), stdout...),
		exitCode: exitCode,
	}
}

func (h *agyGoldenFixturePTYHost) callCount() int {
	return h.starts
}

func (h *agyGoldenFixturePTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	return agyGoldenPTYAllocation{}, nil
}

func (h *agyGoldenFixturePTYHost) Start(
	_ platformpty.ProcessLaunch,
	_ platformpty.Allocation,
) (platformpty.Process, io.ReadCloser, error) {
	h.starts++
	return &agyGoldenPTYProcess{exitCode: h.exitCode}, io.NopCloser(bytes.NewReader(h.stdout)), nil
}

type agyGoldenPTYAllocation struct{}

func (agyGoldenPTYAllocation) Close() error           { return nil }
func (agyGoldenPTYAllocation) Kind() platformpty.Kind { return platformpty.KindPOSIX }

type agyGoldenPTYProcess struct {
	exitCode int
}

func (p *agyGoldenPTYProcess) Wait() error      { return nil }
func (p *agyGoldenPTYProcess) Terminate() error { return nil }
func (*agyGoldenPTYProcess) Close()             {}
func (*agyGoldenPTYProcess) PID() int           { return 0 }
func (p *agyGoldenPTYProcess) ExitCode() int    { return p.exitCode }

func assertAgyFinalOnlyPublicResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()

	var completedMessages int
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool:
			t.Fatalf("final-only replay fabricated tool lifecycle: %#v", event)
		case factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only replay fabricated usage lifecycle: %#v", event)
		}
	}
	if completedMessages == 0 {
		t.Fatal("final-only replay missing authoritative completed message")
	}
}

func agyGoldenInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, string) {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
		dispatchOutput   string
	)
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeInferenceResponse:
			payload, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode inference response: %v", err)
			}
			if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
				continue
			}
			inferencePayload = payload
			foundInference = true
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response: %v", err)
			}
			if payload.Output != nil && *payload.Output != "" {
				dispatchOutput = *payload.Output
			}
		}
	}
	if !foundInference {
		t.Fatal("missing succeeded INFERENCE_RESPONSE in factory events")
	}
	if dispatchOutput == "" {
		t.Fatal("missing dispatch output in factory events")
	}
	return inferencePayload, dispatchOutput
}

func observeAgyProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderAgy)
	sessionID := ""
	if payload.ProviderSession != nil {
		if payload.ProviderSession.Provider != nil {
			provider = support.StringPointerValue(payload.ProviderSession.Provider)
		}
		if payload.ProviderSession.Id != nil {
			sessionID = support.StringPointerValue(payload.ProviderSession.Id)
		}
	}
	record := map[string]string{
		"provider":          provider,
		"providerSessionId": sessionID,
		"fidelityClass":     manifest.FidelityClass,
		"status":            status,
	}
	return mustMarshalJSON(record)
}

func observeAgyResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}
		if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			continue
		}
		message, err := event.Payload.AsFactoryResponseEventMessagePayload()
		if err != nil {
			continue
		}
		text := agyGoldenMessageText(message)
		if text == "" {
			continue
		}
		record := map[string]any{
			"type":             "message.completed",
			"eventId":          event.EventId,
			"factorySessionId": event.FactorySessionId,
			"runId":            event.RunId,
			"itemId":           agyGoldenItemID(event),
			"text":             text,
			"finishReason":     "stop",
		}
		records = append(records, mustMarshalJSON(record))
	}
	return records
}

func observeAgyInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	record := map[string]any{
		"ok":           ok,
		"content":      content,
		"finishReason": "stop",
	}
	return mustMarshalJSON(record)
}

func agyGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func agyGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range message.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
