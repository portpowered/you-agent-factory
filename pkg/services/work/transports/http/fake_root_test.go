package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// rootFake is a focused Work root fake for adapter-edge tests. It avoids
// constructing state-access, content-staging, or materialization graphs.
type rootFake struct {
	work.Service

	listWork func(context.Context, string, work.ListOptions) (work.ListResult, error)
	getWork  func(context.Context, string, string) (work.ReadModel, error)
}

func (fake *rootFake) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if fake.listWork != nil {
		return fake.listWork(ctx, sessionID, options)
	}
	return work.ListResult{}, work.ErrWorkNotFound
}

func (fake *rootFake) GetWork(
	ctx context.Context,
	sessionID string,
	workID string,
) (work.ReadModel, error) {
	if fake.getWork != nil {
		return fake.getWork(ctx, sessionID, workID)
	}
	return work.ReadModel{}, work.ErrWorkNotFound
}
