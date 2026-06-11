package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
)

func TestRunDispatches_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-success-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 dispatches (1):",
		"- disp-petri-success-001 COMPLETED PETRI_TRANSITION",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunDispatches_AsyncPetriRunFixtureIncludesProviderSessionRefs(t *testing.T) {
	service := newContractFakeService(t)
	if err := sessionexecution.RunAsync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-run-001",
			FactoryID: "customer-support-triage",
		},
		Output:  ioDiscard{},
		Service: service,
	}); err != nil {
		t.Fatalf("seed RunAsync: %v", err)
	}

	var output bytes.Buffer
	if err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-run-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunDispatches: %v", err)
	}

	text := output.String()
	if !strings.Contains(text, "dispatches (1):") {
		t.Fatalf("output = %q, want one dispatch", text)
	}
	if !strings.Contains(text, "provider: prov-sess-disp-petri-001") {
		t.Fatalf("output missing provider session ref:\n%s", text)
	}
}

func TestRunDispatches_SuccessFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	base := sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunDispatches(context.Background(), first); err != nil {
		t.Fatalf("first RunDispatches: %v", err)
	}

	listed, err := service.ListDispatches(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	wantHash, err := fixtures.ListDispatchesResultHash(listed)
	if err != nil {
		t.Fatalf("ListDispatchesResultHash: %v", err)
	}
	if wantHash != "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var mapped factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &mapped); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if mapped.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q", mapped.SessionId)
	}
	if mapped.Dispatches == nil || len(mapped.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", mapped.Dispatches)
	}
	if mapped.Dispatches[0].Id != "disp-petri-success-001" {
		t.Fatalf("dispatch id = %q", mapped.Dispatches[0].Id)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunDispatches(context.Background(), second); err != nil {
		t.Fatalf("second RunDispatches: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent dispatch reads")
	}

	wantJSON, err := json.Marshal(factorysession.ListDispatchesResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared ListDispatchesResponseToAPI projection")
	}
}

func TestRunArtifacts_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	var output bytes.Buffer
	if err := sessionexecution.RunArtifacts(context.Background(), sessionexecution.ArtifactsConfig{
		SessionID: "dur-sess-petri-success-001",
		Output:    &output,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunArtifacts: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 artifacts (1):",
		"- art-petri-final-001 Triage summary FINAL_RESULT",
		"/factory-sessions/dur-sess-petri-success-001/artifacts/art-petri-final-001",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunArtifacts_SuccessFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedSyncSuccessSession(t, service)

	base := sessionexecution.ArtifactsConfig{
		SessionID: "dur-sess-petri-success-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunArtifacts(context.Background(), first); err != nil {
		t.Fatalf("first RunArtifacts: %v", err)
	}

	listed, err := service.ListArtifacts(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	wantHash, err := fixtures.ListArtifactsResultHash(listed)
	if err != nil {
		t.Fatalf("ListArtifactsResultHash: %v", err)
	}
	if wantHash != "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunArtifacts(context.Background(), second); err != nil {
		t.Fatalf("second RunArtifacts: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent artifact reads")
	}

	wantJSON, err := json.Marshal(factorysession.ListArtifactsResponseToAPI(listed))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared ListArtifactsResponseToAPI projection")
	}
}

func TestRunEvents_AsyncRunningFixtureHumanAndReconnectCursor(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var allOutput bytes.Buffer
	if err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID: "dur-sess-js-run-n-001",
		Output:    &allOutput,
		Service:   service,
	}); err != nil {
		t.Fatalf("RunEvents all: %v", err)
	}
	allText := allOutput.String()
	for _, want := range []string{
		"Factory session dur-sess-js-run-n-001 events (2):",
		"SESSION_STARTED session-started/dur-sess-js-run-n-001",
	} {
		if !strings.Contains(allText, want) {
			t.Fatalf("all events output missing %q:\n%s", want, allText)
		}
	}

	var afterOutput bytes.Buffer
	if err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID:    "dur-sess-js-run-n-001",
		AfterEventID: "session-started/dur-sess-js-run-n-001",
		Output:       &afterOutput,
		Service:      service,
	}); err != nil {
		t.Fatalf("RunEvents reconnect: %v", err)
	}
	afterText := afterOutput.String()
	if !strings.Contains(afterText, "events (1):") {
		t.Fatalf("reconnect output = %q, want one trailing event", afterText)
	}
	if strings.Contains(afterText, "SESSION_STARTED") {
		t.Fatalf("reconnect output should omit acknowledged event:\n%s", afterText)
	}
}

func TestRunEvents_AsyncRunningFixtureJSONMatchesFixtureHash(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	base := sessionexecution.EventsConfig{
		SessionID: "dur-sess-js-run-n-001",
		JSON:      true,
		Service:   service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunEvents(context.Background(), first); err != nil {
		t.Fatalf("first RunEvents: %v", err)
	}

	read, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	wantHash, err := fixtures.EventReadResultHash(read)
	if err != nil {
		t.Fatalf("EventReadResultHash: %v", err)
	}
	if wantHash != "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65" {
		t.Fatalf("fixture hash drifted to %q", wantHash)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunEvents(context.Background(), second); err != nil {
		t.Fatalf("second RunEvents: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent event reads")
	}

	wantJSON, err := json.Marshal(factorysession.EventReadResponseToAPI(read))
	if err != nil {
		t.Fatalf("marshal shared projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared EventReadResponseToAPI projection")
	}
}

func TestRunEvents_MissingReconnectCursorReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	seedAsyncRunningSession(t, service)

	var output bytes.Buffer
	err := sessionexecution.RunEvents(context.Background(), sessionexecution.EventsConfig{
		SessionID:    "dur-sess-js-run-n-001",
		AfterEventID: "missing-event-cursor",
		JSON:         true,
		Output:       &output,
		Service:      service,
	})
	if err == nil {
		t.Fatal("RunEvents = nil, want reconnect cursor error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeReconnectCursorNotFound) {
		t.Fatalf("output = %q, want reconnect cursor code", output.String())
	}
}

func TestRunDispatches_MissingSessionReturnsDeterministicError(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunDispatches(context.Background(), sessionexecution.DispatchesConfig{
		SessionID: "dur-sess-missing-001",
		JSON:      true,
		Output:    &output,
		Service:   service,
	})
	if err == nil {
		t.Fatal("RunDispatches = nil, want missing session error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeSessionNotFound) {
		t.Fatalf("output = %q, want session not found code", output.String())
	}
}
