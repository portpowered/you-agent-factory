package service

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// cloneDispatchResult gives the executor, canonical terminal record, and each
// caller independent ownership of mutable result metadata.
func cloneDispatchResult(result workers.WorkstationDispatchResult) workers.WorkstationDispatchResult {
	result.Result = cloneWorkResult(result.Result)
	return result
}

func cloneWorkResult(result workers.WorkResult) workers.WorkResult {
	result.RecordedOutputWork = cloneFactoryWorkItems(result.RecordedOutputWork)
	result.FailureMetadata = workers.CloneWorkFailureMetadata(result.FailureMetadata)
	result.ProviderSession = workers.CloneProviderSessionMetadata(result.ProviderSession)
	result.Diagnostics = workers.CloneWorkDiagnostics(result.Diagnostics)
	return result
}

func cloneFactoryWorkItems(items []work.FactoryWorkItem) []work.FactoryWorkItem {
	if items == nil {
		return nil
	}
	cloned := make([]work.FactoryWorkItem, len(items))
	for index, item := range items {
		item.PreviousChainingTraceIDs = append([]string(nil), item.PreviousChainingTraceIDs...)
		item.Content = cloneWorkContentParts(item.Content)
		item.Tags = work.CloneTags(item.Tags)
		cloned[index] = item
	}
	return cloned
}

func cloneWorkContentParts(parts []work.WorkContentPart) []work.WorkContentPart {
	cloned := work.CloneWorkContentParts(parts)
	for index := range cloned {
		cloned[index].Metadata = cloneMetadataMap(parts[index].Metadata)
	}
	return cloned
}

func cloneMetadataMap(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = cloneMetadataValue(value)
	}
	return cloned
}

// Work content metadata is a JSON-shaped tree. Clone every reference-backed
// container accepted by that contract while preserving common typed values.
func cloneMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMetadataMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, entry := range typed {
			cloned[index] = cloneMetadataValue(entry)
		}
		return cloned
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, entry := range typed {
			cloned[key] = entry
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}
