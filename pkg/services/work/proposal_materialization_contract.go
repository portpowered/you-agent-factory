package work

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/proposalmaterialization"
)

// Proposal materialization typed failures peers can distinguish on the root
// Service worker-output materialization slice (MaterializeWorkerOutput).
// Implementations may wrap these sentinels; peers should branch with errors.Is.
var (
	// ErrInvalidProposedWork reports that a Worker proposal failed Work-owned
	// validation before any canonical identity was assigned.
	ErrInvalidProposedWork = errors.New("invalid proposed Work")

	// ErrUnknownProposedWorkType reports that a proposal referenced a Work type
	// outside the Runtime-supplied catalog.
	ErrUnknownProposedWorkType = errors.New("unknown proposed Work type")
)

// ProposedWorkItem is a non-canonical follow-up Work proposal. It must not carry
// Runtime token identity; Work assigns canonical IDs and lineage.
type ProposedWorkItem struct {
	WorkTypeID string
	Name       string
	State      string
	Content    []WorkContentPart
	Tags       map[string]string
	Relations  []Relation
}

// MaterializationLineageContext carries detached dispatch lineage facts Runtime
// supplies so Work can assign request linkage and chaining without importing
// Runtime token vocabulary.
type MaterializationLineageContext struct {
	DispatchID               string
	RequestID                string
	SourceWorkIDs            []string
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	ChainingTraceDepth       int
	ParentWorkID             string
	TraceID                  string
}

// MaterializeWorkerOutputRequest is the Work-owned input for materializing
// Worker-proposed output into canonical Factory Work items.
type MaterializeWorkerOutputRequest struct {
	Lineage           MaterializationLineageContext
	Primary           []WorkContentPart
	Feedback          string
	Classification    string
	ProposedWork      []ProposedWorkItem
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
	DefaultWorkTypeID string
	IDGenerator       RequestIDGenerator
}

// MaterializeWorkerOutputResult carries Work-owned canonical output after
// validation and identity assignment.
type MaterializeWorkerOutputResult struct {
	PrimaryOutput    string
	Feedback         string
	Classification   string
	MaterializedWork []FactoryWorkItem
}

// MaterializeWorkerOutput validates proposed Worker output and assigns
// Work-owned canonical identity and lineage. Peers may call this package
// function directly or through Service.MaterializeWorkerOutput.
func MaterializeWorkerOutput(
	ctx context.Context,
	request MaterializeWorkerOutputRequest,
) (MaterializeWorkerOutputResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return MaterializeWorkerOutputResult{}, err
		}
	}
	result, err := proposalmaterialization.Materialize(ctx, toInternalMaterializeRequest(request))
	if err != nil {
		return MaterializeWorkerOutputResult{}, mapProposalMaterializationError(err)
	}
	return fromInternalMaterializeResult(result), nil
}

func toInternalMaterializeRequest(
	request MaterializeWorkerOutputRequest,
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
		ValidWorkTypes:    request.ValidWorkTypes,
		ValidStatesByType: request.ValidStatesByType,
		DefaultWorkTypeID: request.DefaultWorkTypeID,
		IDGenerator:       proposalmaterialization.IDGenerator(request.IDGenerator),
	}
}

func fromInternalMaterializeResult(
	result proposalmaterialization.Result,
) MaterializeWorkerOutputResult {
	items := make([]FactoryWorkItem, len(result.MaterializedWork))
	for i, item := range result.MaterializedWork {
		items[i] = FactoryWorkItem{
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
	return MaterializeWorkerOutputResult{
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

// Clone returns a detached ProposedWorkItem copy.
func (item ProposedWorkItem) Clone() ProposedWorkItem {
	clone := item
	clone.Content = CloneWorkContentParts(item.Content)
	clone.Tags = cloneStringMap(item.Tags)
	if len(item.Relations) > 0 {
		clone.Relations = append([]Relation(nil), item.Relations...)
	}
	return clone
}

// Clone returns a detached MaterializeWorkerOutputRequest copy.
func (request MaterializeWorkerOutputRequest) Clone() MaterializeWorkerOutputRequest {
	clone := request
	clone.Lineage.SourceWorkIDs = append([]string(nil), request.Lineage.SourceWorkIDs...)
	clone.Lineage.PreviousChainingTraceIDs = append([]string(nil), request.Lineage.PreviousChainingTraceIDs...)
	clone.Primary = CloneWorkContentParts(request.Primary)
	if len(request.ProposedWork) > 0 {
		clone.ProposedWork = make([]ProposedWorkItem, len(request.ProposedWork))
		for i, item := range request.ProposedWork {
			clone.ProposedWork[i] = item.Clone()
		}
	}
	if request.ValidWorkTypes != nil {
		clone.ValidWorkTypes = make(map[string]bool, len(request.ValidWorkTypes))
		for key, value := range request.ValidWorkTypes {
			clone.ValidWorkTypes[key] = value
		}
	}
	if request.ValidStatesByType != nil {
		clone.ValidStatesByType = make(map[string]map[string]bool, len(request.ValidStatesByType))
		for workType, states := range request.ValidStatesByType {
			clone.ValidStatesByType[workType] = make(map[string]bool, len(states))
			for state, ok := range states {
				clone.ValidStatesByType[workType][state] = ok
			}
		}
	}
	return clone
}
