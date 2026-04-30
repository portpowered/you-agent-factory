package functional_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// TestLogicalMove_Success verifies the success path: a LOGICAL_MOVE workstation
// passes an input token through to the output place without invoking any LLM.
func TestLogicalMove_Success(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "logical_move_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("my-payload"))
	h := testutil.NewServiceTestHarness(t, dir)

	// Register a real WorkstationExecutor with LOGICAL_MOVE type — no LLM called.
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

// TestLogicalMove_PreservesTokenColor verifies that a LOGICAL_MOVE workstation
// in a pipeline preserves the input token's color (WorkID, payload) so that
// subsequent steps receive the full token context.
func TestLogicalMove_PreservesTokenColor(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "logical_move_pipeline_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("preserved-payload"))
	h := testutil.NewServiceTestHarness(t, dir)

	// Logical move executor — passes token through unchanged.
	h.SetCustomExecutor("logical-router", &workers.WorkstationExecutor{
		RuntimeConfig: runtimefixtures.RuntimeConfigLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical-router": {Type: interfaces.WorkstationTypeLogical},
				"router":         {Type: interfaces.WorkstationTypeLogical},
			},
		},
		Renderer: &workers.DefaultPromptRenderer{},
	})

	// Model worker captures the dispatch to verify the payload was preserved.
	capExec := &capturePayloadExecutor{}
	h.SetCustomExecutor("model-worker", capExec)

	// Run to completion so both transitions fire.
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

// capturePayloadExecutor records the payload from the first input token.
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
