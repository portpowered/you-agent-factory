// Package checkpointdeletionproof is an external-consumer fixture that must
// fail to compile. It exists only as evidence for
// TestExternalConsumerCannotCallDeletedCheckpointMethods in
// checkpoint_deletion_proof_test.go, which invokes the Go compiler against
// this file and asserts the expected undefined-method diagnostics. The go
// tool ignores directories named "testdata" for `./...` package patterns, so
// this file is never built, vetted, or linted as part of the normal module
// build; it is only ever compiled explicitly by that one test.
package checkpointdeletionproof

import (
	"context"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func callDeletedCheckpointMethods(ctx context.Context, svc factory.Service) {
	svc.CaptureCheckpoint()
	svc.LoadCheckpoint()
	svc.RestoreCheckpoint()
}
