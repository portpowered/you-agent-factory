package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestConflictResolution_ReviewFailResolveReReview(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Error: errors.New("failed")},
			{Content: "Task execution failed.<COMPLETE>"},
		},
		"conflict-resolver": {
			{Content: "Conflicts resolved.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.MockWorker("swe",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:resolving-conflicts").
		HasNoTokenInPlace("code-change:in-review")

	if provider.CallCount("reviewer") != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}
	if provider.CallCount("conflict-resolver") != 1 {
		t.Errorf("expected conflict-resolver called 1 time, got %d", provider.CallCount("conflict-resolver"))
	}
}

func TestConflictResolution_ResolverFails(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Error: errors.New("failed")},
		},
		"conflict-resolver": {
			{Error: errors.New("failed")},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:resolving-conflicts")
}

func TestConflictResolution_ReviewApproveFirstTry(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Content: "Task execution failed.<COMPLETE>"},
		},
		"conflict-resolver": {
			{Content: "Conflicts resolved.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:resolving-conflicts")

	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", provider.CallCount("reviewer"))
	}
	if provider.CallCount("conflict-resolver") != 0 {
		t.Errorf("expected conflict-resolver called 0 time, got %d", provider.CallCount("conflict-resolver"))
	}
}
