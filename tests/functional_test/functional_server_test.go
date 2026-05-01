package functional_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/api"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/apisurface"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/service"
	"go.uber.org/zap"
)

// FunctionalServer starts the agent-factory with a real HTTP API server backed
// by the service layer. It allows functional tests to interact via HTTP rather
// than constructing internal engine or subsystem structs directly.
//
// # How to add a new test scenario
//
//  1. Scaffold a factory dir:  dir := scaffoldFactory(t, simplePipelineConfig())
//  2. Start the server:        fs := StartFunctionalServer(t, dir, true /* use mock workers */)
//  3. Submit work:             traceID := fs.SubmitWork(t, "task", json.RawMessage(`{...}`))
//  4. Poll runtime state:      state := fs.GetState(t)
//  5. Assert results:          check state.Categories.Terminal, state.TotalTokens, etc.
//
// The server shuts down automatically via t.Cleanup — no manual teardown needed.
// Use mock workers for happy-path tests that should complete without live providers.
// Use real executors when you need provider overrides, failure routing, or custom outcomes.
type FunctionalServer struct {
	httpSrv *httptest.Server
	factory factory.APIFactory
	service *service.FactoryService
	cancel  context.CancelFunc
	done    chan struct{}
}

type DashboardStream struct {
	t         *testing.T
	server    *FunctionalServer
	resp      *http.Response
	cancel    context.CancelFunc
	done      chan struct{}
	snapshots chan DashboardResponse
	errs      chan error
}

// SubmitWork POSTs a work item to POST /work and returns the assigned trace ID.
func (fs *FunctionalServer) SubmitWork(t *testing.T, workTypeID string, payload json.RawMessage) string {
	t.Helper()
	req := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeID,
		Payload:      payload,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	resp, err := http.Post(fs.httpSrv.URL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work: expected 201 Created, got %d", resp.StatusCode)
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	return result.TraceId
}

// SubmitRuntimeWork submits work directly to the running factory and returns
// the normalized requests so tests can assert the assigned trace IDs.
func (fs *FunctionalServer) SubmitRuntimeWork(t *testing.T, requests ...interfaces.SubmitRequest) []interfaces.SubmitRequest {
	t.Helper()

	normalized := normalizeSubmitRequestsForFunctionalTest(requests)
	workRequest := workRequestFromSubmitRequests(normalized)
	if _, err := fs.factory.SubmitWorkRequest(context.Background(), workRequest); err != nil {
		t.Fatalf("factory.SubmitWorkRequest: %v", err)
	}
	return normalized
}

// GetState fetches the service engine snapshot and maps it into the legacy
// compatibility response shape.
func (fs *FunctionalServer) GetState(t *testing.T) StateResponse {
	t.Helper()
	snapshot := fs.GetEngineStateSnapshot(t)
	cats, resources := categorizeTokensForFunctionalState(&snapshot.Marking, snapshot.Topology)

	return StateResponse{
		FactoryState:  snapshot.FactoryState,
		RuntimeStatus: string(snapshot.RuntimeStatus),
		TotalTokens:   len(snapshot.Marking.Tokens),
		Categories:    cats,
		Resources:     resourceUsageSlicePtrForFunctionalState(resources),
	}
}

// GetDashboard projects the current runtime into the compatibility dashboard shape.
func (fs *FunctionalServer) GetDashboard(t *testing.T) DashboardResponse {
	t.Helper()

	snapshot := fs.GetEngineStateSnapshot(t)
	events, err := fs.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("get factory events: %v", err)
	}

	worldState, err := projections.ReconstructFactoryWorldState(events, snapshot.TickCount)
	if err != nil {
		t.Fatalf("reconstruct world state: %v", err)
	}
	worldView := projections.BuildFactoryWorldViewWithActiveThrottlePauses(worldState, snapshot.ActiveThrottlePauses)

	_, resources := categorizeTokensForFunctionalState(&snapshot.Marking, snapshot.Topology)
	payload := struct {
		FactoryState  string                              `json:"factory_state"`
		RuntimeStatus string                              `json:"runtime_status"`
		TickCount     int                                 `json:"tick_count"`
		UptimeSeconds int64                               `json:"uptime_seconds"`
		Resources     *[]ResourceUsage                    `json:"resources,omitempty"`
		Topology      interfaces.FactoryWorldTopologyView `json:"topology"`
		Runtime       DashboardRuntime                    `json:"runtime"`
	}{
		FactoryState:  snapshot.FactoryState,
		RuntimeStatus: string(snapshot.RuntimeStatus),
		TickCount:     snapshot.TickCount,
		UptimeSeconds: int64(snapshot.Uptime / time.Second),
		Resources:     resourceUsageSlicePtrForFunctionalState(resources),
		Topology:      worldView.Topology,
		Runtime:       dashboardRuntimeFromWorldView(worldView),
	}

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal dashboard payload: %v", err)
	}
	var dashboard DashboardResponse
	if err := json.Unmarshal(b, &dashboard); err != nil {
		t.Fatalf("decode dashboard payload: %v", err)
	}
	return dashboard
}

