package http

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"
)

// ListOptionsFromAPI maps one public list-work request into validated Work root
// list options.
func ListOptionsFromAPI(params factoryapi.ListWorkBySessionIdParams) (work.ListOptions, error) {
	options := work.ListOptions{
		StateName:    optional.StringValue(params.StateName),
		StateType:    listParamString(params.StateType),
		Name:         optional.StringValue(params.Name),
		WorkTypeName: optional.StringValue(params.WorkTypeName),
		TraceID:      optional.StringValue(params.TraceId),
		SortBy:       listParamString(params.SortBy),
		MaxResults:   optional.IntValue(params.MaxResults),
		NextToken:    optional.StringValue(params.NextToken),
	}
	query, err := work.NormalizeList(options)
	if err != nil {
		return work.ListOptions{}, err
	}
	return query.Options(), nil
}

func listParamString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

// ListWorkResponseToAPI encodes detached Work list results into the public HTTP
// success shape.
func ListWorkResponseToAPI(result work.ListResult) factoryapi.ListWorkResponse {
	results := make([]factoryapi.Work, 0, len(result.Results))
	for _, item := range result.Results {
		results = append(results, WorkReadModelToAPI(item))
	}
	response := factoryapi.ListWorkResponse{
		Results: results,
		PaginationContext: &factoryapi.PaginationContext{
			MaxResults: result.MaxResults,
		},
	}
	if result.NextToken != "" {
		response.PaginationContext.NextToken = &result.NextToken
	}
	return response
}

// WorkReadModelToAPI maps one detached Work read model into the generated HTTP
// representation.
func WorkReadModelToAPI(item work.ReadModel) factoryapi.Work {
	result := factoryapi.Work{
		Name:                     item.Name,
		WorkId:                   optional.NonEmptyStringPtr(item.WorkID),
		WorkTypeName:             optional.NonEmptyStringPtr(item.WorkTypeName),
		ChainingTraceDepth:       optional.PositiveIntPtr(item.ChainingTraceDepth),
		CurrentChainingTraceId:   optional.NonEmptyStringPtr(item.CurrentChainingTraceID),
		PreviousChainingTraceIds: optional.CopiedStringsPtr(item.PreviousChainingTraceIDs),
		TraceId:                  optional.NonEmptyStringPtr(item.TraceID),
		Content:                  contentcontract.GeneratedPtrFromParts(item.Content),
		Tags:                     optional.CopiedStringMapPtr(item.Tags),
		StopSummary:              workStopSummaryToAPI(item.StopSummary),
	}
	if item.State != nil {
		result.State = &factoryapi.WorkState{
			Name: item.State.Name,
			Type: factoryapi.WorkStateType(item.State.Type),
		}
	}
	if len(item.Relations) > 0 {
		relations := make([]factoryapi.Relation, 0, len(item.Relations))
		for _, relation := range item.Relations {
			relations = append(relations, factoryapi.Relation{
				Type:           factoryapi.RelationType(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				TargetWorkId:   optional.NonEmptyStringPtr(relation.TargetWorkID),
				RequiredState:  optional.NonEmptyStringPtr(relation.RequiredState),
			})
		}
		result.Relations = &relations
	}
	return result
}

func workStopSummaryToAPI(summary *work.StopSummary) *factoryapi.FactoryStopSummary {
	if summary == nil {
		return nil
	}
	result := &factoryapi.FactoryStopSummary{
		SessionId:                summary.SessionID,
		StopKind:                 factoryapi.FactoryStopKind(summary.StopKind),
		WorkId:                   summary.WorkID,
		WorkName:                 summary.WorkName,
		WorkTypeName:             summary.WorkTypeName,
		WorkState:                summary.WorkState,
		LatestResultSummary:      summary.LatestResultSummary,
		SuggestedRecoverySurface: summary.SuggestedRecoverySurface,
		SuggestedRecoveryAction:  summary.SuggestedRecoveryAction,
	}
	if summary.SessionLifecycleStatus != nil {
		status := factoryapi.FactorySessionDurableLifecycleStatus(*summary.SessionLifecycleStatus)
		result.SessionLifecycleStatus = &status
	}
	if summary.LatestDispatch != nil {
		result.LatestDispatch = &factoryapi.FactoryStopDispatchSummary{
			DispatchId:      summary.LatestDispatch.DispatchID,
			Status:          factoryapi.FactoryDispatchStatus(summary.LatestDispatch.Status),
			DispatchKind:    factoryapi.FactoryDispatchKind(summary.LatestDispatch.DispatchKind),
			WorkstationName: summary.LatestDispatch.WorkstationName,
		}
		if summary.LatestDispatch.FailureDetail != nil {
			result.LatestDispatch.FailureDetail = &factoryapi.FailureDetail{
				Reason:  factoryapi.WorkFailureType(summary.LatestDispatch.FailureDetail.Reason),
				Message: summary.LatestDispatch.FailureDetail.Message,
			}
		}
	}
	return result
}
