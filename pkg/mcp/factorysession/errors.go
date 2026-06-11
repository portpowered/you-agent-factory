package factorysession

import (
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

const (
	errorCodeBadRequest        = "BAD_REQUEST"
	errorCodeValidateInvalid   = "factory_session.validate.invalid"
	errorMessageValidateInvalid = "factory source validation found blocking issues"
)

func requestValidationErrorEnvelope(err error) ToolErrorEnvelope {
	var validationErr *apisurface.RequestValidationError
	if errors.As(err, &validationErr) {
		return ToolErrorEnvelope{
			Code:      errorCodeBadRequest,
			Message:   validationErr.Error(),
			Retryable: false,
			Details: map[string]any{
				"reason": validationErr.Error(),
			},
		}
	}
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   "invalid validate source request",
		Retryable: false,
	}
}

func validationErrorEnvelopeFromPreview(preview factoryapi.FactoryPreviewResult) ToolErrorEnvelope {
	code := errorCodeValidateInvalid
	message := errorMessageValidateInvalid
	if len(preview.SourceValidationIssues) > 0 {
		code = preview.SourceValidationIssues[0].Code
		message = preview.SourceValidationIssues[0].Message
	} else if preview.SourceResolution.ArtifactRoot != nil && preview.SourceResolution.ArtifactRoot.Diagnostic != nil {
		code = preview.SourceResolution.ArtifactRoot.Diagnostic.Code
		message = preview.SourceResolution.ArtifactRoot.Diagnostic.Message
	} else if len(preview.PolicyPreview.ValidationIssues) > 0 {
		code = preview.PolicyPreview.ValidationIssues[0].Code
		message = preview.PolicyPreview.ValidationIssues[0].Message
	} else if preview.SourceResolution.Diagnostics != nil && len(*preview.SourceResolution.Diagnostics) > 0 {
		code = (*preview.SourceResolution.Diagnostics)[0].Code
		message = (*preview.SourceResolution.Diagnostics)[0].Message
	}

	return ToolErrorEnvelope{
		Code:      code,
		Message:   message,
		Retryable: false,
		Details:   validationDetailsFromPreview(preview),
	}
}

func validationDetailsFromPreview(preview factoryapi.FactoryPreviewResult) map[string]any {
	details := map[string]any{
		"valid": preview.Valid,
	}
	if len(preview.SourceValidationIssues) > 0 {
		details["sourceValidationIssues"] = preview.SourceValidationIssues
	}
	if preview.SourceResolution.Diagnostics != nil && len(*preview.SourceResolution.Diagnostics) > 0 {
		details["sourceResolutionDiagnostics"] = *preview.SourceResolution.Diagnostics
	}
	if len(preview.PolicyPreview.ValidationIssues) > 0 {
		details["policyValidationIssues"] = preview.PolicyPreview.ValidationIssues
	}
	if preview.SourceResolution.SourceHash != nil {
		details["sourceHash"] = *preview.SourceResolution.SourceHash
	}
	if preview.SourceResolution.SourceRef != nil {
		details["sourceRef"] = *preview.SourceResolution.SourceRef
	}
	if preview.PolicyPreview.PolicyHash != "" {
		details["effectivePolicyHash"] = preview.PolicyPreview.PolicyHash
	}
	if len(preview.PolicyPreview.EffectivePolicy) > 0 {
		details["effectivePolicy"] = preview.PolicyPreview.EffectivePolicy
	}
	return details
}
