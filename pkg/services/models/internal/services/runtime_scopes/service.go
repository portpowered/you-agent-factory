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

// ErrScopeForeign reports a reference issued by another Runtime Scopes
// service instance.
var ErrScopeForeign = errors.New("models runtime scope is foreign")

// ErrScopeClosed reports a reference that this Runtime Scopes service issued
// and explicitly closed.
var ErrScopeClosed = errors.New("models runtime scope is closed")

// Reference is an opaque identifier issued by a Runtime Scopes service.
type Reference string

// IssuerIDGenerator supplies a collision-resistant identity for one Runtime
// Scopes service instance. The composition boundary owns this dependency so
// the registry does not use time, randomness, environment, or global state.
type IssuerIDGenerator func() string

// Service opens, resolves, and closes detached Models runtime bindings without
// constructing or activating Models runtime collaborators.
type Service interface {
	Open(models.RuntimeBinding) (Reference, error)
	Resolve(Reference) (models.RuntimeBinding, error)
	Close(Reference) error
}
