package service

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (unusedExecution) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	return ctx.Err()
}
func (failingPublishBoundary) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	return ctx.Err()
}
func (coverageExecution) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	return ctx.Err()
}
func (*controlledBoundary) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	return ctx.Err()
}
func (*admitBeforeCompletionBoundary) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	return ctx.Err()
}
