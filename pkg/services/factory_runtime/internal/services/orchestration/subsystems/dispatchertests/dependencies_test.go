package subsystems_test

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/token_transformer"
)

var dispatchIDSequence atomic.Int64

func testDispatchID() string {
	return fmt.Sprintf("dispatch-test-id-%d", dispatchIDSequence.Add(1))
}

func testTransitioner(net *state.Net) *token_transformer.Transformer {
	return token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator())
}

func testTransitionerNow() time.Time { return time.Now() }
