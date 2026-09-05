package service

import (
	"errors"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

const privateOMNIArtifactLimitBytes int64 = 16 << 20

func isPrivateOMNIOperation(operation string) bool {
	return strings.EqualFold(strings.TrimSpace(operation), models.OperationOMNI)
}

// validatePrivateOMNIResult checks the detached contract before the registrar
// can assign identity or retain a runtime-owned source. Keeping this preflight
// private prevents malformed OMNI metadata from changing the generic contract.
func validatePrivateOMNIResult(result inference.InvocationRuntimeResult) error {
	var text string
	textSeen := false
	usageSeen := false
	for _, content := range result.Content {
		switch content.Name {
		case "text":
			if textSeen || content.Modality != models.ModalityText ||
				content.ContentType != "text/plain" || content.MediaType != "text/plain" ||
				strings.TrimSpace(content.Content) == "" {
				return privateOMNIMalformedFailure()
			}
			textSeen = true
			text = content.Content
		case "usage":
			if usageSeen || content.Modality != models.ModalityJSON ||
				content.ContentType != "application/json" || content.MediaType != "application/json" {
				return privateOMNIMalformedFailure()
			}
			usageSeen = true
		default:
			return privateOMNIMalformedFailure()
		}
	}
	if !textSeen {
		return privateOMNIMalformedFailure()
	}
	if len(result.Artifacts) != 1 {
		return privateOMNIArtifactFailure()
	}
	source := result.Artifacts[0]
	if source.RefValue != "" || source.SourcePath != "" || len(source.Properties) != 0 ||
		source.Name != "text" || source.MediaType != "text/plain" ||
		source.SizeBytes < 0 || source.SizeBytes > privateOMNIArtifactLimitBytes ||
		source.SizeBytes != int64(len([]byte(text))) {
		return privateOMNIArtifactFailure()
	}
	return nil
}

func privateOMNIMalformedFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassMalformedResponse,
		Message:   "OMNI response did not contain valid text output",
		Operation: models.OperationOMNI,
		Slot:      "text",
		Cause:     models.ErrInferenceFailed,
	}
}

func privateOMNIArtifactFailure() error {
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassArtifact,
		Message:   "OMNI text artifact metadata is invalid",
		Operation: models.OperationOMNI,
		Slot:      "text",
		Cause:     models.ErrInferenceArtifactInvalid,
	}
}

func classifyPrivateOMNIError(err error) error {
	if err == nil {
		return nil
	}
	var failure *models.InvocationFailure
	if errors.As(err, &failure) || errors.Is(err, models.ErrInferenceCancelled) || errors.Is(err, models.ErrInferenceTimeout) {
		return err
	}
	return &models.InvocationFailure{
		Class:     models.InvocationFailureClassBackendProtocol,
		Message:   "OMNI backend invocation failed",
		Operation: models.OperationOMNI,
		Cause:     models.ErrInferenceFailed,
	}
}