func dashboardRuntimeFromWorldView(worldView interfaces.FactoryWorldView) DashboardRuntime {
	runtime := worldView.Runtime
	return DashboardRuntime{
		ActiveDispatchIds:             stringSlicePtr(runtime.ActiveDispatchIDs),
		ActiveExecutionsByDispatchId:  dashboardActiveExecutionsByDispatchID(runtime.ActiveExecutionsByDispatchID),
		ActiveThrottlePauses:          dashboardThrottlePauses(runtime.ActiveThrottlePauses),
		ActiveWorkstationNodeIds:      stringSlicePtr(runtime.ActiveWorkstationNodeIDs),
		CurrentWorkItemsByPlaceId:     dashboardWorkItemsByPlaceID(runtime.CurrentWorkItemsByPlaceID),
		InFlightDispatchCount:         runtime.InFlightDispatchCount,
		InferenceAttemptsByDispatchId: dashboardInferenceAttemptsByDispatchID(runtime.InferenceAttemptsByDispatchID),
		PlaceTokenCounts:              integerMapPtrForFunctionalDashboard(runtime.PlaceTokenCounts),
		Session:                       dashboardSessionRuntimeFromWorldView(worldView),
		WorkstationActivityByNodeId:   dashboardWorkstationActivityByNodeID(runtime.WorkstationActivityByNodeID),
	}
}

func dashboardActiveExecutionsByDispatchID(input map[string]interfaces.FactoryWorldActiveExecution) *map[string]DashboardActiveExecution {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]DashboardActiveExecution, len(input))
	for dispatchID, execution := range input {
		workstationName := stringPtrIfNotEmptyForFunctionalDashboard(execution.WorkstationName)
		out[dispatchID] = DashboardActiveExecution{
			ConsumedTokens:    dashboardTraceTokensFromInputs(execution.ConsumedInputs, execution.StartedAt),
			DispatchId:        execution.DispatchID,
			StartedAt:         execution.StartedAt,
			TraceIds:          stringSlicePtr(execution.TraceIDs),
			TransitionId:      execution.TransitionID,
			WorkItems:         dashboardWorkItemRefs(execution.WorkItems),
			WorkTypeIds:       stringSlicePtr(execution.WorkTypeIDs),
			WorkstationName:   workstationName,
			WorkstationNodeId: execution.WorkstationNodeID,
		}
	}
	return &out
}

func dashboardThrottlePauses(input []interfaces.FactoryWorldThrottlePause) *[]DashboardThrottlePause {
	if len(input) == 0 {
		return nil
	}
	out := make([]DashboardThrottlePause, 0, len(input))
	for _, pause := range input {
		out = append(out, DashboardThrottlePause{
			AffectedTransitionIds:    stringSlicePtr(pause.AffectedTransitionIDs),
			AffectedWorkTypeIds:      stringSlicePtr(pause.AffectedWorkTypeIDs),
			AffectedWorkerTypes:      stringSlicePtr(pause.AffectedWorkerTypes),
			AffectedWorkstationNames: stringSlicePtr(pause.AffectedWorkstationNames),
			LaneId:                   pause.LaneID,
			Model:                    pause.Model,
			PausedAt:                 timePtrIfNotZero(pause.PausedAt),
			PausedUntil:              pause.PausedUntil,
			Provider:                 pause.Provider,
			RecoverAt:                pause.RecoverAt,
		})
	}
	return &out
}

