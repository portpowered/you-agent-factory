package api

import (
	"encoding/json"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
)

func submitWorkContent(req factoryapi.SubmitWorkRequest) ([]interfaces.WorkContentPart, error) {
	if req.Items == nil {
		return workcontent.PartsFromGenerated(req.Content), nil
	}
	return submitWorkItemsToContent(*req.Items)
}

func submitWorkItemsToContent(items []factoryapi.SubmitWorkItem) ([]interfaces.WorkContentPart, error) {
	if len(items) == 0 {
		return []interfaces.WorkContentPart{}, nil
	}

	content := make([]interfaces.WorkContentPart, 0, len(items))
	hasMeaningfulItem := false
	for i, item := range items {
		part, meaningful, err := submitWorkItemToContentPart(item)
		if err != nil {
			return nil, requestFieldValidationError{message: fmt.Sprintf("items[%d]: %v", i, err)}
		}
		content = append(content, part)
		hasMeaningfulItem = hasMeaningfulItem || meaningful
	}
	if !hasMeaningfulItem {
		return nil, requestFieldValidationError{message: "items must contain at least one non-empty item"}
	}

	return content, nil
}

func submitWorkItemToContentPart(item factoryapi.SubmitWorkItem) (interfaces.WorkContentPart, bool, error) {
	textItem, textErr := item.AsSubmitWorkTextItem()
	if textErr == nil && textItem.Type == factoryapi.SubmitWorkItemTypeText {
		return interfaces.WorkContentPart{
			Type: interfaces.WorkContentPartTypeText,
			Text: textItem.Text,
		}, strings.TrimSpace(textItem.Text) != "", nil
	}

	imageItem, imageErr := item.AsSubmitWorkImageItem()
	if imageErr == nil && imageItem.Type == factoryapi.SubmitWorkItemTypeImage {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(imageItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeImage,
			string(imageItem.Type),
			stagedFilePath,
			imageItem.FileName,
			imageItem.MediaType,
		), true, nil
	}

	videoItem, videoErr := item.AsSubmitWorkVideoItem()
	if videoErr == nil && videoItem.Type == factoryapi.SubmitWorkItemTypeVideo {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(videoItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeBinary,
			string(videoItem.Type),
			stagedFilePath,
			videoItem.FileName,
			videoItem.MediaType,
		), true, nil
	}

	audioItem, audioErr := item.AsSubmitWorkAudioItem()
	if audioErr == nil && audioItem.Type == factoryapi.SubmitWorkItemTypeAudio {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(audioItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeAudio,
			string(audioItem.Type),
			stagedFilePath,
			audioItem.FileName,
			audioItem.MediaType,
		), true, nil
	}

	documentItem, documentErr := item.AsSubmitWorkDocumentItem()
	if documentErr == nil && documentItem.Type == factoryapi.SubmitWorkItemTypeDocument {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(documentItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeBinary,
			string(documentItem.Type),
			stagedFilePath,
			documentItem.FileName,
			documentItem.MediaType,
		), true, nil
	}

	return interfaces.WorkContentPart{}, false, fmt.Errorf("unsupported item type")
}

func submitWorkFileItemContentPart(
	partType interfaces.WorkContentPartType,
	itemType string,
	stagedFileRef string,
	fileName string,
	mediaType string,
) interfaces.WorkContentPart {
	return interfaces.WorkContentPart{
		Type:        partType,
		File:        stagedFileRef,
		ContentType: mediaType,
		Metadata: map[string]any{
			submitWorkItemTypeMetadataKey: itemType,
			submitWorkFileNameMetadataKey: fileName,
		},
	}
}

func validateSubmitWorkStructuredInputFields(fields map[string]json.RawMessage) error {
	if _, ok := fields["items"]; !ok {
		return nil
	}
	if _, ok := fields["content"]; ok {
		return requestFieldValidationError{message: "items cannot be combined with content"}
	}
	if _, ok := fields["payload"]; ok {
		return requestFieldValidationError{message: "items cannot be combined with payload"}
	}
	return validateSubmitWorkItemsField(fields, "")
}

func validateSubmitWorkItemsField(fields map[string]json.RawMessage, prefix string) error {
	itemsRaw, ok := fields["items"]
	if !ok {
		return nil
	}

	var itemPayloads []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &itemPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%sitems must be an array", prefix)}
	}
	if len(itemPayloads) == 0 {
		return nil
	}

	hasMeaningfulItem := false
	for i, payload := range itemPayloads {
		var itemFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &itemFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%sitems[%d] must be an object", prefix, i)}
		}
		meaningful, err := validateSubmitWorkItemField(itemFields, fmt.Sprintf("%sitems[%d].", prefix, i))
		if err != nil {
			return err
		}
		hasMeaningfulItem = hasMeaningfulItem || meaningful
	}
	if !hasMeaningfulItem {
		return requestFieldValidationError{message: fmt.Sprintf("%sitems must contain at least one non-empty item", prefix)}
	}

	return nil
}

func validateSubmitWorkItemField(fields map[string]json.RawMessage, prefix string) (bool, error) {
	itemType, err := requiredSubmitWorkItemType(fields, prefix)
	if err != nil {
		return false, err
	}

	switch itemType {
	case factoryapi.SubmitWorkItemTypeText:
		if err := requireOnlyFields(fields, prefix, "type", "text"); err != nil {
			return false, err
		}
		text, err := requiredStringField(fields, prefix, "text", "text items")
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(text) != "", nil
	case factoryapi.SubmitWorkItemTypeImage, factoryapi.SubmitWorkItemTypeVideo, factoryapi.SubmitWorkItemTypeAudio, factoryapi.SubmitWorkItemTypeDocument:
		if err := requireOnlyFields(fields, prefix, "type", "stagedFileRef", "fileName", "mediaType"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "stagedFileRef", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "fileName", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "mediaType", string(itemType)+" items"); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

func requiredSubmitWorkItemType(fields map[string]json.RawMessage, prefix string) (factoryapi.SubmitWorkItemType, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var itemType string
	if err := json.Unmarshal(typeRaw, &itemType); err != nil || itemType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	switch factoryapi.SubmitWorkItemType(itemType) {
	case factoryapi.SubmitWorkItemTypeText,
		factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
		return factoryapi.SubmitWorkItemType(itemType), nil
	default:
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

func submitWorkResponseFromResult(result interfaces.WorkRequestSubmitResult) factoryapi.SubmitWorkResponse {
	response := factoryapi.SubmitWorkResponse{TraceId: result.TraceID}
	if result.WorkID != "" {
		response.WorkId = &result.WorkID
	}
	if result.Name != "" {
		response.Name = &result.Name
	}
	if result.WorkTypeName != "" {
		response.WorkTypeName = &result.WorkTypeName
	}
	return response
}
