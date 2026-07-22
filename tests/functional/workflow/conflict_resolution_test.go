package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"code-change:complete":            1,
		"code-change:failed":              0,
		"code-change:resolving-conflicts": 0,
		"code-change:in-review":           0,
	})

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

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"code-change:failed":              1,
		"code-change:complete":            0,
		"code-change:resolving-conflicts": 0,
	})
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

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"code-change:complete":            1,
		"code-change:resolving-conflicts": 0,
	})

	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", provider.CallCount("reviewer"))
	}
	if provider.CallCount("conflict-resolver") != 0 {
		t.Errorf("expected conflict-resolver called 0 time, got %d", provider.CallCount("conflict-resolver"))
	}
}
