package interfaces

import (
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

const workContentTypeValidationMessage = "must be one of text or image"

// WorkContentValidationError identifies a generated work-content part that
// cannot be represented by the canonical domain content shape.
type WorkContentValidationError struct {
	FieldPath string
	Message   string
}

func (e WorkContentValidationError) Error() string {
	if e.FieldPath == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.FieldPath
	}
	return fmt.Sprintf("%s %s", e.FieldPath, e.Message)
}

// WorkContentFromGenerated converts generated OpenAPI work content to the
// backend-owned canonical content shape.
func WorkContentFromGenerated(content *factoryapi.WorkContent) ([]WorkContentPart, error) {
	return WorkContentFromGeneratedAtPath(content, "content")
}

// WorkContentFromGeneratedAtPath converts generated OpenAPI work content and
// reports validation errors with a caller-owned field path.
func WorkContentFromGeneratedAtPath(content *factoryapi.WorkContent, fieldPath string) ([]WorkContentPart, error) {
	return workContentFromGenerated(content, fieldPath, false)
}

// BestEffortWorkContentFromGenerated converts generated content while skipping
// unsupported legacy parts. Strict request ingress should use
// WorkContentFromGeneratedAtPath instead so malformed user input is rejected.
func BestEffortWorkContentFromGenerated(content *factoryapi.WorkContent) []WorkContentPart {
	parts, err := workContentFromGenerated(content, "", true)
	if err != nil {
		return nil
	}
	return parts
}

func workContentFromGenerated(content *factoryapi.WorkContent, fieldPath string, skipInvalid bool) ([]WorkContentPart, error) {
	if content == nil || len(*content) == 0 {
		return nil, nil
	}

	parts := make([]WorkContentPart, 0, len(*content))
	for i, part := range *content {
		textPart, textErr := part.AsWorkTextContentPart()
		if textErr == nil && textPart.Type == factoryapi.WorkContentPartTypeText {
			parts = append(parts, WorkContentPart{
				Type: WorkContentPartTypeText,
				Text: textPart.Text,
			})
			continue
		}

		imagePart, imageErr := part.AsWorkImageContentPart()
		if imageErr == nil && imagePart.Type == factoryapi.WorkContentPartTypeImage {
			parts = append(parts, WorkContentPart{
				Type: WorkContentPartTypeImage,
				File: imagePart.File,
			})
			continue
		}

		if skipInvalid {
			continue
		}
		return nil, WorkContentValidationError{
			FieldPath: workContentPartFieldPath(fieldPath, i, "type"),
			Message:   workContentTypeValidationMessage,
		}
	}
	return parts, nil
}

// GeneratedWorkContentPtr converts canonical domain content to the generated
// OpenAPI shape used by public responses and event artifacts.
func GeneratedWorkContentPtr(parts []WorkContentPart) *factoryapi.WorkContent {
	if len(parts) == 0 {
		return nil
	}
	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				continue
			}
		case WorkContentPartTypeImage:
			if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
				Type: factoryapi.WorkContentPartTypeImage,
				File: part.File,
			}); err != nil {
				continue
			}
		default:
			continue
		}
		content = append(content, generated)
	}
	if len(content) == 0 {
		return nil
	}
	return &content
}

func workContentPartFieldPath(base string, index int, field string) string {
	if base == "" {
		return fmt.Sprintf("[%d].%s", index, field)
	}
	return fmt.Sprintf("%s[%d].%s", base, index, field)
}
