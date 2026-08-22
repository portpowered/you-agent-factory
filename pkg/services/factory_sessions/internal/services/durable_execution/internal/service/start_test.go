package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestDurableStartAsyncReturnsPublishedSuccessShape(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	started, err := owner.StartAsync(
		context.Background(),
		factorysessions.DurableStartRequest(startRequestForPublished(row)),
	)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", started.SessionID, row.SessionID)
	}
	if started.Status != string(factorysessions.LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}
	if started.OrchestratorKind == "" {
		t.Fatalf("StartAsync = %#v, want durable async start with orchestrator kind", started)
	}
}

func TestDurableStartSyncReturnsPublishedSuccessShapeWithSyncOutcome(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	started, err := owner.StartSync(
		context.Background(),
		factorysessions.DurableStartRequest(startRequestForPublished(row)),
	)
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", started.SessionID, row.SessionID)
	}
	if started.Status != string(factorysessions.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if started.SyncOutcome != factorysessions.SyncOutcome("COMPLETED") {
		t.Fatalf("syncOutcome = %q, want COMPLETED", started.SyncOutcome)
	}
}

func TestDurableStartValidationErrorsDistinguishInvalidFields(t *testing.T) {
	t.Parallel()

	owner, err := New(&startValidationStub{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	_, err = owner.StartAsync(ctx, factorysessions.DurableStartRequest{RequestID: "req-invalid-source"})
	var invalidSource *factorysessions.DurableValidationError
	if !errors.As(err, &invalidSource) || invalidSource.Field != "source" {
		t.Fatalf("StartAsync invalid source = %v, want *DurableValidationError field=source", err)
	}

	_, err = owner.StartSync(ctx, factorysessions.DurableStartRequest{RequestID: "req-invalid-policy"})
	var invalidPolicy *factorysessions.DurableValidationError
	if !errors.As(err, &invalidPolicy) || invalidPolicy.Field != "requestedPolicy" {
		t.Fatalf("StartSync invalid policy = %v, want *DurableValidationError field=requestedPolicy", err)
	}
}

type startValidationStub struct {
	durableexecution.Service
}

func (s *startValidationStub) StartAsync(_ context.Context, req factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	if req.RequestID == "req-invalid-source" {
		return factorysessions.AsyncStartResult{}, &factorysessions.DurableValidationError{
			Field:   "source",
			Message: "source is invalid",
		}
	}
	return factorysessions.AsyncStartResult{}, errors.New("unexpected start request")
}

func (s *startValidationStub) StartSync(_ context.Context, req factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	if req.RequestID == "req-invalid-policy" {
		return factorysessions.SyncStartResult{}, &factorysessions.DurableValidationError{
			Field:   "requestedPolicy",
			Message: "requestedPolicy is invalid",
		}
	}
	return factorysessions.SyncStartResult{}, errors.New("unexpected start request")
}

func newFixtureBackedExecution(t *testing.T) durableexecution.Service {
	t.Helper()
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(
		contractFixtureCatalogPath(t),
		fixtureClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		fileeffects.ContractFixtureReader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(
		"..", "..", "..", "..", "..", "..", "..",
		"transports", "http", "testdata", "durable-session-contract-fixtures.json",
	)
}

func publishedScenarioByPurpose(t *testing.T, purpose fixtures.FixtureScenarioPurpose) fixtures.PublishedFixtureScenario {
	t.Helper()
	for _, row := range fixtures.PublishedFixtureScenarios {
		if row.Purpose == purpose {
			return row
		}
	}
	t.Fatalf("published scenario missing for purpose %q", purpose)
	return fixtures.PublishedFixtureScenario{}
}

func startRequestForPublished(row fixtures.PublishedFixtureScenario) factorysessions.StartRequest {
	return factorysessions.StartRequest{
		RequestID: row.RequestID,
		Source: factorysessions.Source{
			Kind:      factory.WorkflowSourceKindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}
}

type fixtureClock struct {
	now time.Time
}

func (c fixtureClock) Now() time.Time { return c.now }
