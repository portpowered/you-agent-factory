// backendsizecheck:ignore-file this legacy API transport surface stays centralized until dedicated handler-splitting work lands.
// pkgmaintcheck:ignore-file-lines legacy API transport handlers still share generated-surface wiring; split by route family in dedicated follow-up work to avoid transport regressions.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api/workstationprojection"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/optional"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

const defaultMaxResults = 50

var _ factoryapi.ServerInterface = (*Server)(nil)

// BuildFactoryWorldWorkstationRequestProjectionSlice delegates to the
// workstationprojection subpackage while preserving the historical pkg/api entrypoint.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	return workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

// --- Handlers ---

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.SessionAPISurface, bool) {
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionRuntime, true
}

func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	response, err := s.runtime.ListModels(r.Context())
	if err != nil {
		s.logger.Error("list models failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list models", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) GetModel(w http.ResponseWriter, r *http.Request, modelName string) {
	model, err := s.runtime.GetModel(r.Context(), modelName)
	if err != nil {
		if errors.Is(err, apisurface.ErrModelNotFound) {
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get model failed", zap.Error(err), zap.String("model_name", modelName))
		s.writeError(w, http.StatusInternalServerError, "failed to load model", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, model)
}

func (s *Server) InvokeModel(w http.ResponseWriter, r *http.Request, modelName string) {
	req, err := decodeModelInvocationRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Operation) == "" {
		s.writeError(w, http.StatusBadRequest, "operation is required", "BAD_REQUEST")
		return
	}

	result, err := s.runtime.InvokeModel(r.Context(), modelName, req)
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrModelNotFound):
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
		case errors.Is(err, apisurface.ErrModelNotAvailable):
			s.writeError(w, http.StatusNotFound, err.Error(), "MODEL_NOT_AVAILABLE")
		case errors.Is(err, apisurface.ErrModelInvocationUnsupportedOperation), errors.Is(err, apisurface.ErrModelInvocationUnsupportedMode):
			s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		default:
			errText := strings.TrimSpace(err.Error())
			if strings.HasPrefix(errText, "provider execution failed:") {
				s.writeError(w, http.StatusInternalServerError, errText, "INTERNAL_ERROR")
				return
			}
			s.writeError(w, http.StatusBadRequest, errText, "BAD_REQUEST")
		}
		return
	}

	if strings.TrimSpace(result.StreamFile) != "" {
		if result.StreamContentType != "" {
			w.Header().Set("Content-Type", result.StreamContentType)
		}
		http.ServeFile(w, r, result.StreamFile)
		return
	}

	s.writeJSON(w, http.StatusOK, factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           result.Worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Content:          derefGeneratedWorkContent(workcontent.GeneratedPtrFromParts(result.Content)),
		Bindings:         generatedResolvedModelInvocationBindings(result.Bindings),
	})
}

func (s *Server) PullModel(w http.ResponseWriter, r *http.Request, modelName string) {
	result, err := s.runtime.PullModel(r.Context(), modelName)
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrModelNotFound):
			s.writeError(w, http.StatusNotFound, "model not found", "NOT_FOUND")
		case errors.Is(err, apisurface.ErrModelPullUnsupported):
			s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		default:
			s.writeError(w, http.StatusInternalServerError, strings.TrimSpace(err.Error()), "INTERNAL_ERROR")
		}
		return
	}
	files := make([]factoryapi.ModelPullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		current := factoryapi.ModelPullDownloadedFile{
			Path:  file.Path,
			Bytes: file.Bytes,
		}
		if sha := strings.TrimSpace(file.SHA256); sha != "" {
			current.Sha256 = &sha
		}
		files = append(files, current)
	}
	s.writeJSON(w, http.StatusOK, factoryapi.ModelPullResponse{
		ModelName:        result.ModelName,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Outcome:          factoryapi.ModelPullOutcome(result.Outcome),
		CachePath:        result.CachePath,
		Revision:         result.Revision,
		DownloadedFiles:  files,
	})
}

