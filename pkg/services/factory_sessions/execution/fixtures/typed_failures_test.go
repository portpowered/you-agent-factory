package fixtures_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/fixtures"
)

func TestFakeService_PublishedTypedFailures_StartAndReadErrors(t *testing.T) {
	service, _, successRow, reconnectRow, artifactRow := seedTypedFailureService(t)
	runTypedFailureCases(t, startReadTypedFailureCases(service, successRow, reconnectRow, artifactRow))
}

func TestFakeService_PublishedTypedFailures_LifecycleErrors(t *testing.T) {
	service, runningRow, successRow, _, _ := seedTypedFailureService(t)
	runTypedFailureCases(t, lifecycleTypedFailureCases(service, runningRow, successRow))
}

func TestFakeService_MalformedRequests_DoNotMutateFixtureState(t *testing.T) {
	service := newContractFakeService(t)
	before := liveSessionCount(t, service)

	_, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "",
		Source: fse.Source{
			Kind:      factory.WorkflowSourceKindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	var validationErr *fse.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("StartAsync empty requestId error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed start = %d, want %d", after, before)
	}

	_, err = service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-unknown-scenario-typed-001",
		Source: fse.Source{
			Kind:      factory.WorkflowSourceKindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("unknown scenario error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown scenario = %d, want %d", after, before)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-missing-typed-001", fse.RetryDispatchRequest{})
	if !errors.As(err, &validationErr) || validationErr.Field != "dispatchId" {
		t.Fatalf("malformed retry error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after malformed control = %d, want %d", after, before)
	}

	_, err = service.GetSession(context.Background(), "dur-sess-missing-typed-001")
	if !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
	if after := liveSessionCount(t, service); after != before {
		t.Fatalf("live sessions after unknown session read = %d, want %d", after, before)
	}
}

type typedFailureCase struct {
	name     string
	run      func() error
	wantHash string
	assert   func(t *testing.T, identity fixtures.TypedFailureIdentity)
}

func seedTypedFailureService(t *testing.T) (*fse.FakeService, fixtures.PublishedFixtureScenario, fixtures.PublishedFixtureScenario, fixtures.PublishedFixtureScenario, fixtures.PublishedFixtureScenario) {
	t.Helper()
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("seed running session: %v", err)
	}
	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	if _, err := service.StartSync(context.Background(), startRequestForPublished(successRow)); err != nil {
		t.Fatalf("seed terminal session: %v", err)
	}
	reconnectRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeEventReconnect)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(reconnectRow)); err != nil {
		t.Fatalf("seed reconnect session: %v", err)
	}
	artifactRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeArtifactInspection)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(artifactRow)); err != nil {
		t.Fatalf("seed artifact session: %v", err)
	}
	return service, runningRow, successRow, reconnectRow, artifactRow
}

func startReadTypedFailureCases(service *fse.FakeService, successRow, reconnectRow, artifactRow fixtures.PublishedFixtureScenario) []typedFailureCase {
	cases := startReadTypedFailureCasesHead(service, successRow)
	cases = append(cases, startReadTypedFailureCasesTail(service, reconnectRow, artifactRow)...)
	cases = append(cases, startReadTypedFailureExecutionConflictCase(service))
	return cases
}

func startReadTypedFailureCasesHead(service *fse.FakeService, successRow fixtures.PublishedFixtureScenario) []typedFailureCase {
	return []typedFailureCase{
		{
			name: "unknown scenario",
			run: func() error {
				_, err := service.StartAsync(context.Background(), fse.StartRequest{
					RequestID: "req-unknown-scenario-999",
					Source: fse.Source{
						Kind:      factory.WorkflowSourceKindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:3ca4d4c3c59dd387d3192c61359f957311940532afde4b54f6567e9324f60025",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureUnknownScenario || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want UNKNOWN_SCENARIO requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing requestId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), fse.StartRequest{
					Source: fse.Source{
						Kind:      factory.WorkflowSourceKindFactoryID,
						FactoryID: "customer-support-triage",
					},
				})
				return err
			},
			wantHash: "sha256:5fd53056c4c9ebd6139c42b2f9ac8c41369e13dc9fa33c2bf2b945f3ddd64a66",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureMalformedRequest || identity.Field != "requestId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST requestId", identity)
				}
			},
		},
		{
			name: "malformed start missing factoryId",
			run: func() error {
				_, err := service.StartAsync(context.Background(), fse.StartRequest{
					RequestID: "req-malformed-factory-001",
					Source:    fse.Source{Kind: factory.WorkflowSourceKindFactoryID},
				})
				return err
			},
			wantHash: "sha256:40e7ece145ff99044ce9136eba49059e6631831169655f3fe640ab937a1a7a4c",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureMalformedRequest || identity.Field != "source.factoryId" {
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
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureSessionNotFound {
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
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureDispatchNotFound {
					t.Fatalf("identity = %#v, want DISPATCH_NOT_FOUND", identity)
				}
			},
		},
	}
}

