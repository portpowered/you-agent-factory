// Package runtimescopes defines the Models-owned registry for detached
// Factory Session runtime bindings. It is private to the Models service.
package runtimescopes

import (
	"errors"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// ErrScopeUnknown reports a reference that does not identify a live scope in
// this Runtime Scopes service.
var ErrScopeUnknown = errors.New("models runtime scope is unknown")

// Reference is an opaque identifier issued by a Runtime Scopes service.
type Reference string

// Service opens and resolves detached Models runtime bindings without
// constructing or activating Models runtime collaborators.
type Service interface {
	Open(models.RuntimeBinding) (Reference, error)
	Resolve(Reference) (models.RuntimeBinding, error)
}
