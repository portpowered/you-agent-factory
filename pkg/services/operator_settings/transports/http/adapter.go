// Package http adapts Operator Settings HTTP operations through the accepted
// Settings root contract. Request decoding, representation mapping, service
// invocation, error mapping, and response encoding for owned Settings HTTP
// operations remain here with the owning service.
package http

import (
	"errors"

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
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the accepted Operator Settings root consumed by adapter-owned
// operations.
func (a *Adapter) Root() operatorsettings.Service {
	if a == nil {
		return nil
	}
	return a.root
}

func (a *Adapter) invokeLoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if a == nil || a.root == nil {
		return operatorsettings.LoadDocumentResult{}, errors.New("operator settings service is required")
	}
	return a.root.LoadDocument(request)
}