func startReadTypedFailureCasesTail(service *fse.FakeService, reconnectRow, artifactRow fixtures.PublishedFixtureScenario) []typedFailureCase {
	return []typedFailureCase{
		{
			name: "unknown artifact",
			run: func() error {
				_, err := service.GetArtifact(context.Background(), artifactRow.SessionID, "art-missing-999")
				return err
			},
			wantHash: "sha256:60eb8812cd1420353e45889092cd8621f08975dc06fda7b82e2fb1f0e6878af6",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureArtifactNotFound {
					t.Fatalf("identity = %#v, want ARTIFACT_NOT_FOUND", identity)
				}
			},
		},
		{
			name: "reconnect cursor miss",
			run: func() error {
				_, err := service.ReadEvents(context.Background(), reconnectRow.SessionID, fse.EventReconnectRequest{
					AfterEventID: "missing-event-id",
				})
				return err
			},
			wantHash: "sha256:825721e8c0269ef7775e6a498f94cf303a7e7f8eb34605443a27e7d47b89003f",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureReconnectCursorNotFound {
					t.Fatalf("identity = %#v, want RECONNECT_CURSOR_NOT_FOUND", identity)
				}
			},
		},
	}
}

func startReadTypedFailureExecutionConflictCase(service *fse.FakeService) typedFailureCase {
	return typedFailureCase{
		name: "execution request id conflict",
		run: func() error {
			req := startRequestForPublished(fixtures.PublishedFixtureScenario{
				RequestID: fixtures.FixtureScenarioIdempotentReplay,
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
		assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
			if identity.Kind != fixtures.TypedFailureExecutionRequestConflict {
				t.Fatalf("identity = %#v, want EXECUTION_REQUEST_ID_CONFLICT", identity)
			}
		},
	}
}

func lifecycleTypedFailureCases(service *fse.FakeService, runningRow, successRow fixtures.PublishedFixtureScenario) []typedFailureCase {
	return []typedFailureCase{
		{
			name: "lifecycle control conflict",
			run: func() error {
				if _, err := service.Pause(context.Background(), runningRow.SessionID, fse.ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				}); err != nil {
					return err
				}
				_, err := service.Resume(context.Background(), runningRow.SessionID, fse.ControlRequest{
					RequestID: "ctrl-conflict-typed-001",
				})
				return err
			},
			wantHash: "sha256:1d628d2ea99e52f916a3c74b240c368ac3d496eed822cfd6bd65f6a32d4e1941",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureLifecycleConflict ||
					identity.Outcome != fse.LifecycleControlOutcomeConflict ||
					identity.Operation != fse.LifecycleControlResume {
					t.Fatalf("identity = %#v, want RESUME CONFLICT", identity)
				}
				if identity.Status != fse.LifecycleStatusPaused {
					t.Fatalf("status = %q, want PAUSED", identity.Status)
				}
			},
		},
		{
			name: "lifecycle invalid state",
			run: func() error {
				if _, err := service.StartAsync(context.Background(), fse.StartRequest{
					RequestID: "req-js-awaiting-001",
					Source: fse.Source{
						Kind:      factory.WorkflowSourceKindFactoryID,
						FactoryID: "customer-support-triage",
					},
				}); err != nil {
					return err
				}
				_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", fse.ControlRequest{})
				return err
			},
			wantHash: "sha256:6c511c017a9ef0d179fe25f803795b3aad27dd469c6f186e80c69c68f7e6b987",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureLifecycleInvalidState ||
					identity.Operation != fse.LifecycleControlPause {
					t.Fatalf("identity = %#v, want PAUSE INVALID_STATE", identity)
				}
			},
		},
		{
			name: "lifecycle terminal session",
			run: func() error {
				_, err := service.Cancel(context.Background(), successRow.SessionID, fse.ControlRequest{})
				return err
			},
			wantHash: "sha256:5521c0202e46e84f30a205891e529b616710d755cefe2f95b37912e45550283d",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureLifecycleTerminalSession ||
					identity.Operation != fse.LifecycleControlCancel {
					t.Fatalf("identity = %#v, want CANCEL TERMINAL_SESSION", identity)
				}
			},
		},
		{
			name: "malformed control missing dispatchId",
			run: func() error {
				_, err := service.RetryDispatch(context.Background(), runningRow.SessionID, fse.RetryDispatchRequest{})
				return err
			},
			wantHash: "sha256:f4ea5eb8291cb4fa851df8c7eeaacffa219dea2140e9abe6d5c7239f570a60e0",
			assert: func(t *testing.T, identity fixtures.TypedFailureIdentity) {
				if identity.Kind != fixtures.TypedFailureMalformedRequest || identity.Field != "dispatchId" {
					t.Fatalf("identity = %#v, want MALFORMED_REQUEST dispatchId", identity)
				}
			},
		},
	}
}

func runTypedFailureCases(t *testing.T, cases []typedFailureCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			identity := assertTypedFailureHash(t, err, tc.wantHash)
			tc.assert(t, identity)
		})
	}
}
