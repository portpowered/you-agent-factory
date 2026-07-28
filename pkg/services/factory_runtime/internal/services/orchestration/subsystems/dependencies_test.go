package subsystems

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
)

func testSubsystemNow() time.Time { return time.Now() }

func testTokenTransformer(net *state.Net) *token_transformer.Transformer {
	return token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator())
}
