package models

import (
	"context"

	"go.uber.org/zap"
)

// PresentationScopeRequest is the consumer-owned request for a model
// presentation scope. Models CLI adapters may alias this value contract.
type PresentationScopeRequest struct {
	FactoryDir       string
	HomeDir          string
	OperatorDefaults any
	Logger           *zap.Logger
	Verbose          bool
	ModelCacheDir    string
}

// PresentationScope is the consumer-owned model presentation scope.
type PresentationScope struct {
	Scope RuntimeScopeRef
	Close func(context.Context) error
}
