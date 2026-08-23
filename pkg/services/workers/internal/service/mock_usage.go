package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
)

func applyMockWorkerUsageDiagnostics(
	result workers.RunnerExecutionResult,
	usage *workers.MockWorkerUsageConfig,
) workers.RunnerExecutionResult {
	return workerexecution.ApplyMockWorkerUsageDiagnostics(result, usage)
}

func publishMockWorkerUsage(
	ctx context.Context,
	request workers.ExecuteRequest,
	usage *workers.MockWorkerUsageConfig,
) {
	workerexecution.PublishMockWorkerUsage(ctx, request.Correlation, usage)
}
