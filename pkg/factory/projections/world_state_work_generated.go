package projections

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func factoryWorkItemsFromGenerated(works *[]factoryapi.Work) []interfaces.FactoryWorkItem {
	if works == nil {
		return nil
	}
	out := make([]interfaces.FactoryWorkItem, 0, len(*works))
	for _, work := range *works {
		item := factoryWorkItemFromGenerated(work)
		if item.ID != "" {
			out = append(out, item)
		}
	}
	return out
}

func factoryWorkItemFromGenerated(work factoryapi.Work) interfaces.FactoryWorkItem {
	currentChainingTraceID := stringValue(work.CurrentChainingTraceId)
	traceID := stringValue(work.TraceId)
	if currentChainingTraceID == "" {
		currentChainingTraceID = traceID
	}
	return interfaces.FactoryWorkItem{
		ID:                       stringValue(work.WorkId),
		WorkTypeID:               stringValue(work.WorkTypeName),
		State:                    generatedWorkStateName(work.State),
		DisplayName:              work.Name,
		ChainingTraceDepth:       intValue(work.ChainingTraceDepth),
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(sliceValue(work.PreviousChainingTraceIds)),
		TraceID:                  traceID,
		Content:                  generatedWorkContentToDomain(work.Content),
		Tags:                     stringMapFromGenerated(work.Tags),
	}
}

func generatedWorkContentToDomain(content *factoryapi.WorkContent) []interfaces.WorkContentPart {
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
