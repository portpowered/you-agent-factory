// Package workstationprojection maps the Recordings-owned workstation request
// read model into generated OpenAPI response types.
package workstationprojection

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
)

// Generated maps one canonical Recordings projection into its OpenAPI shape.
func Generated(
	projection recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	if projection.WorkstationRequestsByDispatchId == nil {
		return factoryapi.FactoryWorldWorkstationRequestProjectionSlice{}
	}
	source := *projection.WorkstationRequestsByDispatchId
	generated := make(map[string]factoryapi.FactoryWorldWorkstationRequestView, len(source))
	for dispatchID, view := range source {
		generated[dispatchID] = generatedRequestView(view)
	}
	if len(generated) == 0 {
		return factoryapi.FactoryWorldWorkstationRequestProjectionSlice{}
	}
	return factoryapi.FactoryWorldWorkstationRequestProjectionSlice{
		WorkstationRequestsByDispatchId: &generated,
	}
}

func generatedRequestView(
	view recordings.WorkstationFactoryWorldWorkstationRequestView,
) factoryapi.FactoryWorldWorkstationRequestView {
	return factoryapi.FactoryWorldWorkstationRequestView{
		Counts: factoryapi.FactoryWorldWorkstationRequestCountView{
			DispatchedCount: view.Counts.DispatchedCount,
			ErroredCount:    view.Counts.ErroredCount,
			RespondedCount:  view.Counts.RespondedCount,
		},
		DispatchId:      view.DispatchId,
		Request:         generatedRequest(view.Request),
		Response:        generatedResponse(view.Response),
		TransitionId:    view.TransitionId,
		WorkstationName: copyString(view.WorkstationName),
	}
}

func generatedRequest(
	view recordings.WorkstationFactoryWorldWorkstationRequestRequestView,
) factoryapi.FactoryWorldWorkstationRequestRequestView {
	return factoryapi.FactoryWorldWorkstationRequestRequestView{
		ConsumedTokens:           generatedTokens(view.ConsumedTokens),
		CurrentChainingTraceId:   copyString(view.CurrentChainingTraceId),
		InputWorkItems:           generatedWorkItems(view.InputWorkItems),
		InputWorkTypeIds:         copyStrings(view.InputWorkTypeIds),
		PreviousChainingTraceIds: copyStrings(view.PreviousChainingTraceIds),
		Runner:                   generatedRunner(view.Runner),
		ScriptRequest:            generatedScriptRequest(view.ScriptRequest),
		StartedAt:                view.StartedAt,
		TraceIds:                 copyStrings(view.TraceIds),
	}
}

func generatedResponse(
	view *recordings.WorkstationFactoryWorldWorkstationRequestResponseView,
) *factoryapi.FactoryWorldWorkstationRequestResponseView {
	if view == nil {
		return nil
	}
	var failureDetail *factoryapi.FailureDetail
	if view.FailureDetail != nil {
		failureDetail = &factoryapi.FailureDetail{
			Reason:  factoryapi.WorkFailureType(view.FailureDetail.Reason),
			Message: view.FailureDetail.Message,
		}
	}
	return &factoryapi.FactoryWorldWorkstationRequestResponseView{
		AgentRunInspection:          workerdiagnosticsmapping.GeneratedFactoryWorldAgentRunInspectionView(view.AgentRunInspection),
		DurationMillis:              view.DurationMillis,
		EndTime:                     view.EndTime,
		FailureDetail:               failureDetail,
		Feedback:                    copyString(view.Feedback),
		Outcome:                     copyString(view.Outcome),
		OutputMutations:             generatedMutations(view.OutputMutations),
		OutputWorkItems:             generatedWorkItems(view.OutputWorkItems),
		Runner:                      generatedRunner(view.Runner),
		ScriptResponse:              generatedScriptResponse(view.ScriptResponse),
		SelectedClassificationLabel: copyString(view.SelectedClassificationLabel),
	}
}

