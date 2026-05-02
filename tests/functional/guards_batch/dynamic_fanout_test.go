package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestConfigDriven_DynamicFanout_ThreeChildren(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Config-driven fanout"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 2 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 3 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 3}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 4 {
		t.Errorf("expected provider called 4 times, got %d", provider.CallCount())
	}
}

func TestConfigDriven_DynamicFanout_AnyChildFailedRoutesParent(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Child failure fan-in"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Chapter failure recorded. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 3}
	processorExec := &failOnNthPageExecutor{failOn: 2}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(
			factory.WithWorkerExecutor("parser", parserExec),
			factory.WithWorkerExecutor("processor", processorExec),
		),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	if err := h.RunUntilCompleteError(15 * time.Second); err != nil {
		marking := h.Marking()
		t.Fatalf("RunUntilComplete: %v; token places: %#v", err, tokenPlaces(*marking))
	}

	h.Assert().
		PlaceTokenCount("chapter:failed", 1).
		PlaceTokenCount("page:complete", 2).
		PlaceTokenCount("page:failed", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("chapter:complete").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 1 {
		t.Errorf("expected failure handler provider call only, got %d", provider.CallCount())
	}
}

func TestConfigDriven_DynamicFanout_OneChild(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Single child fanout"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 1}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

func TestConfigDriven_DynamicFanout_ZeroChildren(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Zero child fanout"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 0}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing")

	if parserExec.callCount() != 1 {
		t.Errorf("expected parser called 1 time, got %d", parserExec.callCount())
	}
}

func TestConfigDriven_DynamicFanout_ParentCompletes(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic_fanout"))

	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Parent completion check"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Page 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Page 2 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Chapter finalized. COMPLETE"},
	)

	parserExec := &fanoutParserExecutor{childCount: 2}

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithExtraOptions(factory.WithWorkerExecutor("parser", parserExec)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing")

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	h.Assert().PlaceTokenCount("page:complete", 2)
}
