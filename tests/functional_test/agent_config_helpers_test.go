package functional_test

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/workers"
)

func buildModelWorkerConfig(provider workers.ModelProvider, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
}
