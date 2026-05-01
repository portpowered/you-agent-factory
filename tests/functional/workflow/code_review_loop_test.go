package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestCodeReviewLoop(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte("implement feature X"))

	work := map[string][]interfaces.InferenceResponse{
		"swe": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with proper error handling <COMPLETE>"},
		},
		"reviewer": {
			{Content: "missing error handling"},
			{Content: "looks good<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:failed").
		TokenCount(1)

	if provider.CallCount("swe") != 2 {
		t.Errorf("expected swe called 2 times, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}

	sweCalls := provider.Calls("swe")
	if len(sweCalls) < 2 {
		t.Fatalf("expected at least 2 swe calls, got %d", len(sweCalls))
	}
	secondDispatch := sweCalls[1]
	if len(secondDispatch.UserMessage) == 0 {
		t.Fatal("second coding dispatch has no input tokens")
	}
}
