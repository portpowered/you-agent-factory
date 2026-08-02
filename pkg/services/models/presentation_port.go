package models

import (
	"context"

	"go.uber.org/zap"
)

// PresentationOperatorDefaults carries the operator model-selection values
// consumed while opening a Models presentation scope. Resolution metadata and
// unrelated operator settings stay owned by Operator Settings.
type PresentationOperatorDefaults struct {
	WorkerModelProvider string
	WorkerModel         string
}

// PresentationScopeRequest is the consumer-owned request for a model
// presentation scope. Models CLI adapters may alias this value contract.
type PresentationScopeRequest struct {
	FactoryDir       string
	HomeDir          string
	OperatorDefaults PresentationOperatorDefaults
	Logger           *zap.Logger
	Verbose          bool
	ModelCacheDir    string
}

// PresentationScope is the consumer-owned model presentation scope.
type PresentationScope struct {
	Scope RuntimeScopeRef
	Close func(context.Context) error
}
