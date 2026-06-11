package fixtures_test

import (
	"context"
	"errors"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestFakeService_PublishedScenarios_AsyncStartInspectionLinksAndEventPrefix(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)

	started, err := service.StartAsync(context.Background(), startRequestForPublished(row))
	if err != nil {
		t.Fatalf("fse.StartAsync: %v", err)
	}
	if started.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", started.SessionID, row.SessionID)
	}
	if started.Status != string(fse.LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}
	if started.Links.Session != "/factory-sessions/"+row.SessionID {
		t.Fatalf("session link = %q", started.Links.Session)
	}
	if started.Links.Results != "/factory-sessions/"+row.SessionID+"/results" {
		t.Fatalf("results link = %q", started.Links.Results)
	}

	events, err := service.ReadEvents(context.Background(), row.SessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("fse.ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("event prefix missing")
	}
	assertCanonicalEventEnvelope(t, events.Events[0], "SESSION_STARTED", "session-started/"+row.SessionID)
}

func TestFakeService_PublishedScenarios_ResultArtifactInclusion(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("fse.StartSync: %v", err)
	}

	excluded, err := service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{
		Mode:             fse.ResultModeFinal,
		IncludeArtifacts: false,
	})
	if err != nil {
		t.Fatalf("fse.GetResult excluded: %v", err)
	}
	if len(excluded.ArtifactIDs) != 1 || excluded.ArtifactIDs[0] != "art-petri-final-001" {
		t.Fatalf("artifactIds = %#v", excluded.ArtifactIDs)
	}
	if len(excluded.ArtifactRefs) != 0 {
		t.Fatalf("artifactRefs = %#v, want omitted", excluded.ArtifactRefs)
	}

	included, err := service.GetResult(context.Background(), row.SessionID, fse.ResultRequest{
		Mode:             fse.ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("fse.GetResult included: %v", err)
	}
	if len(included.ArtifactRefs) != 1 || included.ArtifactRefs[0].ID != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", included.ArtifactRefs)
	}
	if len(included.ArtifactIDs) != 0 {
		t.Fatalf("artifactIds = %#v, want omitted when refs included", included.ArtifactIDs)
	}
}

func TestFakeService_PublishedScenarios_ListDispatchesStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  fixtures.FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  fixtures.FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a",
			wantIDs:  []string{"disp-petri-success-001"},
		},
		{
			purpose:  fixtures.FixturePurposeAsyncRunning,
			wantHash: "sha256:51df934ba2b35b5baa20c4b64b1907cf66f109ffbffe2d3e9eedac747b07ded9",
			wantIDs:  []string{"disp-js-001", "disp-js-002", "disp-js-003"},
		},
		{
			purpose:  fixtures.FixturePurposeArtifactInspection,
			wantHash: "sha256:9387a745d2699e5b22d92b2152183aecf3a8db85966630de7b0899a3f19e504c",
			wantIDs:  []string{"disp-js-pause-001", "disp-js-pause-002"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			startPublishedScenarioWithSync(t, service, row, tc.sync)
			assertDispatchListStableSummaries(t, service, row, tc.wantIDs, tc.wantHash)
		})
	}
}

