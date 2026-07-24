package workers

import (
	"context"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

// Service is the singular Workers root contract. Cross-service peers depend on
// this interface for Workers authority for the published runtime-build,
// workstation-dispatch, and Runner-neutral slices.
//
// Additive CTR-WRK slices publish those operations on this same Service using
// plain Workers-owned request, result, value, and typed-error contracts rather
// than concrete types from service/, construction/, executor/, or similar
// implementation packages. Nested IMP-WRK runner/subservice moves, provider/*
// migrations, Wire/root, CLI-manifest, CTR-PROV, and OpenAPI package-motion
// changes remain out of scope for the root-contract packet.
//
// Existing model invocation remains on this root so peers keep one Workers
// authority surface. Approved peer root contracts (Models, Work) may appear in
// signatures where the aggregate already requires them.
type Service interface {
	// InvokeModel executes one model operation through the configured Worker
	// path. Request and result use Models-owned plain contracts rather than
	// Workers implementation structs.
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
}

// ModelInvoker is the narrow Workers role that exposes only direct model
// invocation. Prefer Service for the singular cross-service Workers seam;
// ModelInvoker remains available for callers that intentionally bind only this
// capability.
type ModelInvoker interface {
	InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error)
}
