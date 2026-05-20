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
