package sessionexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
)

func TestRunSync_SuccessFixtureHumanOutput(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &output,
		Service: service,
	})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	text := output.String()
	for _, want := range []string{
		"Factory session dur-sess-petri-success-001 completed (SUCCEEDED).",
		"Source hash: sha256:petri-factory-001",
		"Primary result: Ticket triaged and resolved.",
		"Session link: /factory-sessions/dur-sess-petri-success-001",
		"Results link: /factory-sessions/dur-sess-petri-success-001/results",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunSync_SuccessFixtureJSONOutputIsDeterministic(t *testing.T) {
	service := newContractFakeService(t)
	want := expectedSyncSuccessAPIResponse(t)

	var firstOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &firstOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	var gotResponse factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(firstOutput.Bytes()), &gotResponse); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	assertSyncSuccessAPIResponse(t, gotResponse, want)

	var secondOutput bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &secondOutput,
		Service: service,
	}); err != nil {
		t.Fatalf("second RunSync: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("json output is not deterministic across equivalent runs")
	}
}

func TestRunSync_ReplaysSameRequestIDWithoutNewSession(t *testing.T) {
	service := newContractFakeService(t)
	base := sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Service: service,
	}

	var firstOutput bytes.Buffer
	first := base
	first.Output = &firstOutput
	if err := sessionexecution.RunSync(context.Background(), first); err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	var secondOutput bytes.Buffer
	second := base
	second.Output = &secondOutput
	if err := sessionexecution.RunSync(context.Background(), second); err != nil {
		t.Fatalf("replay RunSync: %v", err)
	}

	if !bytes.Equal(bytes.TrimSpace(firstOutput.Bytes()), bytes.TrimSpace(secondOutput.Bytes())) {
		t.Fatalf("replay output changed:\nfirst: %s\nsecond: %s", firstOutput.Bytes(), secondOutput.Bytes())
	}
}

func TestRunSync_RejectsAsyncMode(t *testing.T) {
	service := newContractFakeService(t)
	var output bytes.Buffer
	err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeAsync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		Output:  &output,
		Service: service,
	})
	if err == nil {
		t.Fatal("RunSync = nil, want unsupported mode error")
	}
	if !strings.Contains(output.String(), sessionexecution.ErrorCodeUnsupportedMode) {
		t.Fatalf("output = %q, want unsupported mode code", output.String())
	}
}

func assertSyncSuccessAPIResponse(
	t *testing.T,
	got, want factoryapi.FactorySessionSyncExecutionResponse,
) {
	t.Helper()
	if got.SessionId != want.SessionId {
		t.Fatalf("sessionId = %q, want %q", got.SessionId, want.SessionId)
	}
	if got.Status != want.Status {
		t.Fatalf("status = %q, want %q", got.Status, want.Status)
	}
	if got.SyncOutcome != want.SyncOutcome {
		t.Fatalf("syncOutcome = %q, want %q", got.SyncOutcome, want.SyncOutcome)
	}
	if got.SourceHash == nil || want.SourceHash == nil || *got.SourceHash != *want.SourceHash {
		t.Fatalf("sourceHash = %#v, want %#v", got.SourceHash, want.SourceHash)
	}
	if got.Result == nil || want.Result == nil {
		t.Fatalf("result = %#v, want %#v", got.Result, want.Result)
	}
	if got.Result.ResultStatus != want.Result.ResultStatus {
		t.Fatalf("resultStatus = %q, want %q", got.Result.ResultStatus, want.Result.ResultStatus)
	}
	if got.Links == nil || want.Links == nil {
		t.Fatalf("links = %#v, want %#v", got.Links, want.Links)
	}
	if got.Links.Session == nil || want.Links.Session == nil || *got.Links.Session != *want.Links.Session {
		t.Fatalf("session link = %#v, want %#v", got.Links.Session, want.Links.Session)
	}
	if got.Links.Results == nil || want.Links.Results == nil || *got.Links.Results != *want.Links.Results {
		t.Fatalf("results link = %#v, want %#v", got.Links.Results, want.Links.Results)
	}
	if primaryResultText(got.Result) != primaryResultText(want.Result) {
		t.Fatalf("primaryResult text = %q, want %q", primaryResultText(got.Result), primaryResultText(want.Result))
	}
}

func primaryResultText(result *factoryapi.FactorySessionResult) string {
	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	for _, part := range *result.PrimaryResult {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(textPart.Text); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func expectedSyncSuccessAPIResponse(t *testing.T) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	raw, err := os.ReadFile(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("read fixture catalog: %v", err)
	}
	var catalog struct {
		Scenarios []struct {
			ID           string          `json:"id"`
			SyncResponse json.RawMessage `json:"syncResponse"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode fixture catalog: %v", err)
	}
	for _, scenario := range catalog.Scenarios {
		if scenario.ID != fixtures.FixtureScenarioSyncSuccess {
			continue
		}
		var response factoryapi.FactorySessionSyncExecutionResponse
		if err := json.Unmarshal(scenario.SyncResponse, &response); err != nil {
			t.Fatalf("decode sync response fixture: %v", err)
		}
		return response
	}
	t.Fatalf("missing sync response fixture for %q", fixtures.FixtureScenarioSyncSuccess)
	return factoryapi.FactorySessionSyncExecutionResponse{}
}

func newContractFakeService(t *testing.T) fse.Service {
	t.Helper()
	service, err := fse.NewFakeServiceFromContractFixtures(contractFixtureCatalogPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
}
func TestRunSync_UsesSharedServicePrimaryResultProjection(t *testing.T) {
	service := newContractFakeService(t)
	normalized, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeSync,
		RequestID: "req-petri-success-001",
		FactoryID: "customer-support-triage",
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}

	var output bytes.Buffer
	if err := sessionexecution.RunSync(context.Background(), sessionexecution.RunConfig{
		StartConfig: sessionexecution.StartConfig{
			Mode:      sessionexecution.ExecutionModeSync,
			RequestID: "req-petri-success-001",
			FactoryID: "customer-support-triage",
		},
		JSON:    true,
		Output:  &output,
		Service: service,
	}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	var mapped factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &mapped); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if mapped.Result == nil || mapped.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL primary result from shared sync start projection", mapped.Result)
	}

	direct, err := service.StartSync(context.Background(), normalized)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	want := factorysession.SyncStartResponseToAPI(direct)
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal direct projection: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(output.Bytes()), wantJSON) {
		t.Fatalf("CLI output diverged from shared SyncStartResponseToAPI projection")
	}
}
