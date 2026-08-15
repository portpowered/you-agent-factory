package run

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// MapServerFailure classifies local hosting failures at the CLI boundary while
// preserving all other errors for their owning mapper. Pre-readiness runtime
// exits receive the same operator-facing context as listener bind failures.
func MapServerFailure(err error) error {
	if err == nil {
		return nil
	}

	cause := err
	var startupErr *initializer.RuntimeHostStartupError
	if errors.As(err, &startupErr) {
		cause = errors.Unwrap(startupErr)
		if cause == nil {
			cause = initializer.ErrRuntimeHostExitedBeforeReadiness
		}
	}

	mapped := factoryruntimecli.MapServerFailure(cause)
	var mappedInvocationErr *InvocationError
	if errors.As(mapped, &mappedInvocationErr) && mappedInvocationErr.Code == ServerBindFailedCode {
		return withRequestedServerMessage(mapped, err)
	}
	if startupErr == nil {
		return mapped
	}

	if coded, ok := safeCLIError(cause); ok {
		return &InvocationError{
			Code:    coded.code,
			Message: fmt.Sprintf("requested server did not start: %s: %s", coded.code, coded.message),
			Cause:   err,
		}
	}
	return &InvocationError{
		Code:    ServerStartFailedCode,
		Message: "requested server did not start: runtime startup failed (failure_class=runtime_startup_failed)",
		Cause:   err,
	}
}

type safeCLIErrorFields struct {
	code    string
	message string
}

func safeCLIError(err error) (safeCLIErrorFields, bool) {
	if err == nil {
		return safeCLIErrorFields{}, false
	}
	var coded clidiag.CodedError
	if errors.As(err, &coded) {
		fields := safeCLIErrorFields{
			code:    strings.TrimSpace(coded.CLIErrorCode()),
			message: strings.TrimSpace(coded.CLIErrorMessage()),
		}
		if fields.code != "" && fields.message != "" {
			return fields, true
		}
	}
	var invocation clidiag.InvocationCodedError
	if errors.As(err, &invocation) {
		fields := safeCLIErrorFields{
			code:    strings.TrimSpace(invocation.InvocationErrorCode()),
			message: strings.TrimSpace(invocation.InvocationErrorMessage()),
		}
		if fields.code != "" && fields.message != "" {
			return fields, true
		}
	}
	return safeCLIErrorFields{}, false
}

func withRequestedServerMessage(err error, cause error) error {
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		return err
	}
	return &InvocationError{
		Code:    invocationErr.Code,
		Message: fmt.Sprintf("requested server did not start: %s", strings.TrimSpace(invocationErr.Message)),
		Cause:   cause,
	}
}
