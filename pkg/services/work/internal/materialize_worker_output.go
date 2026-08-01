package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/proposalmaterialization"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

// materializeWorkerOutput is the private application seam for Worker output.
// The public Work Service owns the published request/result vocabulary; this
// package owns the conversion to the canonical proposal implementation and
// the translation of its private failures back to that vocabulary.
func materializeWorkerOutput(
	ctx context.Context,
	request work.MaterializeWorkerOutputRequest,
) (work.MaterializeWorkerOutputResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return work.MaterializeWorkerOutputResult{}, err
		}
	}
	result, err := proposalmaterialization.Materialize(ctx, toInternalMaterializeRequest(request))
	if err != nil {
		return work.MaterializeWorkerOutputResult{}, mapProposalMaterializationError(err)
	}
	return fromInternalMaterializeResult(result), nil
}

func toInternalMaterializeRequest(
	request work.MaterializeWorkerOutputRequest,
) proposalmaterialization.Request {
	proposed := make([]proposalmaterialization.ProposedWorkItem, len(request.ProposedWork))
	for i, item := range request.ProposedWork {
		proposed[i] = proposalmaterialization.ProposedWorkItem{
			WorkTypeID: item.WorkTypeID,
			Name:       item.Name,
			State:      item.State,
			Content:    workContentPartsToAdmission(item.Content),
			Tags:       cloneStringMap(item.Tags),
			Relations:  relationsToAdmission(item.Relations),
		}
	}
	return proposalmaterialization.Request{
		Lineage: proposalmaterialization.LineageContext{
			DispatchID:               request.Lineage.DispatchID,
			RequestID:                request.Lineage.RequestID,
			SourceWorkIDs:            append([]string(nil), request.Lineage.SourceWorkIDs...),
			CurrentChainingTraceID:   request.Lineage.CurrentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), request.Lineage.PreviousChainingTraceIDs...),
			ChainingTraceDepth:       request.Lineage.ChainingTraceDepth,
			ParentWorkID:             request.Lineage.ParentWorkID,
			TraceID:                  request.Lineage.TraceID,
		},
		Primary:           workContentPartsToAdmission(request.Primary),
		Feedback:          request.Feedback,
		Classification:    request.Classification,
		ProposedWork:      proposed,
		ValidWorkTypes:    cloneBoolMap(request.ValidWorkTypes),
		ValidStatesByType: cloneNestedBoolMap(request.ValidStatesByType),
		DefaultWorkTypeID: request.DefaultWorkTypeID,
		IDGenerator:       proposalmaterialization.IDGenerator(request.IDGenerator),
	}
}

func fromInternalMaterializeResult(
	result proposalmaterialization.Result,
) work.MaterializeWorkerOutputResult {
	items := make([]work.FactoryWorkItem, len(result.MaterializedWork))
	for i, item := range result.MaterializedWork {
		items[i] = work.FactoryWorkItem{
			ID:                       item.ID,
			WorkTypeID:               item.WorkTypeID,
			State:                    item.State,
			DisplayName:              item.DisplayName,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   item.CurrentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
			TraceID:                  item.TraceID,
			Content:                  workContentPartsFromAdmission(item.Content),
			ParentID:                 item.ParentID,
			Tags:                     cloneStringMap(item.Tags),
		}
	}
	return work.MaterializeWorkerOutputResult{
		PrimaryOutput:    result.PrimaryOutput,
		Feedback:         result.Feedback,
		Classification:   result.Classification,
		MaterializedWork: items,
	}
}

func mapProposalMaterializationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, proposalmaterialization.ErrUnknownWorkType):
		return fmt.Errorf("%w: %w", work.ErrUnknownProposedWorkType, err)
	case errors.Is(err, proposalmaterialization.ErrInvalidProposal):
		return fmt.Errorf("%w: %w", work.ErrInvalidProposedWork, err)
	default:
		if strings.TrimSpace(err.Error()) == "" {
			return fmt.Errorf("%w", work.ErrInvalidProposedWork)
		}
		return fmt.Errorf("%w: %w", work.ErrInvalidProposedWork, err)
	}
}

func workContentPartsToAdmission(parts []work.WorkContentPart) []requestadmission.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]requestadmission.ContentPart, len(parts))
	for i, part := range parts {
		converted[i] = requestadmission.ContentPart{
			Type:        requestadmission.ContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func workContentPartsFromAdmission(parts []requestadmission.ContentPart) []work.WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]work.WorkContentPart, len(parts))
	for i, part := range parts {
		converted[i] = work.WorkContentPart{
			Type:        work.WorkContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func relationsToAdmission(relations []work.Relation) []requestadmission.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]requestadmission.Relation, len(relations))
	for i, relation := range relations {
		converted[i] = requestadmission.Relation{
			Type:          requestadmission.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkID,
			RequiredState: relation.RequiredState,
		}
	}
	return converted
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	clone := make(map[string]bool, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneNestedBoolMap(values map[string]map[string]bool) map[string]map[string]bool {
	if values == nil {
		return nil
	}
	clone := make(map[string]map[string]bool, len(values))
	for key, nested := range values {
		clone[key] = cloneBoolMap(nested)
	}
	return clone
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case []any:
		clone := make([]any, len(typed))
		for i, item := range typed {
			clone[i] = cloneAnyValue(item)
		}
		return clone
	case map[string]any:
		return cloneAnyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		clone := make(map[string]string, len(typed))
		for key, item := range typed {
			clone[key] = item
		}
		return clone
	default:
		return value
	}
}
