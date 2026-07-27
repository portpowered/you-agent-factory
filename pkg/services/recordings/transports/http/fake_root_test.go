package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// rootFake is a focused Recordings root fake for adapter-edge tests. It avoids
// constructing ledger, lifecycle flush, replay, or artifact-export graphs.
type rootFake struct {
	recordings.Service

	subscribeFrom func(
		context.Context,
		recordings.SubscribeRequest,
	) (recordings.SubscribeResult, error)
}

func (fake *rootFake) SubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if fake.subscribeFrom != nil {
		return fake.subscribeFrom(ctx, request)
	}
	return recordings.SubscribeResult{}, recordings.ErrInvalidSubscribeScope
}
