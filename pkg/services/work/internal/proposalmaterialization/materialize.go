// Package proposalmaterialization validates Worker-proposed output and assigns
// Work-owned canonical identity and lineage before Runtime applies effects.
package proposalmaterialization

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/lineagegraph"
)

var (
	// ErrInvalidProposal reports a proposal that failed Work validation.
	ErrInvalidProposal = errors.New("invalid proposed Work")
	// ErrUnknownWorkType reports a proposal Work type outside the catalog.
	ErrUnknownWorkType = errors.New("unknown proposed Work type")
)

// IDGenerator supplies opaque identity components for canonical Work IDs.
type IDGenerator func() string

// ProposedWorkItem is a non-canonical follow-up Work proposal.
type ProposedWorkItem struct {
	WorkTypeID string
	Name       string
	State      string
	Content    []requestadmission.ContentPart
	Tags       map[string]string
	Relations  []requestadmission.Relation
}

// LineageContext carries detached dispatch lineage facts.
type LineageContext struct {
	DispatchID               string
	RequestID                string
	SourceWorkIDs            []string
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	ChainingTraceDepth       int
	ParentWorkID             string
	TraceID                  string
}

// Request is the materialization input.
type Request struct {
	Lineage           LineageContext
	Primary           []requestadmission.ContentPart
	Feedback          string
	Classification    string
	ProposedWork      []ProposedWorkItem
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
	DefaultWorkTypeID string
	IDGenerator       IDGenerator
}

// MaterializedWorkItem is one canonical Work item after validation and identity
// assignment. ParentID is Work-owned lineage, not a Runtime token place.
type MaterializedWorkItem struct {
	ID                       string
	WorkTypeID               string
	State                    string
	DisplayName              string
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []requestadmission.ContentPart
	ParentID                 string
	Tags                     map[string]string
}

// Result is the Work-owned materialized output.
type Result struct {
	PrimaryOutput    string
	Feedback         string
	Classification   string
	MaterializedWork []MaterializedWorkItem
}

// Materialize validates proposals and assigns canonical Work identity/lineage.
func Materialize(ctx context.Context, request Request) (Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
	}

	primary, err := requestadmission.NewContentPreparation().PrepareWorkContent(ctx, request.Primary)
	if err != nil {
		return Result{}, fmt.Errorf("%w: primary content: %w", ErrInvalidProposal, err)
	}

	result := Result{
		PrimaryOutput:  primaryText(primary),
		Feedback:       strings.TrimSpace(request.Feedback),
		Classification: strings.TrimSpace(request.Classification),
	}
	if len(request.ProposedWork) == 0 {
		return result, nil
	}

	requestID := strings.TrimSpace(request.Lineage.RequestID)
	if requestID == "" {
		generated, genErr := generatedIdentity("request", request.IDGenerator)
		if genErr != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrInvalidProposal, genErr)
		}
		requestID = generated
	}

	traceID := strings.TrimSpace(request.Lineage.TraceID)
	if traceID == "" {
		traceID = strings.TrimSpace(request.Lineage.CurrentChainingTraceID)
	}
	if traceID == "" {
		traceID = "trace-" + requestID
	}
	currentChaining := requestadmission.ResolveWorkRequestCurrentChainingTraceID(
		request.Lineage.CurrentChainingTraceID,
		traceID,
	)
	previousChaining := lineagegraph.CanonicalChainingTraceIDs(request.Lineage.PreviousChainingTraceIDs)
	depth := request.Lineage.ChainingTraceDepth
	if depth <= 0 && currentChaining != "" {
		depth = 1
	}

	seenNames := make(map[string]struct{}, len(request.ProposedWork))
	materialized := make([]MaterializedWorkItem, 0, len(request.ProposedWork))
	for i, proposal := range request.ProposedWork {
		item, itemErr := materializeOne(
			ctx,
			i,
			proposal,
			request,
			requestID,
			traceID,
			currentChaining,
			previousChaining,
			depth,
			seenNames,
		)
		if itemErr != nil {
			return Result{}, itemErr
		}
		materialized = append(materialized, item)
	}
	result.MaterializedWork = materialized
	return result, nil
}

