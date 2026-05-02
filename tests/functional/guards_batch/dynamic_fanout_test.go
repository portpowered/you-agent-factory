package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

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