func generatedRunner(
	view *recordings.WorkstationFactoryWorldSelectedRunnerView,
) *factoryapi.FactoryWorldSelectedRunnerView {
	if view == nil {
		return nil
	}
	result := &factoryapi.FactoryWorldSelectedRunnerView{
		DisplayName: copyString(view.DisplayName),
	}
	if view.RunnerId != nil {
		value := factoryapi.RunnerID(*view.RunnerId)
		result.RunnerId = &value
	}
	if view.SelectionSource != nil {
		value := factoryapi.RunnerSelectionSource(*view.SelectionSource)
		result.SelectionSource = &value
	}
	if view.Capabilities != nil {
		baseline := make(
			[]factoryapi.FactoryWorldRunnerBaselineCapability,
			len(view.Capabilities.BaselineCapabilities),
		)
		for i, capability := range view.Capabilities.BaselineCapabilities {
			baseline[i] = factoryapi.FactoryWorldRunnerBaselineCapability(capability)
		}
		optional := make(
			[]factoryapi.FactoryWorldRunnerOptionalCapabilitySupportView,
			len(view.Capabilities.OptionalCapabilities),
		)
		for i, support := range view.Capabilities.OptionalCapabilities {
			optional[i] = factoryapi.FactoryWorldRunnerOptionalCapabilitySupportView{
				Capability: factoryapi.FactoryWorldRunnerOptionalCapability(support.Capability),
				Detail:     copyString(support.Detail),
				Status:     factoryapi.FactoryWorldRunnerOptionalCapabilityStatus(support.Status),
			}
		}
		result.Capabilities = &factoryapi.FactoryWorldRunnerCapabilitiesView{
			BaselineCapabilities: baseline,
			OptionalCapabilities: optional,
		}
	}
	return result
}