func materializeOne(
	ctx context.Context,
	index int,
	proposal ProposedWorkItem,
	request Request,
	requestID string,
	traceID string,
	currentChaining string,
	previousChaining []string,
	depth int,
	seenNames map[string]struct{},
) (MaterializedWorkItem, error) {
	name := strings.TrimSpace(proposal.Name)
	if name == "" {
		name = fmt.Sprintf("proposed-%d", index+1)
	}
	if _, exists := seenNames[name]; exists {
		return MaterializedWorkItem{}, fmt.Errorf(
			"%w: proposed work[%d] has duplicate name %q",
			ErrInvalidProposal,
			index,
			name,
		)
	}
	seenNames[name] = struct{}{}

	workTypeID := strings.TrimSpace(proposal.WorkTypeID)
	if workTypeID == "" {
		workTypeID = strings.TrimSpace(request.DefaultWorkTypeID)
	}
	if workTypeID == "" {
		return MaterializedWorkItem{}, fmt.Errorf(
			"%w: proposed work[%d] (%q) is missing work type",
			ErrInvalidProposal,
			index,
			name,
		)
	}
	if request.ValidWorkTypes != nil && !request.ValidWorkTypes[workTypeID] {
		return MaterializedWorkItem{}, fmt.Errorf(
			"%w: proposed work[%d] (%q) references unknown work type %q",
			ErrUnknownWorkType,
			index,
			name,
			workTypeID,
		)
	}
	state := strings.TrimSpace(proposal.State)
	if state != "" && request.ValidStatesByType != nil && !request.ValidStatesByType[workTypeID][state] {
		return MaterializedWorkItem{}, fmt.Errorf(
			"%w: proposed work[%d] (%q) references unknown state %q for work type %q",
			ErrInvalidProposal,
			index,
			name,
			state,
			workTypeID,
		)
	}

	content, err := requestadmission.NewContentPreparation().PrepareWorkContent(ctx, proposal.Content)
	if err != nil {
		return MaterializedWorkItem{}, fmt.Errorf(
			"%w: proposed work[%d] (%q) content: %w",
			ErrInvalidProposal,
			index,
			name,
			err,
		)
	}

	workID, err := generatedIdentity("work", request.IDGenerator)
	if err != nil {
		// Fall back to deterministic batch-style identity when no generator is
		// supplied, matching admission's request-scoped naming.
		workID = fmt.Sprintf("batch-%s-%s", requestID, name)
	}

	tags := maps.Clone(proposal.Tags)
	if tags == nil {
		tags = map[string]string{}
	}
	tags["_work_name"] = name
	tags["_work_type"] = workTypeID
	if dispatchID := strings.TrimSpace(request.Lineage.DispatchID); dispatchID != "" {
		tags["_source_dispatch_id"] = dispatchID
	}

	parentID := strings.TrimSpace(request.Lineage.ParentWorkID)
	if parentID == "" && len(request.Lineage.SourceWorkIDs) > 0 {
		parentID = strings.TrimSpace(request.Lineage.SourceWorkIDs[0])
	}

	return MaterializedWorkItem{
		ID:                       workID,
		WorkTypeID:               workTypeID,
		State:                    state,
		DisplayName:              name,
		ChainingTraceDepth:       depth,
		CurrentChainingTraceID:   currentChaining,
		PreviousChainingTraceIDs: append([]string(nil), previousChaining...),
		TraceID:                  traceID,
		Content:                  content,
		ParentID:                 parentID,
		Tags:                     tags,
	}, nil
}

func generatedIdentity(prefix string, generateID IDGenerator) (string, error) {
	if generateID == nil {
		return "", fmt.Errorf("ID generator is required")
	}
	identity := strings.TrimSpace(generateID())
	if identity == "" {
		return "", fmt.Errorf("ID generator returned an empty identity")
	}
	return prefix + "-" + identity, nil
}

func primaryText(parts []requestadmission.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type != requestadmission.ContentPartTypeText {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(text)
	}
	return builder.String()
}