func TestFakeService_PublishedScenarios_GetDispatchDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	dispatchRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(dispatchRow)); err != nil {
		t.Fatalf("fse.StartSync dispatch scenario: %v", err)
	}

	detail, err := service.GetDispatch(context.Background(), dispatchRow.SessionID, "disp-petri-success-001")
	if err != nil {
		t.Fatalf("fse.GetDispatch: %v", err)
	}
	if detail.SessionID != dispatchRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", detail.SessionID, dispatchRow.SessionID)
	}
	if detail.OrchestratorKind != "PETRI" {
		t.Fatalf("orchestratorKind = %q, want PETRI", detail.OrchestratorKind)
	}
	if detail.Petri == nil || detail.Petri.TransitionID != "transition-plan-task" {
		t.Fatalf("petri projection = %#v", detail.Petri)
	}
	hash, err := fixtures.DispatchDetailHash(detail)
	if err != nil {
		t.Fatalf("fixtures.DispatchDetailHash: %v", err)
	}
	if hash != "sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f" {
		t.Fatalf("dispatch detail hash = %q, want sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f", hash)
	}

	artifactRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("fse.StartAsync artifact scenario: %v", err)
	}
	jsDetail, err := service.GetDispatch(context.Background(), artifactRow.SessionID, "disp-js-pause-002")
	if err != nil {
		t.Fatalf("fse.GetDispatch javascript detail: %v", err)
	}
	if jsDetail.JavaScript == nil || jsDetail.JavaScript.TaskKind != "SYSTEM" {
		t.Fatalf("javascript projection = %#v", jsDetail.JavaScript)
	}

	_, err = service.GetDispatch(context.Background(), dispatchRow.SessionID, "missing-dispatch-id")
	if !errors.Is(err, fse.ErrDispatchNotFound) {
		t.Fatalf("unknown dispatch error = %v, want fse.ErrDispatchNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ListArtifactsStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  fixtures.FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  fixtures.FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce",
			wantIDs:  []string{"art-petri-final-001"},
		},
		{
			purpose:  fixtures.FixturePurposeArtifactInspection,
			wantHash: "sha256:57fa7af131ce29cb2a254d2548ef8b8f9b0ccf6de7fb6cc185beabf8190f1dcb",
			wantIDs:  []string{"art-js-pause-001"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			startPublishedScenarioWithSync(t, service, row, tc.sync)
			assertArtifactListStableSummaries(t, service, row, tc.wantIDs, tc.wantHash)
		})
	}
}

func TestFakeService_PublishedScenarios_GetArtifactDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("fse.StartAsync: %v", err)
	}

	detail, err := service.GetArtifact(context.Background(), row.SessionID, "art-js-pause-001")
	if err != nil {
		t.Fatalf("fse.GetArtifact: %v", err)
	}
	if detail.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", detail.SessionID, row.SessionID)
	}
	if detail.DispatchID != "disp-js-pause-001" {
		t.Fatalf("dispatchId = %q, want disp-js-pause-001", detail.DispatchID)
	}
	if detail.ContentRef == nil || detail.ContentRef.Href == "" {
		t.Fatalf("contentRef missing: %#v", detail.ContentRef)
	}
	hash, err := fixtures.ArtifactDetailHash(detail)
	if err != nil {
		t.Fatalf("fixtures.ArtifactDetailHash: %v", err)
	}
	if hash != "sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20" {
		t.Fatalf("artifact detail hash = %q, want sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20", hash)
	}

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("fse.StartSync success: %v", err)
	}
	successDetail, err := service.GetArtifact(context.Background(), successRow.SessionID, "art-petri-final-001")
	if err != nil {
		t.Fatalf("fse.GetArtifact terminal: %v", err)
	}
	if len(successDetail.Content) == 0 {
		t.Fatal("terminal artifact content missing")
	}

	_, err = service.GetArtifact(context.Background(), row.SessionID, "missing-artifact-id")
	if !errors.Is(err, fse.ErrArtifactNotFound) {
		t.Fatalf("unknown artifact error = %v, want fse.ErrArtifactNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ReadEventsCanonicalAndReconnect(t *testing.T) {
	service := newContractFakeService(t)
	for _, tc := range canonicalEventReconnectCases() {
		t.Run(tc.name, func(t *testing.T) {
			runCanonicalEventReconnectCase(t, service, tc)
		})
	}
}

func TestFakeService_PublishedScenarios_ReadEventsMissingCursorReturnsTypedError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("fse.StartAsync: %v", err)
	}
	_, err := service.ReadEvents(context.Background(), row.SessionID, fse.EventReconnectRequest{
		AfterEventID: "missing-event-cursor",
	})
	if !errors.Is(err, fse.ErrReconnectCursorNotFound) {
		t.Fatalf("error = %v, want fse.ErrReconnectCursorNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs(t *testing.T) {
	service := newContractFakeService(t)
	if _, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-petri-run-001",
		Source: fse.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}); err != nil {
		t.Fatalf("fse.StartAsync: %v", err)
	}
	listed, err := service.ListDispatches(context.Background(), "dur-sess-petri-run-001")
	if err != nil {
		t.Fatalf("fse.ListDispatches: %v", err)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", listed.Dispatches)
	}
	refs := listed.Dispatches[0].ProviderSessionRefs
	if len(refs) != 1 || refs[0].ID != "prov-sess-disp-petri-001" {
		t.Fatalf("providerSessionRefs = %#v", refs)
	}
}