func (s *Server) ListWork(w http.ResponseWriter, r *http.Request, params factoryapi.ListWorkParams) {
	s.listWork(w, r, params, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) ListWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListWorkBySessionIdParams) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	legacyParams := factoryapi.ListWorkParams{
		MaxResults:   params.MaxResults,
		NextToken:    params.NextToken,
		StateName:    params.StateName,
		Name:         params.Name,
		WorkTypeName: params.WorkTypeName,
		TraceId:      params.TraceId,
	}
	if params.StateType != nil {
		stateType := factoryapi.WorkStateType(*params.StateType)
		legacyParams.StateType = &stateType
	}
	if params.SortBy != nil {
		sortBy := factoryapi.ListWorkParamsSortBy(*params.SortBy)
		legacyParams.SortBy = &sortBy
	}
	s.listWork(w, r, legacyParams, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
	})
}

func (s *Server) listWork(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.ListWorkParams,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
) {
	if params.StateType != nil && !validWorkStateType(factoryapi.WorkStateType(*params.StateType)) {
		s.writeError(w, http.StatusBadRequest, "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED", "BAD_REQUEST")
		return
	}
	if params.SortBy != nil && *params.SortBy != factoryapi.ListWorkParamsSortByStateType {
		s.writeError(w, http.StatusBadRequest, "sortBy must be state.type", "BAD_REQUEST")
		return
	}

	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	// Collect, filter, and sort public work for deterministic pagination.
	workNamesByID := publicWorkNamesByID(snapshot.Marking.Tokens)
	items := make([]listWorkItem, 0, len(snapshot.Marking.Tokens))
	for _, t := range snapshot.Marking.Tokens {
		if !publicWorkToken(t) {
			continue
		}
		work := tokenToWork(t, snapshot.Topology)
		work.Relations = generatedWorkRelations(t, work.Name, workNamesByID)
		if !workMatchesListFilters(work, params) {
			continue
		}
		items = append(items, listWorkItem{cursorID: t.ID, work: work})
	}
	sortListWorkItems(items, listWorkSortMode(params.SortBy))

	// Consume the generated route params directly. Non-positive values still fall back
	// to the default page size after successful integer binding.
	maxResults := defaultMaxResults
	if params.MaxResults != nil && *params.MaxResults > 0 {
		maxResults = *params.MaxResults
	}

	startIdx := 0
	if cursor := stringValue(params.NextToken); cursor != "" {
		decoded, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil {
			startIdx = nextListWorkIndex(items, string(decoded))
		}
	}

	// Slice the results.
	end := min(startIdx+maxResults, len(items))
	page := items[startIdx:end]

	resp := factoryapi.ListWorkResponse{
		Results: listWorkResults(page),
		PaginationContext: &factoryapi.PaginationContext{
			MaxResults: maxResults,
		},
	}
	if end < len(items) {
		lastID := page[len(page)-1].cursorID
		nextToken := base64.StdEncoding.EncodeToString([]byte(lastID))
		resp.PaginationContext.NextToken = &nextToken
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func validWorkStateType(stateType factoryapi.WorkStateType) bool {
	switch stateType {
	case factoryapi.WorkStateTypeINITIAL,
		factoryapi.WorkStateTypePROCESSING,
		factoryapi.WorkStateTypeTERMINAL,
		factoryapi.WorkStateTypeFAILED:
		return true
	default:
		return false
	}
}

type listWorkItem struct {
	cursorID string
	work     factoryapi.Work
}

type listWorkSortModeValue int

const (
	listWorkSortDefault listWorkSortModeValue = iota
	listWorkSortStateType
)

func listWorkSortMode(sortBy *factoryapi.ListWorkParamsSortBy) listWorkSortModeValue {
	if sortBy != nil && *sortBy == factoryapi.ListWorkParamsSortByStateType {
		return listWorkSortStateType
	}
	return listWorkSortDefault
}

func sortListWorkItems(items []listWorkItem, mode listWorkSortModeValue) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if mode == listWorkSortStateType {
			return lessListWorkByStateType(left, right)
		}

		leftOrder := listWorkStateOrder(left.work.State)
		rightOrder := listWorkStateOrder(right.work.State)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}

		leftStateType := listWorkStateType(left.work.State)
		rightStateType := listWorkStateType(right.work.State)
		if leftStateType != rightStateType {
			return leftStateType < rightStateType
		}

		return left.cursorID < right.cursorID
	})
}

