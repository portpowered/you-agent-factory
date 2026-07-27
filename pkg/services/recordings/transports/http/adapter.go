// Package http owns HTTP adaptation for Recordings operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Recordings root invocation, error mapping, and streaming policy for
// Recordings-owned HTTP operations remain here with the owning service.
package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Adapter maps Recordings service values at the outward HTTP boundary.
type Adapter struct {
	root recordings.Service
}

// NewAdapter constructs the Recordings HTTP representation adapter.
func NewAdapter(root recordings.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the accepted Recordings root consumed by adapter-owned operations.
func (a *Adapter) Root() recordings.Service {
	if a == nil {
		return nil
	}
	return a.root
}

// invokeSubscribeFrom forwards subscribe requests through the accepted Recordings root.
func (a *Adapter) invokeSubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if a == nil || a.root == nil {
		return recordings.SubscribeResult{}, errors.New("recordings service is required")
	}
	return a.root.SubscribeFrom(ctx, request)
}
