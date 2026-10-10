// Package http adapts Operator Settings HTTP operations through the accepted
// Settings root contract. Request decoding, representation mapping, service
// invocation, error mapping, and response encoding for owned Settings HTTP
// operations remain here with the owning service.
package http

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Adapter maps Operator Settings HTTP operations through the accepted root
// contract without importing Settings internals or owning canonical Settings
// state.
type Adapter struct {
	root operatorsettings.Service
}

// NewAdapter constructs the Operator Settings HTTP adapter bound to the
// accepted root Service seam.
func NewAdapter(root operatorsettings.Service) *Adapter {
	return &Adapter{root: root}
}

func (a *Adapter) invokeLoadDocument(
	ctx context.Context,
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	return invokeWithRequestContext(ctx, func() (operatorsettings.LoadDocumentResult, error) {
		return a.root.LoadDocument(request)
	})
}

func (a *Adapter) invokeApplyDocumentUpdate(
	ctx context.Context,
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return invokeWithRequestContext(ctx, func() (operatorsettings.ApplyDocumentUpdateResult, error) {
		return a.root.ApplyDocumentUpdate(request)
	})
}

func (a *Adapter) invokeResolveEffective(
	ctx context.Context,
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return invokeWithRequestContext(ctx, func() (operatorsettings.ResolveEffectiveResult, error) {
		return a.root.ResolveEffective(request)
	})
}
