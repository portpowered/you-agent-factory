package factorysessionexecution

import (
	"context"
	"errors"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestFakeService_PublishedScenarios_ListDispatchesStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:a32d5d0f136dcfef8061746c8f270702163c92a04e3c9f75eb9248e19bebd34a",
			wantIDs:  []string{"disp-petri-success-001"},
		},
		{
			purpose:  FixturePurposeAsyncRunning,
			wantHash: "sha256:51df934ba2b35b5baa20c4b64b1907cf66f109ffbffe2d3e9eedac747b07ded9",
			wantIDs:  []string{"disp-js-001", "disp-js-002", "disp-js-003"},
		},
		{
			purpose:  FixturePurposeArtifactInspection,
			wantHash: "sha256:9387a745d2699e5b22d92b2152183aecf3a8db85966630de7b0899a3f19e504c",
			wantIDs:  []string{"disp-js-pause-001", "disp-js-pause-002"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			listed, err := service.ListDispatches(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("ListDispatches: %v", err)
			}
			if listed.SessionID != row.SessionID {
				t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
			}
			if len(listed.Dispatches) != len(tc.wantIDs) {
				t.Fatalf("dispatches = %#v, want %d rows", listed.Dispatches, len(tc.wantIDs))
			}
			for index, wantID := range tc.wantIDs {
				got := listed.Dispatches[index]
				if got.ID != wantID {
					t.Fatalf("dispatch[%d].id = %q, want %q", index, got.ID, wantID)
				}
				if got.Status == "" || got.DispatchKind == "" {
					t.Fatalf("dispatch[%d] missing status/kind: %#v", index, got)
				}
			}

			read, err := service.GetSession(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if err := ValidateDispatchListMatchesSessionProgress(read, listed.Dispatches); err != nil {
				t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
			}

			hash, err := ListDispatchesResultHash(listed)
			if err != nil {
				t.Fatalf("ListDispatchesResultHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("dispatch list hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_GetDispatchDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	dispatchRow := publishedScenarioByPurpose(t, FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(dispatchRow)); err != nil {
		t.Fatalf("StartSync dispatch scenario: %v", err)
	}

	detail, err := service.GetDispatch(context.Background(), dispatchRow.SessionID, "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
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
	hash, err := DispatchDetailHash(detail)
	if err != nil {
		t.Fatalf("DispatchDetailHash: %v", err)
	}
	if hash != "sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f" {
		t.Fatalf("dispatch detail hash = %q, want sha256:0309e245dc0354d3d0083b0d3a083fe9862ac01415d243914098a49c819cf37f", hash)
	}

	artifactRow := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("StartAsync artifact scenario: %v", err)
	}
	jsDetail, err := service.GetDispatch(context.Background(), artifactRow.SessionID, "disp-js-pause-002")
	if err != nil {
		t.Fatalf("GetDispatch javascript detail: %v", err)
	}
	if jsDetail.JavaScript == nil || jsDetail.JavaScript.TaskKind != "SYSTEM" {
		t.Fatalf("javascript projection = %#v", jsDetail.JavaScript)
	}

	_, err = service.GetDispatch(context.Background(), dispatchRow.SessionID, "missing-dispatch-id")
	if !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("unknown dispatch error = %v, want ErrDispatchNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ListArtifactsStableSummaries(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		purpose  FixtureScenarioPurpose
		sync     bool
		wantHash string
		wantIDs  []string
	}{
		{
			purpose:  FixturePurposeDispatchInspection,
			sync:     true,
			wantHash: "sha256:c42d891189b507df18e127e6cf10deeacf3d56a97c48786491d0ddfd3ed65fce",
			wantIDs:  []string{"art-petri-final-001"},
		},
		{
			purpose:  FixturePurposeArtifactInspection,
			wantHash: "sha256:57fa7af131ce29cb2a254d2548ef8b8f9b0ccf6de7fb6cc185beabf8190f1dcb",
			wantIDs:  []string{"art-js-pause-001"},
		},
	}
	for _, tc := range cases {
		row := publishedScenarioByPurpose(t, tc.purpose)
		t.Run(string(tc.purpose), func(t *testing.T) {
			req := startRequestForPublished(row)
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			listed, err := service.ListArtifacts(context.Background(), row.SessionID)
			if err != nil {
				t.Fatalf("ListArtifacts: %v", err)
			}
			if listed.SessionID != row.SessionID {
				t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
			}
			if len(listed.Artifacts) != len(tc.wantIDs) {
				t.Fatalf("artifacts = %#v, want %d rows", listed.Artifacts, len(tc.wantIDs))
			}
			for index, wantID := range tc.wantIDs {
				got := listed.Artifacts[index]
				if got.ID != wantID {
					t.Fatalf("artifact[%d].id = %q, want %q", index, got.ID, wantID)
				}
				if got.Kind == "" || got.ContentHash == "" {
					t.Fatalf("artifact[%d] missing kind/contentHash: %#v", index, got)
				}
				if got.RetrievalRef == nil || got.RetrievalRef.Href == "" {
					t.Fatalf("artifact[%d] missing retrieval ref: %#v", index, got)
				}
				wantHref := "/factory-sessions/" + row.SessionID + "/artifacts/" + wantID
				if got.RetrievalRef.Href != wantHref {
					t.Fatalf("retrieval href = %q, want %q", got.RetrievalRef.Href, wantHref)
				}
			}

			hash, err := ListArtifactsResultHash(listed)
			if err != nil {
				t.Fatalf("ListArtifactsResultHash: %v", err)
			}
			if hash != tc.wantHash {
				t.Fatalf("artifact list hash = %q, want %q", hash, tc.wantHash)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_GetArtifactDetailAndUnknownError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	detail, err := service.GetArtifact(context.Background(), row.SessionID, "art-js-pause-001")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
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
	hash, err := ArtifactDetailHash(detail)
	if err != nil {
		t.Fatalf("ArtifactDetailHash: %v", err)
	}
	if hash != "sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20" {
		t.Fatalf("artifact detail hash = %q, want sha256:0b4d4d6d8483cb9ad7f867019145f069752b2663890689775bbd20325716cf20", hash)
	}

	successRow := publishedScenarioByPurpose(t, FixturePurposeDispatchInspection)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	successDetail, err := service.GetArtifact(context.Background(), successRow.SessionID, "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact terminal: %v", err)
	}
	if len(successDetail.Content) == 0 {
		t.Fatal("terminal artifact content missing")
	}

	_, err = service.GetArtifact(context.Background(), row.SessionID, "missing-artifact-id")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("unknown artifact error = %v, want ErrArtifactNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_ReadEventsCanonicalAndReconnect(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		name           string
		requestID      string
		sessionID      string
		sync           bool
		wantCount      int
		wantHash       string
		reconnectAfter string
		wantAfterCount int
	}{
		{
			name:           "running",
			requestID:      "req-js-run-n-001",
			sessionID:      "dur-sess-js-run-n-001",
			wantCount:      2,
			wantHash:       "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65",
			reconnectAfter: "session-started/dur-sess-js-run-n-001",
			wantAfterCount: 1,
		},
		{
			name:      "terminal",
			requestID: "req-js-success-002",
			sessionID: "dur-sess-js-success-002",
			wantCount: 3,
			wantHash:  "sha256:956aeb10de9e9e3a8e5ced44d32e1a15c41d770359259ad148d446611e6fce5c",
		},
		{
			name:      "dispatch-inspection",
			requestID: "req-petri-success-001",
			sessionID: "dur-sess-petri-success-001",
			sync:      true,
			wantCount: 3,
			wantHash:  "sha256:9dbb55ddc666ebae19e02b67b3eab9e0e1916241a08341949dec6d5f11f49348",
		},
		{
			name:      "artifact-inspection",
			requestID: "req-js-paused-001",
			sessionID: "dur-sess-js-paused-001",
			wantCount: 2,
			wantHash:  "sha256:4fc92b6cff30745dfe1112fcbbf1bb70fc1f132bdfec25b5b0e39128ac6f054c",
		},
		{
			name:      "awaiting-approval",
			requestID: "req-js-awaiting-001",
			sessionID: "dur-sess-js-awaiting-001",
			wantCount: 2,
			wantHash:  "sha256:330aaa8847dbd0ef3e40b573fbda9354fbd38b075dfb7402360d82fd617f4a40",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := StartRequest{
				RequestID: tc.requestID,
				Source: Source{
					Kind:      workflowsource.KindFactoryID,
					FactoryID: "customer-support-triage",
				},
			}
			if tc.requestID == "req-js-success-002" {
				req.Source = Source{
					Kind:         workflowsource.KindWorkflowFile,
					WorkflowFile: ".claude/workflows/docs-refresh.yaml",
				}
			}
			if tc.requestID == "req-js-awaiting-001" {
				req.Source = Source{
					Kind:         workflowsource.KindWorkflowFile,
					WorkflowFile: ".claude/workflows/policy-gated-release.yaml",
				}
			}
			if tc.sync {
				if _, err := service.StartSync(context.Background(), req); err != nil {
					t.Fatalf("StartSync: %v", err)
				}
			} else if _, err := service.StartAsync(context.Background(), req); err != nil {
				t.Fatalf("StartAsync: %v", err)
			}

			all, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			if len(all.Events) != tc.wantCount {
				t.Fatalf("events = %d, want %d", len(all.Events), tc.wantCount)
			}
			for _, raw := range all.Events {
				assertCanonicalEventEnvelope(t, raw, "", "")
			}
			if tc.wantHash != "" {
				hash, err := EventReadResultHash(all)
				if err != nil {
					t.Fatalf("EventReadResultHash: %v", err)
				}
				if hash != tc.wantHash {
					t.Fatalf("event hash = %q, want %q", hash, tc.wantHash)
				}
			}

			if tc.reconnectAfter == "" {
				return
			}
			after, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{
				AfterEventID: tc.reconnectAfter,
			})
			if err != nil {
				t.Fatalf("ReadEvents reconnect: %v", err)
			}
			if len(after.Events) != tc.wantAfterCount {
				t.Fatalf("reconnect events = %d, want %d", len(after.Events), tc.wantAfterCount)
			}
		})
	}
}

func TestFakeService_PublishedScenarios_ReadEventsMissingCursorReturnsTypedError(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(row)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	_, err := service.ReadEvents(context.Background(), row.SessionID, EventReconnectRequest{
		AfterEventID: "missing-event-cursor",
	})
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestFakeService_PublishedScenarios_DispatchListIncludesProviderSessionRefs(t *testing.T) {
	service := newContractFakeService(t)
	if _, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	listed, err := service.ListDispatches(context.Background(), "dur-sess-petri-run-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v", listed.Dispatches)
	}
	refs := listed.Dispatches[0].ProviderSessionRefs
	if len(refs) != 1 || refs[0].ID != "prov-sess-disp-petri-001" {
		t.Fatalf("providerSessionRefs = %#v", refs)
	}
}