func generatedWorkItems(
	views *[]recordings.WorkstationFactoryWorldWorkItemRef,
) *[]factoryapi.FactoryWorldWorkItemRef {
	if views == nil {
		return nil
	}
	result := make([]factoryapi.FactoryWorldWorkItemRef, 0, len(*views))
	for _, view := range *views {
		result = append(result, factoryapi.FactoryWorldWorkItemRef{
			ChainingTraceDepth:       view.ChainingTraceDepth,
			Content:                  contentmapping.GeneratedPtrFromParts(valueOrNil(view.Content)),
			CurrentChainingTraceId:   copyString(view.CurrentChainingTraceId),
			DisplayName:              copyString(view.DisplayName),
			LineageContinuity:        lineageContinuity(view.LineageContinuity),
			LineageLogicalWorkId:     copyString(view.LineageLogicalWorkId),
			LineageParentWorkIds:     copyStrings(view.LineageParentWorkIds),
			LineageSourceKind:        lineageSourceKind(view.LineageSourceKind),
			PayloadStatus:            payloadStatus(view.PayloadStatus),
			PayloadUnavailableReason: copyString(view.PayloadUnavailableReason),
			PreviousChainingTraceIds: copyStrings(view.PreviousChainingTraceIds),
			State:                    copyString(view.State),
			TraceId:                  copyString(view.TraceId),
			WorkId:                   view.WorkId,
			WorkTypeId:               copyString(view.WorkTypeId),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return &result
}

func generatedTokens(
	views *[]recordings.WorkstationFactoryWorldTokenView,
) *[]factoryapi.FactoryWorldTokenView {
	if views == nil {
		return nil
	}
	result := make([]factoryapi.FactoryWorldTokenView, 0, len(*views))
	for _, view := range *views {
		result = append(result, generatedToken(view))
	}
	if len(result) == 0 {
		return nil
	}
	return &result
}

func generatedToken(
	view recordings.WorkstationFactoryWorldTokenView,
) factoryapi.FactoryWorldTokenView {
	var tags *factoryapi.StringMap
	if view.Tags != nil {
		cloned := make(factoryapi.StringMap, len(*view.Tags))
		for key, value := range *view.Tags {
			cloned[key] = value
		}
		tags = &cloned
	}
	return factoryapi.FactoryWorldTokenView{
		ChainingTraceDepth:       view.ChainingTraceDepth,
		CurrentChainingTraceId:   copyString(view.CurrentChainingTraceId),
		Name:                     copyString(view.Name),
		PlaceId:                  view.PlaceId,
		PreviousChainingTraceIds: copyStrings(view.PreviousChainingTraceIds),
		Tags:                     tags,
		TokenId:                  view.TokenId,
		TraceId:                  copyString(view.TraceId),
		WorkId:                   copyString(view.WorkId),
		WorkTypeId:               copyString(view.WorkTypeId),
	}
}

func generatedMutations(
	views *[]recordings.WorkstationFactoryWorldMutationView,
) *[]factoryapi.FactoryWorldMutationView {
	if views == nil {
		return nil
	}
	result := make([]factoryapi.FactoryWorldMutationView, 0, len(*views))
	for _, view := range *views {
		var token *factoryapi.FactoryWorldTokenView
		if view.Token != nil {
			generated := generatedToken(*view.Token)
			token = &generated
		}
		result = append(result, factoryapi.FactoryWorldMutationView{
			FromPlace: copyString(view.FromPlace),
			Reason:    copyString(view.Reason),
			ToPlace:   copyString(view.ToPlace),
			Token:     token,
			TokenId:   view.TokenId,
			Type:      view.Type,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return &result
}

func generatedScriptRequest(
	view *recordings.WorkstationFactoryWorldScriptRequestView,
) *factoryapi.FactoryWorldScriptRequestView {
	if view == nil {
		return nil
	}
	return &factoryapi.FactoryWorldScriptRequestView{
		Args:            copyStrings(view.Args),
		Attempt:         view.Attempt,
		Command:         copyString(view.Command),
		ScriptRequestId: copyString(view.ScriptRequestId),
	}
}

func generatedScriptResponse(
	view *recordings.WorkstationFactoryWorldScriptResponseView,
) *factoryapi.FactoryWorldScriptResponseView {
	if view == nil {
		return nil
	}
	return &factoryapi.FactoryWorldScriptResponseView{
		Attempt:         view.Attempt,
		DurationMillis:  view.DurationMillis,
		ExitCode:        view.ExitCode,
		FailureType:     copyString(view.FailureType),
		Outcome:         copyString(view.Outcome),
		ScriptRequestId: copyString(view.ScriptRequestId),
		Stderr:          copyString(view.Stderr),
		Stdout:          copyString(view.Stdout),
	}
}

func lineageContinuity(
	value *recordings.WorkstationFactoryWorldWorkItemRefLineageContinuity,
) *factoryapi.FactoryWorldWorkItemRefLineageContinuity {
	if value == nil {
		return nil
	}
	converted := factoryapi.FactoryWorldWorkItemRefLineageContinuity(*value)
	return &converted
}

func lineageSourceKind(
	value *recordings.WorkstationFactoryWorldWorkItemRefLineageSourceKind,
) *factoryapi.FactoryWorldWorkItemRefLineageSourceKind {
	if value == nil {
		return nil
	}
	converted := factoryapi.FactoryWorldWorkItemRefLineageSourceKind(*value)
	return &converted
}

func payloadStatus(
	value *recordings.WorkstationFactoryWorldWorkItemRefPayloadStatus,
) *factoryapi.FactoryWorldWorkItemRefPayloadStatus {
	if value == nil {
		return nil
	}
	converted := factoryapi.FactoryWorldWorkItemRefPayloadStatus(*value)
	return &converted
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func copyStrings(values *[]string) *[]string {
	if values == nil {
		return nil
	}
	cloned := append([]string(nil), (*values)...)
	if len(cloned) == 0 {
		return nil
	}
	return &cloned
}

func valueOrNil[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}