func dashboardWorkItemsByPlaceID(input map[string][]interfaces.FactoryWorldWorkItemRef) *map[string][]DashboardWorkItemRef {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]DashboardWorkItemRef, len(input))
	for placeID, items := range input {
		out[placeID] = dashboardWorkItemRefsValue(items)
	}
	return &out
}

func dashboardInferenceAttemptsByDispatchID(input map[string]map[string]interfaces.FactoryWorldInferenceAttempt) *map[string]map[string]InferenceAttempt {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]map[string]InferenceAttempt, len(input))
	for dispatchID, attempts := range input {
		if len(attempts) == 0 {
			continue
		}
		converted := make(map[string]InferenceAttempt, len(attempts))
		for requestID, attempt := range attempts {
			converted[requestID] = InferenceAttempt{
				Attempt:            attempt.Attempt,
				DispatchId:         attempt.DispatchID,
				DurationMillis:     attempt.DurationMillis,
				ErrorClass:         attempt.ErrorClass,
				ExitCode:           copyIntPtrForFunctionalDashboard(attempt.ExitCode),
				InferenceRequestId: attempt.InferenceRequestID,
				Outcome:            attempt.Outcome,
				Prompt:             attempt.Prompt,
				RequestTime:        dashboardTimeString(attempt.RequestTime),
				Response:           attempt.Response,
				ResponseTime:       dashboardTimeString(attempt.ResponseTime),
				TransitionId:       attempt.TransitionID,
				WorkingDirectory:   attempt.WorkingDirectory,
				Worktree:           attempt.Worktree,
			}
		}
		out[dispatchID] = converted
	}
	return &out
}

func dashboardSessionRuntimeFromWorldView(worldView interfaces.FactoryWorldView) DashboardSessionRuntime {
	session := worldView.Runtime.Session
	return DashboardSessionRuntime{
		CompletedByWorkType:  integerMapPtrForFunctionalDashboard(session.CompletedByWorkType),
		CompletedCount:       session.CompletedCount,
		CompletedWorkLabels:  functionalDashboardSessionWorkLabels(worldView, "TERMINAL"),
		DispatchHistory:      dashboardDispatchHistory(session.DispatchHistory),
		DispatchedByWorkType: integerMapPtrForFunctionalDashboard(session.DispatchedByWorkType),
		DispatchedCount:      session.DispatchedCount,
		FailedByWorkType:     integerMapPtrForFunctionalDashboard(session.FailedByWorkType),
		FailedCount:          session.FailedCount,
		FailedWorkLabels:     functionalDashboardSessionWorkLabels(worldView, "FAILED"),
		HasData:              session.HasData,
		ProviderSessions:     dashboardProviderSessions(session.ProviderSessions),
	}
}

