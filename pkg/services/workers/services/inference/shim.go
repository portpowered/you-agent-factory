// Package inference is a transitional compile shim that re-exports inference
// operation binding helpers from the private runners destination. Peers should
// resolve through workers/wire; baseline deletion of this path is owned by
// DEL-WRK.
package inference

import (
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/inference"
)

var (
	ResolveInferenceOperationBindings = runnerinference.ResolveInferenceOperationBindings
	DirectInferenceWorkstationConfig  = runnerinference.DirectInferenceWorkstationConfig
	InferenceOperationUserMessage     = runnerinference.InferenceOperationUserMessage
	WorkContentFromInferenceOutput    = runnerinference.WorkContentFromInferenceOutput
	MarshalWorkContentOutput          = runnerinference.MarshalWorkContentOutput
)
