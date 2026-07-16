package workcontent

import (
	"encoding/json"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

// PartsFromGenerated translates supported generated work content parts into the
// backend-owned canonical shape while preserving order.
func PartsFromGenerated(content *factoryapi.WorkContent) []work.WorkContentPart {
	if content == nil || len(*content) == 0 {
		return nil
	}

	parts := make([]work.WorkContentPart, 0, len(*content))
	for _, part := range *content {
		domainPart, ok := PartFromGenerated(part)
		if ok {
			parts = append(parts, domainPart)
		}
	}

	return parts
}

// GeneratedPtrFromParts translates supported canonical work content parts into
// the generated API shape while preserving order.
func GeneratedPtrFromParts(parts []work.WorkContentPart) *factoryapi.WorkContent {
	if len(parts) == 0 {
		return nil
	}

	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		generated, ok := GeneratedPartFromPart(part)
		if !ok {
			continue
		}
		content = append(content, generated)
	}
	if len(content) == 0 {
		return nil
	}

	return &content
}

// PartFromGenerated translates one generated content part into the canonical
// backend-owned shape.
func PartFromGenerated(part factoryapi.WorkContentPart) (work.WorkContentPart, bool) {
	textPart, textErr := part.AsWorkTextContentPart()
	if textErr == nil {
		switch textPart.Type {
		case factoryapi.WorkContentPartTypeText, factoryapi.WorkContentPartTypeTextUpper:
			return work.WorkContentPart{
				Type:        work.WorkContentPartType(textPart.Type).Normalized(),
				Text:        textPart.Text,
				Slot:        stringValue(textPart.Slot),
				Label:       stringValue(textPart.Label),
				Role:        stringValue(textPart.Role),
				ContentType: stringValue(textPart.ContentType),
				ArtifactID:  stringValue(textPart.ArtifactId),
				Metadata:    cloneMetadata(textPart.Metadata),
			}, true
		}
	}

	imagePart, imageErr := part.AsWorkImageContentPart()
	if imageErr == nil {
		switch imagePart.Type {
		case factoryapi.WorkContentPartTypeImage, factoryapi.WorkContentPartTypeImageUpper:
			return work.WorkContentPart{
				Type:        work.WorkContentPartType(imagePart.Type).Normalized(),
				URL:         string(imagePart.Url),
				File:        deprecatedFileValue(imagePart.File),
				Slot:        stringValue(imagePart.Slot),
				Label:       stringValue(imagePart.Label),
				Role:        stringValue(imagePart.Role),
				ContentType: stringValue(imagePart.ContentType),
				ArtifactID:  stringValue(imagePart.ArtifactId),
				Metadata:    cloneMetadata(imagePart.Metadata),
			}, true
		}
	}

	audioPart, audioErr := part.AsWorkAudioContentPart()
	if audioErr == nil && audioPart.Type == factoryapi.WorkContentPartTypeAudio {
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			URL:         string(audioPart.Url),
			File:        deprecatedFileValue(audioPart.File),
			Slot:        stringValue(audioPart.Slot),
			Label:       stringValue(audioPart.Label),
			Role:        stringValue(audioPart.Role),
			ContentType: stringValue(audioPart.ContentType),
			ArtifactID:  stringValue(audioPart.ArtifactId),
			Metadata:    cloneMetadata(audioPart.Metadata),
		}, true
	}

	jsonPart, jsonErr := part.AsWorkJsonContentPart()
	if jsonErr == nil && jsonPart.Type == factoryapi.WorkContentPartTypeJSON {
		rawJSON, err := json.Marshal(jsonPart.Json)
		if err != nil {
			return work.WorkContentPart{}, false
		}
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeJSON,
			JSON:        cloneRawMessage(rawJSON),
			Slot:        stringValue(jsonPart.Slot),
			Label:       stringValue(jsonPart.Label),
			Role:        stringValue(jsonPart.Role),
			ContentType: stringValue(jsonPart.ContentType),
			ArtifactID:  stringValue(jsonPart.ArtifactId),
			Metadata:    cloneMetadata(jsonPart.Metadata),
		}, true
	}

	binaryPart, binaryErr := part.AsWorkBinaryContentPart()
	if binaryErr == nil && binaryPart.Type == factoryapi.WorkContentPartTypeBinary {
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeBinary,
			URL:         string(binaryPart.Url),
			File:        deprecatedFileValue(binaryPart.File),
			Slot:        stringValue(binaryPart.Slot),
			Label:       stringValue(binaryPart.Label),
			Role:        stringValue(binaryPart.Role),
			ContentType: stringValue(binaryPart.ContentType),
			ArtifactID:  stringValue(binaryPart.ArtifactId),
			Metadata:    cloneMetadata(binaryPart.Metadata),
		}, true
	}

	return work.WorkContentPart{}, false
}

