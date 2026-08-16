package work

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/proposalmaterialization"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

// MaterializeWorkerOutput applies Work-owned validation and identity
// assignment to Worker-proposed output. The root operation is intentionally
// free of session or Runtime implementation state so callers can inject it as
// the published Work materialization role.
func MaterializeWorkerOutput(
	ctx context.Context,
	request MaterializeWorkerOutputRequest,
) (MaterializeWorkerOutputResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return MaterializeWorkerOutputResult{}, err
		}
	}
	result, err := proposalmaterialization.Materialize(ctx, materializationRequest(request))
	if err != nil {
		return MaterializeWorkerOutputResult{}, mapMaterializationError(err)
	}
	return materializationResult(result), nil
}

func materializationRequest(request MaterializeWorkerOutputRequest) proposalmaterialization.Request {
	proposed := make([]proposalmaterialization.ProposedWorkItem, len(request.ProposedWork))
	for index, item := range request.ProposedWork {
		proposed[index] = proposalmaterialization.ProposedWorkItem{
			WorkTypeID: item.WorkTypeID,
			Name:       item.Name,
			State:      item.State,
			Content:    materializationContent(item.Content),
			Tags:       CloneTags(item.Tags),
			Relations:  materializationRelations(item.Relations),
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
		Primary:           materializationContent(request.Primary),
		Feedback:          request.Feedback,
		Classification:    request.Classification,
		ProposedWork:      proposed,
		ValidWorkTypes:    materializationBoolMap(request.ValidWorkTypes),
		ValidStatesByType: materializationNestedBoolMap(request.ValidStatesByType),
		DefaultWorkTypeID: request.DefaultWorkTypeID,
		IDGenerator:       proposalmaterialization.IDGenerator(request.IDGenerator),
	}
}

func materializationResult(result proposalmaterialization.Result) MaterializeWorkerOutputResult {
	items := make([]FactoryWorkItem, len(result.MaterializedWork))
	for index, item := range result.MaterializedWork {
		items[index] = FactoryWorkItem{
			ID:                       item.ID,
			WorkTypeID:               item.WorkTypeID,
			State:                    item.State,
			DisplayName:              item.DisplayName,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   item.CurrentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
			TraceID:                  item.TraceID,
			Content:                  materializationContentFromAdmission(item.Content),
			ParentID:                 item.ParentID,
			Tags:                     CloneTags(item.Tags),
		}
	}
	return MaterializeWorkerOutputResult{
		PrimaryOutput:    result.PrimaryOutput,
		Feedback:         result.Feedback,
		Classification:   result.Classification,
		MaterializedWork: items,
	}
}

func mapMaterializationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, proposalmaterialization.ErrUnknownWorkType):
		return fmt.Errorf("%w: %w", ErrUnknownProposedWorkType, err)
	case errors.Is(err, proposalmaterialization.ErrInvalidProposal):
		return fmt.Errorf("%w: %w", ErrInvalidProposedWork, err)
	default:
		if strings.TrimSpace(err.Error()) == "" {
			return fmt.Errorf("%w", ErrInvalidProposedWork)
		}
		return fmt.Errorf("%w: %w", ErrInvalidProposedWork, err)
	}
}

func materializationContent(parts []WorkContentPart) []requestadmission.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]requestadmission.ContentPart, len(parts))
	for index, part := range parts {
		converted[index] = requestadmission.ContentPart{
			Type:        requestadmission.ContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append([]byte(nil), part.JSON...),
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

func materializationContentFromAdmission(parts []requestadmission.ContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]WorkContentPart, len(parts))
	for index, part := range parts {
		converted[index] = WorkContentPart{
			Type:        WorkContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append([]byte(nil), part.JSON...),
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

func materializationRelations(relations []Relation) []requestadmission.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]requestadmission.Relation, len(relations))
	for index, relation := range relations {
		converted[index] = requestadmission.Relation{
			Type:          requestadmission.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkID,
			RequiredState: relation.RequiredState,
		}
	}
	return converted
}

func materializationBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	clone := make(map[string]bool, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func materializationNestedBoolMap(values map[string]map[string]bool) map[string]map[string]bool {
	if values == nil {
		return nil
	}
	clone := make(map[string]map[string]bool, len(values))
	for key, nested := range values {
		clone[key] = materializationBoolMap(nested)
	}
	return clone
}