func functionalDashboardSessionWorkLabels(
	worldView interfaces.FactoryWorldView,
	category string,
) *[]string {
	placeCategories := make(map[string]string)
	for _, node := range worldView.Topology.WorkstationNodesByID {
		for _, place := range node.InputPlaces {
			if place.PlaceID != "" && place.StateCategory != "" {
				placeCategories[place.PlaceID] = place.StateCategory
			}
		}
		for _, place := range node.OutputPlaces {
			if place.PlaceID != "" && place.StateCategory != "" {
				placeCategories[place.PlaceID] = place.StateCategory
			}
		}
	}
	workItemsByID := make(map[string]interfaces.FactoryWorldWorkItemRef)
	for placeID, workItems := range worldView.Runtime.PlaceOccupancyWorkItemsByPlaceID {
		if placeCategories[placeID] != category {
			continue
		}
		for _, workItem := range workItems {
			if workItem.WorkID == "" {
				continue
			}
			workItemsByID[workItem.WorkID] = workItem
		}
	}
	if len(workItemsByID) == 0 {
		return nil
	}
	labels := make([]string, 0, len(workItemsByID))
	for _, workItem := range workItemsByID {
		label := workItem.WorkID
		if workItem.DisplayName != "" {
			label = workItem.DisplayName
		}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return stringSlicePtr(labels)
}

func dashboardDispatchHistory(input []interfaces.FactoryWorldDispatchCompletion) *[]DashboardDispatchView {
	if len(input) == 0 {
		return nil
	}
	out := make([]DashboardDispatchView, 0, len(input))
	for _, dispatch := range input {
		workItems := dashboardDispatchWorkItems(dispatch)
		workTypeIDs := dashboardDispatchWorkTypeIDs(dispatch, workItems)
		out = append(out, DashboardDispatchView{
			ConsumedTokens:  dashboardTraceTokensFromInputs(dispatch.ConsumedInputs, dispatch.StartedAt),
			DispatchId:      dispatch.DispatchID,
			DurationMillis:  dispatch.DurationMillis,
			EndTime:         dashboardTimeString(dispatch.CompletedAt),
			Outcome:         dispatch.Result.Outcome,
			OutputMutations: dashboardTraceMutationsFromCompletion(dispatch),
			ProviderSession: dashboardProviderSessionMetadata(dispatch.ProviderSession),
			StartedAt:       dashboardTimeString(dispatch.StartedAt),
			TraceIds:        stringSlicePtr(dispatch.TraceIDs),
			TransitionId:    dashboardCompatTransitionID(dispatch.TransitionID),
			WorkItems:       dashboardWorkItemRefs(workItems),
			WorkTypeIds:     stringSlicePtr(workTypeIDs),
			WorkstationName: stringPtrIfNotEmptyForFunctionalDashboard(dashboardCompatWorkstationName(dispatch.Workstation.Name, dispatch.TransitionID)),
		})
	}
	return &out
}

func dashboardProviderSessions(input []interfaces.FactoryWorldProviderSessionRecord) *[]ProviderSessionAttempt {
	if len(input) == 0 {
		return nil
	}
	out := make([]ProviderSessionAttempt, 0, len(input))
	for _, session := range input {
		workItems := dashboardProviderSessionWorkItems(session)
		out = append(out, ProviderSessionAttempt{
			ConsumedTokens:  dashboardTraceTokensFromInputs(session.ConsumedInputs, time.Time{}),
			DispatchId:      session.DispatchID,
			Outcome:         session.Outcome,
			ProviderSession: dashboardProviderSessionMetadata(&session.ProviderSession),
			TransitionId:    dashboardCompatTransitionID(session.TransitionID),
			WorkItems:       dashboardWorkItemRefs(workItems),
			WorkstationName: stringPtrIfNotEmptyForFunctionalDashboard(dashboardCompatWorkstationName(session.WorkstationName, session.TransitionID)),
		})
	}
	return &out
}

func dashboardWorkstationActivityByNodeID(input map[string]interfaces.FactoryWorldActivity) *map[string]DashboardWorkstationActivity {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]DashboardWorkstationActivity, len(input))
	for nodeID, activity := range input {
		out[nodeID] = DashboardWorkstationActivity{
			ActiveDispatchIds: stringSlicePtr(activity.ActiveDispatchIDs),
			ActiveWorkItems:   dashboardWorkItemRefs(activity.ActiveWorkItems),
			TraceIds:          stringSlicePtr(activity.TraceIDs),
			WorkstationNodeId: activity.WorkstationNodeID,
		}
	}
	return &out
}

func dashboardWorkItemRefs(input []interfaces.FactoryWorldWorkItemRef) *[]DashboardWorkItemRef {
	if len(input) == 0 {
		return nil
	}
	out := dashboardWorkItemRefsValue(input)
	return &out
}

func dashboardWorkItemRefsValue(input []interfaces.FactoryWorldWorkItemRef) []DashboardWorkItemRef {
	out := make([]DashboardWorkItemRef, 0, len(input))
	for _, item := range input {
		out = append(out, dashboardWorkItemRef(item))
	}
	return out
}

func dashboardWorkItemRef(item interfaces.FactoryWorldWorkItemRef) DashboardWorkItemRef {
	return DashboardWorkItemRef{
		DisplayName: stringPtrIfNotEmptyForFunctionalDashboard(item.DisplayName),
		TraceId:     stringPtrIfNotEmptyForFunctionalDashboard(item.TraceID),
		WorkId:      item.WorkID,
		WorkTypeId:  stringPtrIfNotEmptyForFunctionalDashboard(dashboardCompatWorkTypeID(item.WorkTypeID)),
	}
}

