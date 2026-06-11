package factorysessionexecution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func liveSessionCount(t *testing.T, service *FakeService) int {
	t.Helper()
	result, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	return len(result.LiveSessions)
}

func assertTypedFailureHash(t *testing.T, err error, wantHash string) TypedFailureIdentity {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed failure")
	}
	identity, ok := TypedFailureIdentityFromError(err)
	if !ok {
		t.Fatalf("error = %v, want mappable typed failure identity", err)
	}
	hash, err := TypedFailureHash(identity)
	if err != nil {
		t.Fatalf("TypedFailureHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("typed failure hash = %q, want %q (identity=%#v)", hash, wantHash, identity)
	}
	return identity
}

func TestFakeService_PublishedTypedFailures_StableErrorIdentities(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("seed running session: %v", err)
	}
	successRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("seed terminal session: %v", err)
	}
	reconnectRow := publishedScenarioByPurpose(t, FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(reconnectRow)); err != nil {
		t.Fatalf("seed reconnect session: %v", err)
	}
	artifactRow := publishedScenarioByPurpose(t, FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("seed artifact session: %v", err)
	}

	cases := []struct {
		name     string
		run      func() error
		wantHash string
		assert   func(t *testing.T, identity TypedFailureIdentity)
	}{
		{
			name: "unknown scenario",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-unknown-scenario-999",
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:3ca4d4c3c59dd387d3192c61359f957311940532afde4b54f6567e9324f60025",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureUnknownScenario || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want UNKNOWN_SCENARIO requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing requestId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:5fd53056c4c9ebd6139c42b2f9ac8c41369e13dc9fa33c2bf2b945f3ddd64a66",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing factoryId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-malformed-factory-001",
					Source:    Source{Kind: workflowsource.KindFactoryID},
				})
				return err
			},
			wantHash: "sha256:40e7ece145ff99044ce9136eba49059e6631831169655f3fe640ab937a1a7a4c",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "source.factoryId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST source.factoryId", identity)
				}
			},
		},
		{
			name: "unknown session",
			run: func() error {
				_, err := service.GetSession(context.Background(), "dur-sess-missing-999")
				return err
			},
			wantHash: "sha256:4e8710020fa29e5e1a71572d34d95b31eac0585c58b0b969375722e2080df427",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureSessionNotFound {
					t.Fatalf("identity = %#v, want SESSION_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "unknown dispatch",
			run: func() error {
				_, err := service.GetDispatch(context.Background(), successRow.SessionID, "disp-missing-999")
				return err
			},
			wantHash: "sha256:be4a698e8381fb189f8835458d6010cdcb2bd0d12340ab3f14d4d738722291c9",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureDispatchNotFound {
					t.Fatalf("identity = %#v, want DISPATCH_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "unknown artifact",
			run: func() error {
				_, err := service.GetArtifact(context.Background(), artifactRow.SessionID, "art-missing-999")
				return err
			},
			wantHash: "sha256:60eb8812cd1420353e45889092cd8621f08975dc06fda7b82e2fb1f0e6878af6",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureArtifactNotFound {
					t.Fatalf("identity = %#v, want ARTIFACT_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "reconnect cursor miss",
			run: func() error {
				_, err := service.ReadEvents(context.Background(), reconnectRow.SessionID, EventReconnectRequest{
					AfterEventID: "missing-event-id",
				})
				return err
			},
			wantHash: "sha256:825721e8c0269ef7775e6a498f94cf303a7e7f8eb34605443a27e7d47b89003f",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureReconnectCursorNotFound {
					t.Fatalf("identity = %#v, want RECONNECT_CURSOR_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "execution request id conflict",
			run: func() error {
				req := startRequestForPublished(PublishedFixtureScenario{
					RequestID: FixtureScenarioIdempotentReplay,
				})
				req.RequestID = "req-idempotent-replay-001"
				if _, err := service.StartAsync(context.Background(), req); err != nil {
					return err
				}
				conflict := req
				conflict.Args = map[string]any{"task": "different"}
				_, err := service.StartAsync(context.Background(), conflict)
				return err
			},
			wantHash: "sha256:4f23c1535bd281b8c86838d72e23aa678de499ff6f7cec2c74e6e86327f1355d",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureExecutionRequestConflict {
					t.Fatalf("identity = %#v, want EXECUTION_REQUEST_ID_CONFLICT", identity)
				}
			},
		},
		{
			name: "lifecycle control conflict",
			run: func() error {
				if _, err := service.Pause(context.Background(), runningRow.SessionID, ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				}); err != nil {
					return err
				}
				_, err := service.Resume(context.Background(), runningRow.SessionID, ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				})
				return err
			},
			wantHash: "sha256:1d628d2ea99e52f916a3c74b240c368ac3d496eed822cfd6bd65f6a32d4e1941",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleConflict ||
					identity.Outcome != LifecycleControlOutcomeConflict ||
					identity.Operation != LifecycleControlResume {
					t.Fatalf("identity = %#v, want RESUME CONFLICT", identity)
				}
				if identity.Status != LifecycleStatusPaused {
					t.Fatalf("status = %q, want PAUSED", identity.Status)
				}
			},
		},
		{
			name: "lifecycle invalid state",
			run: func() error {
				if _, err := service.StartAsync(context.Background(), StartRequest{
					RequestID: "req-js-awaiting-001",
					Source: Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				}); err != nil {
					return err
				}
				_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", ControlRequest{})
				return err
			},
			wantHash: "sha256:6c511c017a9ef0d179fe25f803795b3aad27dd469c6f186e80c69c68f7e6b987",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleInvalidState ||
					identity.Operation != LifecycleControlPause {
					t.Fatalf("identity = %#v, want PAUSE INVALID_STATE", identity)
				}
			},
		},
		{
			name: "lifecycle terminal session",
			run: func() error {
				_, err := service.Cancel(context.Background(), successRow.SessionID, ControlRequest{})
				return err
			},
			wantHash: "sha256:5521c0202e46e84f30a205891e529b616710d755cefe2f95b37912e45550283d",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureLifecycleTerminalSession ||
					identity.Operation != LifecycleControlCancel {
					t.Fatalf("identity = %#v, want CANCEL TERMINAL_SESSION", identity)
				}
			},
		},
		{
			name: "malformed control missing dispatchId",
			run: func() error {
				_, err := service.RetryDispatch(context.Background(), runningRow.SessionID, RetryDispatchRequest{})
				return err
			},
			wantHash: "sha256:f4ea5eb8291cb4fa851df8c7eeaacffa219dea2140e9abe6d5c7239f570a60e0",
			assert: func(t *testing.T, identity TypedFailureIdentity) {
				if identity.Kind != TypedFailureMalformedRequest || identity.Field != "dispatchId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST dispatchId", identity)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			identity := assertTypedFailureHash(t, err, tc.wantHash)
			tc.assert(t, identity)
		})
	}
}

func TestFakeService_MalformedRequests_DoNotMutateFixtureState(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	_, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("StartAsync empty requestId error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed start = %d, want %d", after, before)
	}

	_, err = service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-unknown-scenario-typed-001",
		Source: Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("unknown scenario error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown scenario = %d, want %d", after, before)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-missing-typed-001", RetryDispatchRequest{})
	if !errors.As(err, &validationErr) || validationErr.Field != "dispatchId" {
		t.Fatalf("malformed retry error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed control = %d, want %d", after, before)
	}

	_, err = service.GetSession(context.Background(), "dur-sess-missing-typed-001")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown session read = %d, want %d", after, before)
	}
}

func TestFactorySessionExecutionTests_AvoidForbiddenWorkflowRunVocabulary(t *testing.T) {
	root := "."
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(raw)
		for _, term := range ForbiddenFixtureVocabularyTerms() {
			if strings.Contains(text, term) {
				t.Fatalf("%s contains forbidden term %q", entry.Name(), term)
			}
		}
	}
}