func lessListWorkByStateType(left, right listWorkItem) bool {
	leftStateType := listWorkStateType(left.work.State)
	rightStateType := listWorkStateType(right.work.State)
	if leftStateType != rightStateType {
		return leftStateType < rightStateType
	}
	return left.cursorID < right.cursorID
}

func listWorkStateOrder(workState *factoryapi.WorkState) int {
	if workState == nil {
		return 4
	}
	switch workState.Type {
	case factoryapi.WorkStateTypeINITIAL:
		return 0
	case factoryapi.WorkStateTypePROCESSING:
		return 1
	case factoryapi.WorkStateTypeFAILED:
		return 2
	case factoryapi.WorkStateTypeTERMINAL:
		return 3
	default:
		return 4
	}
}

func listWorkStateType(workState *factoryapi.WorkState) string {
	if workState == nil {
		return ""
	}
	return string(workState.Type)
}

func nextListWorkIndex(items []listWorkItem, cursorID string) int {
	for i, item := range items {
		if item.cursorID == cursorID {
			return i + 1
		}
	}
	return len(items)
}

func listWorkResults(items []listWorkItem) []factoryapi.Work {
	results := make([]factoryapi.Work, len(items))
	for i, item := range items {
		results[i] = item.work
	}
	return results
}

func workMatchesListFilters(work factoryapi.Work, params factoryapi.ListWorkParams) bool {
	return workMatchesStateListFilters(work, params) &&
		workMatchesNameListFilter(work, params) &&
		workMatchesWorkTypeNameListFilter(work, params) &&
		workMatchesTraceIDListFilter(work, params)
}

func workMatchesStateListFilters(work factoryapi.Work, params factoryapi.ListWorkParams) bool {
	if params.StateName != nil {
		if work.State == nil || work.State.Name != *params.StateName {
			return false
		}
	}
	if params.StateType != nil {
		if work.State == nil || work.State.Type != *params.StateType {
			return false
		}
	}
	return true
}

func workMatchesNameListFilter(work factoryapi.Work, params factoryapi.ListWorkParams) bool {
	if params.Name == nil || *params.Name == "" {
		return true
	}
	return strings.Contains(strings.ToLower(work.Name), strings.ToLower(string(*params.Name)))
}

func workMatchesWorkTypeNameListFilter(work factoryapi.Work, params factoryapi.ListWorkParams) bool {
	if params.WorkTypeName == nil || *params.WorkTypeName == "" {
		return true
	}
	return stringValue(work.WorkTypeName) == string(*params.WorkTypeName)
}

func workMatchesTraceIDListFilter(work factoryapi.Work, params factoryapi.ListWorkParams) bool {
	if params.TraceId == nil || *params.TraceId == "" {
		return true
	}
	traceID := string(*params.TraceId)
	return stringValue(work.TraceId) == traceID || stringValue(work.CurrentChainingTraceId) == traceID
}

func (s *Server) GetWork(w http.ResponseWriter, r *http.Request, id factoryapi.WorkOrTokenID) {
	s.getWork(w, r, id, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) GetWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getWork(w, r, id, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
	})
}

func (s *Server) getWork(
	w http.ResponseWriter,
	r *http.Request,
	id factoryapi.WorkOrTokenID,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
) {
	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	token, ok := findPublicWorkToken(snapshot.Marking.Tokens, string(id))
	if !ok {
		s.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
		return
	}

	workNamesByID := publicWorkNamesByID(snapshot.Marking.Tokens)
	work := tokenToWork(token, snapshot.Topology)
	work.Relations = generatedWorkRelations(token, work.Name, workNamesByID)
	s.writeJSON(w, http.StatusOK, work)
}

func findPublicWorkToken(tokens map[string]*interfaces.Token, id string) (*interfaces.Token, bool) {
	if token, ok := tokens[id]; ok && publicWorkToken(token) {
		return token, true
	}
	for _, token := range tokens {
		if !publicWorkToken(token) {
			continue
		}
		if token.Color.WorkID == id {
			return token, true
		}
	}
	return nil, false
}

