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
	text, err := privateOMNIText(result.Content)
	if err != nil {
		return err
	}
	if !validPrivateOMNIArtifact(result.Artifacts, text) {
		return privateOMNIArtifactFailure()
	}
	return nil
}

func privateOMNIText(contents []models.InferenceContent) (string, error) {
	text := ""
	usageSeen := false
	textSeen := false
	for _, content := range contents {
		switch content.Name {
		case "text":
			if textSeen || !validPrivateOMNITextContent(content) {
				return "", privateOMNIMalformedFailure()
			}
			textSeen = true
			text = content.Content
		case "usage":
			if usageSeen || !validPrivateOMNIUsageContent(content) {
				return "", privateOMNIMalformedFailure()
			}
			usageSeen = true
		default:
			return "", privateOMNIMalformedFailure()
		}
	}
	if !textSeen {
		return "", privateOMNIMalformedFailure()
	}
	return text, nil
}

func validPrivateOMNITextContent(content models.InferenceContent) bool {
	return content.Modality == models.ModalityText &&
		content.ContentType == "text/plain" && content.MediaType == "text/plain" &&
		strings.TrimSpace(content.Content) != ""
}

func validPrivateOMNIUsageContent(content models.InferenceContent) bool {
	return content.Modality == models.ModalityJSON &&
		content.ContentType == "application/json" && content.MediaType == "application/json"
}

func validPrivateOMNIArtifact(artifacts []inference.InvocationArtifactSource, text string) bool {
	if len(artifacts) != 1 {
		return false
	}
	source := artifacts[0]
	return source.RefValue == "" && source.SourcePath == "" && len(source.Properties) == 0 &&
		source.Name == "text" && source.MediaType == "text/plain" &&
		source.SizeBytes >= 0 && source.SizeBytes <= privateOMNIArtifactLimitBytes &&
		source.SizeBytes == int64(len([]byte(text)))
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