func dashboardDispatchWorkItems(dispatch interfaces.FactoryWorldDispatchCompletion) []interfaces.FactoryWorldWorkItemRef {
	seen := map[string]struct{}{}
	out := make([]interfaces.FactoryWorldWorkItemRef, 0, len(dispatch.InputWorkItems)+len(dispatch.OutputWorkItems)+len(dispatch.WorkItemIDs))
	appendItem := func(item interfaces.FactoryWorkItem) {
		if item.ID == "" {
			return
		}
		if _, ok := seen[item.ID]; ok {
			return
		}
		seen[item.ID] = struct{}{}
		out = append(out, interfaces.FactoryWorldWorkItemRef{
			WorkID:      item.ID,
			WorkTypeID:  dashboardCompatWorkTypeID(item.WorkTypeID),
			DisplayName: item.DisplayName,
			TraceID:     item.TraceID,
		})
	}
	for _, item := range dispatch.InputWorkItems {
		appendItem(item)
	}
	for _, item := range dispatch.OutputWorkItems {
		appendItem(item)
	}
	if dispatch.TerminalWork != nil {
		appendItem(dispatch.TerminalWork.WorkItem)
	}
	for _, input := range dispatch.ConsumedInputs {
		if input.WorkItem != nil {
			appendItem(*input.WorkItem)
		}
	}
	for _, workID := range dispatch.WorkItemIDs {
		if _, ok := seen[workID]; ok || workID == "" {
			continue
		}
		seen[workID] = struct{}{}
		out = append(out, interfaces.FactoryWorldWorkItemRef{WorkID: workID})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dashboardDispatchWorkTypeIDs(dispatch interfaces.FactoryWorldDispatchCompletion, workItems []interfaces.FactoryWorldWorkItemRef) []string {
	workTypeIDs := make([]string, 0, len(workItems))
	seen := map[string]struct{}{}
	for _, item := range workItems {
		if item.WorkTypeID == "" {
			continue
		}
		if _, ok := seen[item.WorkTypeID]; ok {
			continue
		}
		seen[item.WorkTypeID] = struct{}{}
		workTypeIDs = append(workTypeIDs, item.WorkTypeID)
	}
	for _, input := range dispatch.ConsumedInputs {
		if input.WorkItem == nil {
			continue
		}
		workTypeID := dashboardCompatWorkTypeID(input.WorkItem.WorkTypeID)
		if workTypeID == "" {
			continue
		}
		if _, ok := seen[workTypeID]; ok {
			continue
		}
		seen[workTypeID] = struct{}{}
		workTypeIDs = append(workTypeIDs, workTypeID)
	}
	sort.Strings(workTypeIDs)
	if len(workTypeIDs) == 0 {
		return nil
	}
	return workTypeIDs
}

func dashboardProviderSessionWorkItems(session interfaces.FactoryWorldProviderSessionRecord) []interfaces.FactoryWorldWorkItemRef {
	seen := map[string]struct{}{}
	out := make([]interfaces.FactoryWorldWorkItemRef, 0, len(session.ConsumedInputs)+len(session.WorkItemIDs))
	for _, input := range session.ConsumedInputs {
		if input.WorkItem == nil || input.WorkItem.ID == "" {
			continue
		}
		if _, ok := seen[input.WorkItem.ID]; ok {
			continue
		}
		seen[input.WorkItem.ID] = struct{}{}
		out = append(out, interfaces.FactoryWorldWorkItemRef{
			WorkID:      input.WorkItem.ID,
			WorkTypeID:  dashboardCompatWorkTypeID(input.WorkItem.WorkTypeID),
			DisplayName: input.WorkItem.DisplayName,
			TraceID:     input.WorkItem.TraceID,
		})
	}
	for _, workID := range session.WorkItemIDs {
		if workID == "" {
			continue
		}
		if _, ok := seen[workID]; ok {
			continue
		}
		seen[workID] = struct{}{}
		out = append(out, interfaces.FactoryWorldWorkItemRef{WorkID: workID})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dashboardTraceTokensFromInputs(inputs []interfaces.WorkstationInput, fallbackTime time.Time) *[]TraceTokenView {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]TraceTokenView, 0, len(inputs))
	for _, input := range inputs {
		if input.WorkItem == nil {
			continue
		}
		tokenID := input.TokenID
		if tokenID == "" {
			tokenID = input.WorkItem.ID
		}
		token := dashboardTraceTokenFromWorkItem(*input.WorkItem, tokenID, fallbackTime)
		token.PlaceId = dashboardCompatPlaceID(input.PlaceID)
		out = append(out, token)
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func dashboardTraceMutationsFromCompletion(dispatch interfaces.FactoryWorldDispatchCompletion) *[]TraceMutationView {
	if len(dispatch.OutputWorkItems) == 0 && dispatch.TerminalWork == nil {
		return nil
	}
	fromPlace := ""
	if len(dispatch.ConsumedInputs) > 0 {
		fromPlace = dispatch.ConsumedInputs[0].PlaceID
	}
	out := make([]TraceMutationView, 0, len(dispatch.OutputWorkItems)+1)
	for _, item := range dispatch.OutputWorkItems {
		out = append(out, dashboardTraceMutationFromWorkItem(item, fromPlace, dispatch.CompletedAt))
	}
	if dispatch.TerminalWork != nil {
		out = append(out, dashboardTraceMutationFromWorkItem(dispatch.TerminalWork.WorkItem, fromPlace, dispatch.CompletedAt))
	}
	return &out
}

func dashboardTraceMutationFromWorkItem(item interfaces.FactoryWorkItem, fromPlace string, fallbackTime time.Time) TraceMutationView {
	token := dashboardTraceTokenFromWorkItem(item, item.ID, fallbackTime)
	return TraceMutationView{
		FromPlace:      stringPtrIfNotEmptyForFunctionalDashboard(dashboardCompatPlaceID(fromPlace)),
		ResultingToken: &token,
		ToPlace:        stringPtrIfNotEmptyForFunctionalDashboard(dashboardCompatPlaceID(item.PlaceID)),
		TokenId:        item.ID,
		Type:           string(interfaces.MutationMove),
	}
}

func dashboardTraceTokenFromWorkItem(item interfaces.FactoryWorkItem, tokenID string, fallbackTime time.Time) TraceTokenView {
	return TraceTokenView{
		CreatedAt:  dashboardTimeString(fallbackTime),
		EnteredAt:  dashboardTimeString(fallbackTime),
		Name:       stringPtrIfNotEmptyForFunctionalDashboard(item.DisplayName),
		PlaceId:    dashboardCompatPlaceID(item.PlaceID),
		Tags:       stringMapPtrForFunctionalDashboard(item.Tags),
		TokenId:    tokenID,
		TraceId:    stringPtrIfNotEmptyForFunctionalDashboard(item.TraceID),
		WorkId:     item.ID,
		WorkTypeId: dashboardCompatWorkTypeID(item.WorkTypeID),
	}
}

func dashboardProviderSessionMetadata(session *interfaces.ProviderSessionMetadata) *ProviderSessionMetadata {
	if session == nil || session.ID == "" {
		return nil
	}
	return &ProviderSessionMetadata{
		Id:       stringPtrIfNotEmptyForFunctionalDashboard(session.ID),
		Kind:     stringPtrIfNotEmptyForFunctionalDashboard(session.Kind),
		Provider: stringPtrIfNotEmptyForFunctionalDashboard(session.Provider),
	}
}

func integerMapPtrForFunctionalDashboard(values map[string]int) *IntegerMap {
	if len(values) == 0 {
		return nil
	}
	converted := IntegerMap(values)
	return &converted
}

func stringMapPtrForFunctionalDashboard(values map[string]string) *StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := StringMap(values)
	return &converted
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func dashboardTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func stringPtrIfNotEmptyForFunctionalDashboard(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func copyIntPtrForFunctionalDashboard(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func dashboardCompatTransitionID(transitionID string) string {
	if transitionID == interfaces.SystemTimeExpiryTransitionID {
		return interfaces.SystemTimeDashboardExpiryTransitionID
	}
	return transitionID
}

func dashboardCompatWorkstationName(name, transitionID string) string {
	mappedTransitionID := dashboardCompatTransitionID(transitionID)
	if name != "" && name != transitionID {
		return name
	}
	return mappedTransitionID
}

func dashboardCompatPlaceID(placeID string) string {
	if placeID == interfaces.SystemTimePendingPlaceID {
		return interfaces.SystemTimeDashboardPendingPlaceID
	}
	return placeID
}

func dashboardCompatWorkTypeID(workTypeID string) string {
	if interfaces.IsSystemTimeWorkType(workTypeID) {
		return interfaces.SystemTimeDashboardWorkTypeID
	}
	return workTypeID
}

// ListWork fetches GET /work and returns the parsed response.
func (fs *FunctionalServer) ListWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	resp, err := http.Get(fs.httpSrv.URL + "/work")
	if err != nil {
		t.Fatalf("GET /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /work: expected 200 OK, got %d", resp.StatusCode)
	}
	var work factoryapi.ListWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		t.Fatalf("decode list work response: %v", err)
	}
	return work
}

// URL returns the base URL of the functional HTTP server.
func (fs *FunctionalServer) URL() string {
	return fs.httpSrv.URL
}

// OpenDashboardStream opens GET /events and projects each canonical event into
// the compatibility dashboard view used by older functional assertions.
func (fs *FunctionalServer) OpenDashboardStream(t *testing.T) *DashboardStream {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fs.httpSrv.URL+"/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("build events stream request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET /events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		t.Fatalf("GET /events: expected 200 OK, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		defer resp.Body.Close()
		t.Fatalf("GET /events: unexpected content type %q", resp.Header.Get("Content-Type"))
	}

	stream := &DashboardStream{
		t:         t,
		server:    fs,
		resp:      resp,
		cancel:    cancel,
		done:      make(chan struct{}),
		snapshots: make(chan DashboardResponse, 8),
		errs:      make(chan error, 1),
	}

	go stream.readSnapshots()

	t.Cleanup(stream.Close)
	return stream
}

// GetEngineStateSnapshot returns the canonical service-level aggregate snapshot
// so functional tests can compare it directly with HTTP observability surfaces.
func (fs *FunctionalServer) GetEngineStateSnapshot(t *testing.T) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	engineState, err := fs.service.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return engineState
}

func (stream *DashboardStream) readSnapshots() {
	defer close(stream.done)
	defer func() {
		if closeErr := stream.resp.Body.Close(); closeErr != nil {
			select {
			case stream.errs <- closeErr:
			default:
			}
		}
	}()

	scanner := bufio.NewScanner(stream.resp.Body)
	var dataLines []string

	flushEvent := func() {
		if len(dataLines) == 0 {
			dataLines = nil
			return
		}

		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &event); err != nil {
			select {
			case stream.errs <- err:
			default:
			}
			return
		}

		payload := stream.server.GetDashboard(stream.t)
		select {
		case stream.snapshots <- payload:
		default:
			select {
			case stream.errs <- fmt.Errorf("dashboard stream snapshot buffer overflow"):
			default:
			}
		}

		dataLines = nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushEvent()
			continue
		}

		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	flushEvent()

	if err := scanner.Err(); err != nil && !errorsIsContextCanceled(err) {
		select {
		case stream.errs <- err:
		default:
		}
	}
}

func (stream *DashboardStream) NextSnapshot(timeout time.Duration) DashboardResponse {
	stream.t.Helper()
	select {
	case snapshot := <-stream.snapshots:
		return snapshot
	case err := <-stream.errs:
		stream.t.Fatalf("dashboard stream error: %v", err)
	case <-time.After(timeout):
		stream.t.Fatalf("timed out waiting for dashboard stream snapshot within %s", timeout)
	}
	return DashboardResponse{}
}

func (stream *DashboardStream) Close() {
	stream.cancel()
	select {
	case <-stream.done:
	case <-time.After(2 * time.Second):
	}
}

func errorsIsContextCanceled(err error) bool {
	return err == context.Canceled || strings.Contains(err.Error(), "operation was canceled")
}

func resourceUsageSlicePtrForFunctionalState(values []ResourceUsage) *[]ResourceUsage {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func categorizeTokensForFunctionalState(snap *petri.MarkingSnapshot, net *state.Net) (StateCategories, []ResourceUsage) {
	var cats StateCategories
	resourceCounts := make(map[string]int)
	resourceTotals := make(map[string]int)

	for _, t := range snap.Tokens {
		if t == nil {
			continue
		}

		if t.Color.WorkTypeID == "" {
			name := t.PlaceID
			placeState := ""
			if idx := lastIndexByte(t.PlaceID, ':'); idx >= 0 {
				name = t.PlaceID[:idx]
				placeState = t.PlaceID[idx+1:]
			}
			resourceTotals[name]++
			if placeState == interfaces.ResourceStateAvailable {
				resourceCounts[name]++
			}
			continue
		}

		switch lookupStateCategoryForFunctionalState(net, t.PlaceID) {
		case state.StateCategoryFailed:
			cats.Failed++
		case state.StateCategoryTerminal:
			cats.Terminal++
		case state.StateCategoryInitial:
			cats.Initial++
		default:
			cats.Processing++
		}
	}

	resources := make([]ResourceUsage, 0, len(resourceTotals))
	for name, total := range resourceTotals {
		resources = append(resources, ResourceUsage{
			Name:      name,
			Available: resourceCounts[name],
			Total:     total,
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return cats, resources
}

func lookupStateCategoryForFunctionalState(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	place, ok := net.Places[placeID]
	if !ok {
		return state.StateCategoryProcessing
	}
	workType, ok := net.WorkTypes[place.TypeID]
	if !ok {
		return state.StateCategoryProcessing
	}
	for _, s := range workType.States {
		if s.Value == place.State {
			return s.Category
		}
	}
	return state.StateCategoryProcessing
}

func lastIndexByte(value string, c byte) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == c {
			return i
		}
	}
	return -1
}

// StartFunctionalServerWithConfig builds a FactoryService and exposes it as an
// HTTP test server, allowing callers to inject service-layer test seams such as
// provider or script command runners while preserving the real API/runtime
// wiring.
func StartFunctionalServerWithConfig(
	t *testing.T,
	factoryDir string,
	useMockWorkers bool,
	configure func(*service.FactoryServiceConfig),
	extraOpts ...factory.FactoryOption,
) *FunctionalServer {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Capture the API handler via the APIServerStarter callback.
	// The callback runs inside svc.Run() and receives the live service-facing
	// factory boundary used by the API server.
	var handler http.Handler
	var runtimeFactory apisurface.APISurface
	readyCh := make(chan struct{})

	cfg := &service.FactoryServiceConfig{
		Dir:          factoryDir,
		Port:         1, // non-zero enables APIServerStarter
		Logger:       zap.NewNop(),
		ExtraOptions: extraOpts,
		APIServerStarter: func(ctx context.Context, f apisurface.APISurface, port int, l *zap.Logger) error {
			runtimeFactory = f
			apiSrv := api.NewServer(f, 0, l)
			handler = apiSrv.Handler()
			close(readyCh)
			// Block until context is cancelled (required by callback contract).
			<-ctx.Done()
			return nil
		},
	}
	if useMockWorkers {
		cfg.MockWorkersConfig = config.NewEmptyMockWorkersConfig()
	}
	if configure != nil {
		configure(cfg)
	}

	svc, err := service.BuildFactoryService(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("BuildFactoryService: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			fmt.Printf("FunctionalServer: svc.Run ended: %v\n", err)
		}
	}()

	waitForFunctionalServerAPIHandler(t, readyCh, cancel)
	if cfg.RuntimeMode == interfaces.RuntimeModeService {
		waitForFunctionalServiceRuntime(t, svc, cancel)
	}

	httpSrv := httptest.NewServer(handler)

	fs := &FunctionalServer{
		httpSrv: httpSrv,
		factory: runtimeFactory,
		service: svc,
		cancel:  cancel,
		done:    done,
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		httpSrv.Close()
	})
	return fs
}

func waitForFunctionalServerAPIHandler(t *testing.T, readyCh <-chan struct{}, cancel context.CancelFunc) {
	t.Helper()
	select {
	case <-readyCh:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("FunctionalServer: timed out waiting for API handler")
	}
}

func waitForFunctionalServiceRuntime(t *testing.T, svc *service.FactoryService, cancel context.CancelFunc) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil && snapshot.FactoryState == string(interfaces.FactoryStateRunning) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatal("FunctionalServer: timed out waiting for service runtime startup")
}

// StartFunctionalServer builds a FactoryService and exposes it as an HTTP test
// server. The factory runs until its context is cancelled or all work completes.
// When useMockWorkers is true, the service uses default accepted mock-worker execution.
// Pass factory.WithWorkerExecutor(...) in extraOpts to register mock executors
// for tests that intentionally bypass service-level mock workers.
func StartFunctionalServer(t *testing.T, factoryDir string, useMockWorkers bool, extraOpts ...factory.FactoryOption) *FunctionalServer {
	return StartFunctionalServerWithConfig(t, factoryDir, useMockWorkers, nil, extraOpts...)
}
