package internal

import (
	"context"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// InvokeModel remains only as the compatibility implementation of the
// RuntimeService interface. Direct model operations are admitted by the
// Factory Session runtime and enter workers.Service.Execute with a complete,
// request-scoped target; this legacy runtime role has no implicit session
// configuration from which to construct a Workstation executor.
func (s *Service) InvokeModel(
	ctx context.Context,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	_ = s
	_ = ctx
	failureContext := workers.InferenceFailureContext{
		ModelName: strings.TrimSpace(modelName),
		Operation: strings.TrimSpace(request.Operation),
	}
	return modelinference.Result{}, classifyModelInvocationError(
		fmt.Errorf("factory service runtime is not available"), failureContext,
	)
}

func classifyModelInvocationError(err error, context workers.InferenceFailureContext) error {
	failure, ok := workers.ClassifyInferenceFailure(err, context)
	if !ok {
		return err
	}
	return failure
}