// GetStatus handles GET /status as the supported runtime status read model.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.getStatus(w, r, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) GetStatusBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getStatus(w, r, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
	})
}

func (s *Server) getStatus(
	w http.ResponseWriter,
	r *http.Request,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
) {
	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, statusFromEngineStateSnapshot(*snapshot))
}

// GetEvents handles GET /events as a canonical factory event SSE stream.
func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request) {
	s.getEvents(w, r, s.runtime.SubscribeFactoryEvents)
}

func (s *Server) GetEventsBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getEvents(w, r, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return sessionRuntime.SubscribeFactoryEventsForSession(ctx, string(sessionID))
	})
}

func (s *Server) getEvents(
	w http.ResponseWriter,
	r *http.Request,
	subscribe func(context.Context) (*interfaces.FactoryEventStream, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	stream, err := subscribe(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("subscribe factory events failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to subscribe to factory events", "INTERNAL_ERROR")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	for _, event := range stream.History {
		if err := s.writeSSEDataJSON(w, event); err != nil {
			s.logger.Debug("write historical factory event failed", zap.Error(err))
			return
		}
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-stream.Events:
			if !ok {
				return
			}
			if err := s.writeSSEDataJSON(w, event); err != nil {
				s.logger.Debug("write live factory event failed", zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}

// --- Helpers ---

func tokenToResponse(t *interfaces.Token, includeHistory bool) factoryapi.TokenResponse {
	resp := factoryapi.TokenResponse{
		Id:                       t.ID,
		PlaceId:                  t.PlaceID,
		WorkId:                   t.Color.WorkID,
		WorkType:                 t.Color.WorkTypeID,
		ChainingTraceDepth:       intPtrIfPositive(t.Color.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(firstNonEmptyString(t.Color.CurrentChainingTraceID, t.Color.TraceID)),
		PreviousChainingTraceIds: stringSlicePtrCopy(t.Color.PreviousChainingTraceIDs),
		TraceId:                  t.Color.TraceID,
		Content:                  domainWorkContentToGeneratedPtr(t.Color.Content),
		Tags:                     stringMapPtr(t.Color.Tags),
		CreatedAt:                t.CreatedAt,
		EnteredAt:                t.EnteredAt,
	}
	if t.Color.Name != "" {
		resp.Name = &t.Color.Name
	}
	if len(t.Color.Tags) == 0 {
		resp.Tags = nil
	}
	if includeHistory {
		resp.History = &factoryapi.TokenHistory{
			TotalVisits:         integerMapPtr(t.History.TotalVisits),
			ConsecutiveFailures: integerMapPtr(t.History.ConsecutiveFailures),
			PlaceVisits:         integerMapPtr(t.History.PlaceVisits),
			LastError:           stringPtrIfNotEmpty(t.History.LastError),
		}
	}
	return resp
}

func tokenToWork(t *interfaces.Token, net *state.Net) factoryapi.Work {
	name := firstNonEmptyString(t.Color.Name, t.Color.WorkID, t.ID)
	return factoryapi.Work{
		Name:                     name,
		WorkId:                   stringPtrIfNotEmpty(t.Color.WorkID),
		WorkTypeName:             stringPtrIfNotEmpty(t.Color.WorkTypeID),
		State:                    workStateForToken(t, net),
		ChainingTraceDepth:       intPtrIfPositive(t.Color.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(firstNonEmptyString(t.Color.CurrentChainingTraceID, t.Color.TraceID)),
		PreviousChainingTraceIds: stringSlicePtrCopy(t.Color.PreviousChainingTraceIDs),
		TraceId:                  stringPtrIfNotEmpty(t.Color.TraceID),
		Content:                  domainWorkContentToGeneratedPtr(t.Color.Content),
		Tags:                     stringMapPtr(t.Color.Tags),
	}
}

func publicWorkNamesByID(tokens map[string]*interfaces.Token) map[string]string {
	names := make(map[string]string, len(tokens))
	for _, token := range tokens {
		if !publicWorkToken(token) || token.Color.WorkID == "" {
			continue
		}
		names[token.Color.WorkID] = firstNonEmptyString(token.Color.Name, token.Color.WorkID, token.ID)
	}
	return names
}

func generatedWorkRelations(token *interfaces.Token, sourceWorkName string, workNamesByID map[string]string) *[]factoryapi.Relation {
	if token == nil || len(token.Color.Relations) == 0 {
		return nil
	}

	relations := make([]factoryapi.Relation, 0, len(token.Color.Relations))
	for _, relation := range token.Color.Relations {
		targetWorkName := firstNonEmptyString(workNamesByID[relation.TargetWorkID], relation.TargetWorkID)
		relations = append(relations, factoryapi.Relation{
			Type:           factoryapi.RelationType(relation.Type),
			SourceWorkName: sourceWorkName,
			TargetWorkName: targetWorkName,
			TargetWorkId:   stringPtrIfNotEmpty(relation.TargetWorkID),
			RequiredState:  stringPtrIfNotEmpty(relation.RequiredState),
		})
	}
	return &relations
}

func workStateForToken(t *interfaces.Token, net *state.Net) *factoryapi.WorkState {
	if t == nil {
		return nil
	}
	workTypeID, stateName := state.SplitPlaceID(t.PlaceID)
	if t.Color.WorkTypeID != "" {
		workTypeID = t.Color.WorkTypeID
	}
	if net != nil {
		if place, ok := net.Places[t.PlaceID]; ok {
			workTypeID = place.TypeID
			stateName = place.State
		}
	}
	if stateName == "" {
		return nil
	}
	return &factoryapi.WorkState{
		Name: stateName,
		Type: factoryapi.WorkStateType(state.CategoryForState(workTypesFromNet(net), workTypeID, stateName)),
	}
}

func workTypesFromNet(net *state.Net) map[string]*state.WorkType {
	if net == nil {
		return nil
	}
	return net.WorkTypes
}

func publicWorkToken(token *interfaces.Token) bool {
	return token != nil &&
		token.Color.DataType != interfaces.DataTypeResource &&
		!interfaces.IsSystemTimeToken(token)
}

func statusFromEngineStateSnapshot(snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) factoryapi.StatusResponse {
	categories, resources := categorizeStatusTokens(&snapshot.Marking, snapshot.Topology)
	return factoryapi.StatusResponse{
		Categories:    categories,
		FactoryState:  snapshot.FactoryState,
		Resources:     resourceUsagePtr(resources),
		RuntimeStatus: string(snapshot.RuntimeStatus),
		TotalTokens:   countPublicStatusTokens(&snapshot.Marking),
	}
}

func categorizeStatusTokens(marking *petri.MarkingSnapshot, net *state.Net) (factoryapi.StatusCategories, []factoryapi.ResourceUsage) {
	var categories factoryapi.StatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := resourceTotalsFromTopology(net)

	if marking == nil {
		return categories, resourceUsage(resourceCounts, resourceTotals)
	}

	for _, token := range marking.Tokens {
		if token == nil {
			continue
		}
		if interfaces.IsSystemTimeToken(token) {
			continue
		}

		if token.Color.DataType == interfaces.DataTypeResource {
			resourceID, resourceState := state.SplitPlaceID(token.PlaceID)
			if _, ok := resourceTotals[resourceID]; !ok {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}

		switch statusStateCategory(net, token.PlaceID) {
		case state.StateCategoryFailed:
			categories.Failed++
		case state.StateCategoryTerminal:
			categories.Terminal++
		case state.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}

	return categories, resourceUsage(resourceCounts, resourceTotals)
}

func countPublicStatusTokens(marking *petri.MarkingSnapshot) int {
	if marking == nil {
		return 0
	}
	count := 0
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		count++
	}
	return count
}

func statusStateCategory(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	return net.StateCategoryForPlace(placeID)
}

func resourceTotalsFromTopology(net *state.Net) map[string]int {
	totals := make(map[string]int)
	if net == nil {
		return totals
	}
	for id, resource := range net.Resources {
		if resource == nil {
			continue
		}
		totals[id] = resource.Capacity
	}
	return totals
}

func resourceUsage(counts map[string]int, totals map[string]int) []factoryapi.ResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resources := make([]factoryapi.ResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}

func resourceUsagePtr(values []factoryapi.ResourceUsage) *[]factoryapi.ResourceUsage {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringSlicePtrCopy(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func integerMapPtr(values map[string]int) *factoryapi.IntegerMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.IntegerMap(values)
	return &converted
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	return optional.CopiedStringMapPtr(values)
}

func intPtrIfPositive(value int) *int {
	return optional.PositiveIntPtr(value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func generatedStringMap(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}

func generatedSubmitRelations(values *[]factoryapi.SubmitRelation) []interfaces.Relation {
	if values == nil || len(*values) == 0 {
		return nil
	}
	relations := make([]interfaces.Relation, 0, len(*values))
	for _, relation := range *values {
		relations = append(relations, interfaces.Relation{
			Type:          interfaces.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkId,
			RequiredState: stringValue(relation.RequiredState),
		})
	}
	return relations
}

func generatedWorkRequestToDomain(req factoryapi.WorkRequest) (interfaces.WorkRequest, error) {
	workRequest := interfaces.WorkRequest{
		RequestID:              req.RequestId,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		Type:                   interfaces.WorkRequestType(req.Type),
	}
	if req.Works != nil {
		workRequest.Works = make([]interfaces.Work, 0, len(*req.Works))
		for i, work := range *req.Works {
			if err := validateGeneratedWorkContentAtPath(work.Content, fmt.Sprintf("works[%d].content", i)); err != nil {
				return interfaces.WorkRequest{}, err
			}
			workRequest.Works = append(workRequest.Works, interfaces.Work{
				Name:                     work.Name,
				WorkID:                   stringValue(work.WorkId),
				RequestID:                stringValue(work.RequestId),
				WorkTypeID:               stringValue(work.WorkTypeName),
				State:                    generatedWorkStateName(work.State),
				ChainingTraceDepth:       intValue(work.ChainingTraceDepth),
				CurrentChainingTraceID:   stringValue(work.CurrentChainingTraceId),
				PreviousChainingTraceIDs: stringSliceValue(work.PreviousChainingTraceIds),
				TraceID:                  stringValue(work.TraceId),
				Content:                  workcontent.PartsFromGenerated(work.Content),
				Payload:                  work.Payload,
				Tags:                     generatedStringMap(work.Tags),
			})
		}
	}
	if req.Relations != nil {
		workRequest.Relations = make([]interfaces.WorkRelation, 0, len(*req.Relations))
		for _, relation := range *req.Relations {
			workRequest.Relations = append(workRequest.Relations, interfaces.WorkRelation{
				Type:           interfaces.WorkRelationType(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  stringValue(relation.RequiredState),
			})
		}
	}
	return workRequest, nil
}

func domainWorkContentToGeneratedPtr(parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	return workcontent.GeneratedPtrFromParts(parts)
}

func validateGeneratedWorkContentAtPath(content *factoryapi.WorkContent, fieldPath string) error {
	if content == nil || len(*content) == 0 {
		return nil
	}

	for i, part := range *content {
		pathPrefix := fmt.Sprintf("%s[%d].", fieldPath, i)
		if _, ok := workcontent.PartFromGenerated(part); ok {
			continue
		}

		return requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", pathPrefix)}
	}
	return nil
}

func generatedResolvedModelInvocationBindings(values []interfaces.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	bindings := make([]factoryapi.ResolvedModelOperationBinding, 0, len(values))
	for _, binding := range values {
		content := workcontent.GeneratedPtrFromParts(binding.Content)
		bindings = append(bindings, factoryapi.ResolvedModelOperationBinding{
			Slot:    binding.Slot,
			Source:  factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: derefGeneratedWorkContent(content),
		})
	}
	return bindings
}

func derefGeneratedWorkContent(content *factoryapi.WorkContent) factoryapi.WorkContent {
	if content == nil {
		return nil
	}
	return *content
}

func generatedPayloadToRawMessage(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

