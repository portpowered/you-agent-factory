//go:build functionallong

package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestLogicalMove_Success(t *testing.T) {
	support.SkipLongFunctional(t, "slow logical-move success sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("my-payload"))
	h := testutil.NewServiceTestHarness(t, dir)

	h.SetCustomExecutor("logical-router", &workers.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical-router": {Type: interfaces.WorkstationTypeLogical},
				"router":         {Type: interfaces.WorkstationTypeLogical},
			},
		},
		Renderer: &workers.DefaultPromptRenderer{},
	})

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")
}

func TestLogicalMove_PreservesTokenColor(t *testing.T) {
	support.SkipLongFunctional(t, "slow logical-move color sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_pipeline_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("preserved-payload"))
	h := testutil.NewServiceTestHarness(t, dir)

	h.SetCustomExecutor("logical-router", &workers.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical-router": {Type: interfaces.WorkstationTypeLogical},
				"router":         {Type: interfaces.WorkstationTypeLogical},
			},
		},
		Renderer: &workers.DefaultPromptRenderer{},
	})

	capExec := &capturePayloadExecutor{}
	h.SetCustomExecutor("model-worker", capExec)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:staging")

	if string(capExec.capturedPayload) != "preserved-payload" {
		t.Errorf("expected payload %q preserved through logical move, got %q",
			"preserved-payload", capExec.capturedPayload)
	}
}

type capturePayloadExecutor struct {
	capturedPayload []byte
}

func (c *capturePayloadExecutor) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	if len(d.InputTokens) > 0 {
		c.capturedPayload = firstInputToken(d.InputTokens).Color.Payload
	}
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}
