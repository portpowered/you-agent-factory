package workcontent

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PartsFromGenerated translates supported generated work content parts into the
// backend-owned canonical shape while preserving order.
func PartsFromGenerated(content *factoryapi.WorkContent) []interfaces.WorkContentPart {
	if content == nil || len(*content) == 0 {
		return nil
	}

	parts := make([]interfaces.WorkContentPart, 0, len(*content))
	for _, part := range *content {
		textPart, textErr := part.AsWorkTextContentPart()
		if textErr == nil && textPart.Type == factoryapi.WorkContentPartTypeText {
			parts = append(parts, interfaces.WorkContentPart{
				Type: interfaces.WorkContentPartTypeText,
				Text: textPart.Text,
			})
			continue
		}

		imagePart, imageErr := part.AsWorkImageContentPart()
		if imageErr == nil && imagePart.Type == factoryapi.WorkContentPartTypeImage {
			parts = append(parts, interfaces.WorkContentPart{
				Type: interfaces.WorkContentPartTypeImage,
				File: imagePart.File,
			})
		}
	}

	return parts
}

// GeneratedPtrFromParts translates supported canonical work content parts into
// the generated API shape while preserving order.
func GeneratedPtrFromParts(parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	if len(parts) == 0 {
		return nil
	}

	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				continue
			}
		case interfaces.WorkContentPartTypeImage:
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