// GeneratedPartFromPart translates one canonical content part into the
// generated API shape.
func GeneratedPartFromPart(part work.WorkContentPart) (factoryapi.WorkContentPart, bool) {
	var generated factoryapi.WorkContentPart
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
			Type:        factoryapi.WorkContentPartType(part.Type.Normalized()),
			Text:        part.Text,
			Slot:        stringPtr(part.Slot),
			Label:       stringPtr(part.Label),
			Role:        stringPtr(part.Role),
			ContentType: stringPtr(part.ContentType),
			ArtifactId:  stringPtr(part.ArtifactID),
			Metadata:    metadataPtr(part.Metadata),
		}); err != nil {
			return factoryapi.WorkContentPart{}, false
		}
	case work.WorkContentPartTypeImage:
		if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
			Type:        factoryapi.WorkContentPartType(part.Type.Normalized()),
			Url:         factoryapi.WorkContentURLProperty(part.URL),
			File:        deprecatedFilePtr(part.File),
			Slot:        stringPtr(part.Slot),
			Label:       stringPtr(part.Label),
			Role:        stringPtr(part.Role),
			ContentType: stringPtr(part.ContentType),
			ArtifactId:  stringPtr(part.ArtifactID),
			Metadata:    metadataPtr(part.Metadata),
		}); err != nil {
			return factoryapi.WorkContentPart{}, false
		}
	case work.WorkContentPartTypeAudio:
		if err := generated.FromWorkAudioContentPart(factoryapi.WorkAudioContentPart{
			Type:        factoryapi.WorkContentPartTypeAudio,
			Url:         factoryapi.WorkContentURLProperty(part.URL),
			File:        deprecatedFilePtr(part.File),
			Slot:        stringPtr(part.Slot),
			Label:       stringPtr(part.Label),
			Role:        stringPtr(part.Role),
			ContentType: stringPtr(part.ContentType),
			ArtifactId:  stringPtr(part.ArtifactID),
			Metadata:    metadataPtr(part.Metadata),
		}); err != nil {
			return factoryapi.WorkContentPart{}, false
		}
	case work.WorkContentPartTypeJSON:
		var jsonValue any
		if len(part.JSON) != 0 {
			if err := json.Unmarshal(part.JSON, &jsonValue); err != nil {
				return factoryapi.WorkContentPart{}, false
			}
		}
		if err := generated.FromWorkJsonContentPart(factoryapi.WorkJsonContentPart{
			Type:        factoryapi.WorkContentPartTypeJSON,
			Json:        jsonValue,
			Slot:        stringPtr(part.Slot),
			Label:       stringPtr(part.Label),
			Role:        stringPtr(part.Role),
			ContentType: stringPtr(part.ContentType),
			ArtifactId:  stringPtr(part.ArtifactID),
			Metadata:    metadataPtr(part.Metadata),
		}); err != nil {
			return factoryapi.WorkContentPart{}, false
		}
	case work.WorkContentPartTypeBinary:
		if err := generated.FromWorkBinaryContentPart(factoryapi.WorkBinaryContentPart{
			Type:        factoryapi.WorkContentPartTypeBinary,
			Url:         factoryapi.WorkContentURLProperty(part.URL),
			File:        deprecatedFilePtr(part.File),
			Slot:        stringPtr(part.Slot),
			Label:       stringPtr(part.Label),
			Role:        stringPtr(part.Role),
			ContentType: stringPtr(part.ContentType),
			ArtifactId:  stringPtr(part.ArtifactID),
			Metadata:    metadataPtr(part.Metadata),
		}); err != nil {
			return factoryapi.WorkContentPart{}, false
		}
	default:
		return factoryapi.WorkContentPart{}, false
	}

	return generated, true
}

func deprecatedFileValue(file *factoryapi.WorkContentDeprecatedFileProperty) string {
	if file == nil {
		return ""
	}
	return string(*file)
}

func deprecatedFilePtr(value string) *factoryapi.WorkContentDeprecatedFileProperty {
	if value == "" {
		return nil
	}
	typed := factoryapi.WorkContentDeprecatedFileProperty(value)
	return &typed
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func cloneMetadata(value *factoryapi.WorkContentMetadata) map[string]any {
	if value == nil || len(*value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(*value))
	for key, entry := range *value {
		cloned[key] = entry
	}
	return cloned
}

func metadataPtr(value map[string]any) *factoryapi.WorkContentMetadata {
	if len(value) == 0 {
		return nil
	}
	cloned := make(factoryapi.WorkContentMetadata, len(value))
	for key, entry := range value {
		cloned[key] = entry
	}
	return &cloned
}
