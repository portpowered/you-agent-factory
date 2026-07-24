// Package admission defines the Work-owned private nested admission subservice.
// Cross-service peers continue to use the Work root Service admission slice;
// they do not import this parent-private contract.
package admission

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the singular admission subservice-root contract for normalize,
// validate, and idempotent-accept operations required by the CTR-WORK
// admission slice.
type Service interface {
	Normalize(context.Context, NormalizeRequest) (NormalizeResult, error)
	Validate(context.Context, ValidateRequest) error
	Accept(context.Context, AcceptRequest) (work.WorkRequestSubmitResult, error)
}

// NormalizeRequest carries a Work Request and optional normalize options.
type NormalizeRequest struct {
	Request work.WorkRequest
	Options work.WorkRequestNormalizeOptions
}

// NormalizeResult is the detached normalized admission input shape produced
// before idempotent accept.
type NormalizeResult struct {
	RequestID  string
	Normalized []work.SubmitRequest
}

// ValidateRequest carries a Work Request and optional normalize options used
// for validation without accept.
type ValidateRequest struct {
	Request work.WorkRequest
	Options work.WorkRequestNormalizeOptions
}

// AcceptRequest carries a previously normalized admission batch for
// idempotent accept.
type AcceptRequest struct {
	RequestID  string
	Normalized []work.SubmitRequest
}
