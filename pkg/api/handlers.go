package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorypkg "github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

const defaultMaxResults = 50

var _ factoryapi.ServerInterface = (*Server)(nil)

// --- Handlers ---

func (s *Server) SubmitWork(w http.ResponseWriter, r *http.Request) {
	req, err := decodeSubmitWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if req.WorkTypeName == "" {
		s.writeError(w, http.StatusBadRequest, "workTypeName is required", "BAD_REQUEST")
		return
	}

	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	submitReq := interfaces.SubmitRequest{
		Name:                   strings.TrimSpace(req.Name),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                factorypkg.ResolveWorkRequestCurrentChainingTraceID(stringValue(req.CurrentChainingTraceId), stringValue(req.TraceId)),
		Content:                generatedWorkContentToDomain(req.Content),
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	workRequest := factorypkg.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

	result, err := s.runtime.SubmitWorkRequest(r.Context(), workRequest)
	if err != nil {
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("submit work failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, factoryapi.SubmitWorkResponse{TraceId: result.TraceID})
}

func (s *Server) UpsertWorkRequest(w http.ResponseWriter, r *http.Request, requestID string) {
	req, err := decodeWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	if requestID == "" {
		s.writeError(w, http.StatusBadRequest, "request_id is required", "BAD_REQUEST")
		return
	}
	if req.RequestId == "" {
		s.writeError(w, http.StatusBadRequest, "requestId is required", "BAD_REQUEST")
		return
	}
	if req.RequestId != requestID {
		s.writeError(w, http.StatusBadRequest, "request_id path and requestId body must match", "BAD_REQUEST")
		return
	}

	workRequest, err := generatedWorkRequestToDomain(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	applyStableTraceToWorkRequest(&workRequest)
	result, err := s.runtime.SubmitWorkRequest(r.Context(), workRequest)
	if err != nil {
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		s.logger.Error("upsert work request failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work request", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, factoryapi.UpsertWorkRequestResponse{RequestId: result.RequestID, TraceId: result.TraceID})
}

func (s *Server) CreateFactory(w http.ResponseWriter, r *http.Request) {
	req, err := decodeNamedFactoryBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if err := apisurface.ValidateWritableNamedFactoryName(req.Name); err != nil {
		s.writeError(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME")
		return
	}

	created, err := s.runtime.CreateNamedFactory(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrInvalidNamedFactoryName):
			s.writeError(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME")
			return
		case errors.Is(err, apisurface.ErrInvalidNamedFactory):
			s.writeError(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY")
			return
		case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
			s.writeError(w, http.StatusConflict, "Named factory already exists.", "FACTORY_ALREADY_EXISTS")
			return
		case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
			s.writeError(w, http.StatusConflict, "Current factory runtime must be idle before activation.", "FACTORY_NOT_IDLE")
			return
		default:
			s.logger.Error("create factory failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to store named factory", "INTERNAL_ERROR")
			return
		}
	}

	s.writeJSON(w, http.StatusCreated, created)
}

func (s *Server) GetCurrentFactory(w http.ResponseWriter, r *http.Request) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, namedFactory)
}

func (s *Server) GetCurrentFactoryWorkstationPromptTemplateContract(w http.ResponseWriter, r *http.Request, workstationName string) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	workstation, ok := currentFactoryWorkstation(namedFactory, workstationName)
	if !ok {
		s.writeError(w, http.StatusNotFound, "Current named factory workstation not found.", "NOT_FOUND")
		return
	}

	contract := workers.BuildPromptTemplateContract(len(workstation.Inputs))
	s.writeJSON(w, http.StatusOK, promptTemplateContractResponse(contract))
}

func (s *Server) ValidateCurrentFactoryWorkstationPromptTemplate(w http.ResponseWriter, r *http.Request, workstationName string) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	workstation, ok := currentFactoryWorkstation(namedFactory, workstationName)
	if !ok {
		s.writeError(w, http.StatusNotFound, "Current named factory workstation not found.", "NOT_FOUND")
		return
	}
	req, err := decodePromptTemplateValidationRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	result := workers.ValidatePromptTemplate(req.Prompt, len(workstation.Inputs))
	s.writeJSON(w, http.StatusOK, promptTemplateValidationResultResponse(result))
}

func (s *Server) loadCurrentFactory(w http.ResponseWriter, r *http.Request) (factoryapi.Factory, bool) {
	namedFactory, err := s.runtime.GetCurrentNamedFactory(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrCurrentNamedFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current named factory not found.", "NOT_FOUND")
			return factoryapi.Factory{}, false
		default:
			s.logger.Error("get current factory failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load current named factory", "INTERNAL_ERROR")
			return factoryapi.Factory{}, false
		}
	}
	return namedFactory, true
}

func (s *Server) GetEditableCurrentFactoryDefinition(w http.ResponseWriter, r *http.Request) {
	editable, err := s.runtime.GetEditableFactoryDefinition(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrCurrentNamedFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current named factory not found.", "NOT_FOUND")
			return
		default:
			s.logger.Error("get editable current factory definition failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load editable current factory definition", "INTERNAL_ERROR")
			return
		}
	}
	s.writeJSON(w, http.StatusOK, editable)
}

func (s *Server) SaveEditableCurrentFactoryDefinition(w http.ResponseWriter, r *http.Request) {
	req, err := decodeSaveEditableFactoryDefinitionBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeErrorWithTargets(w, http.StatusBadRequest, message, "BAD_REQUEST", []factoryapi.ErrorTarget{errorTarget("form", "", "factoryDefinition")})
			return
		}
		s.writeErrorWithTargets(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST", []factoryapi.ErrorTarget{errorTarget("form", "", "factoryDefinition")})
		return
	}

	saved, err := s.runtime.SaveEditableFactoryDefinition(r.Context(), req)
	if err != nil {
		var topologyErr *apisurface.TopologyValidationError
		switch {
		case errors.Is(err, apisurface.ErrCurrentNamedFactoryNotFound):
			s.writeError(w, http.StatusNotFound, "Current named factory not found.", "NOT_FOUND")
			return
		case errors.Is(err, apisurface.ErrInvalidNamedFactoryName):
			s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.", "INVALID_FACTORY_NAME", []factoryapi.ErrorTarget{errorTarget("field", "", "factoryDefinition.name")})
			return
		case errors.Is(err, apisurface.ErrEditableFactoryVersionStale):
			s.writeErrorWithTargets(w, http.StatusConflict, "Editable factory definition is stale. Refresh the graph before saving.", "STALE_FACTORY_VERSION", []factoryapi.ErrorTarget{errorTarget("save", "stale-version", "")})
			return
		case errors.As(err, &topologyErr):
			targets := topologyErr.Targets
			if len(targets) == 0 {
				targets = []factoryapi.ErrorTarget{errorTarget("form", "", "factoryDefinition")}
			}
			s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", targets)
			return
		case errors.Is(err, apisurface.ErrInvalidNamedFactory):
			s.writeErrorWithTargets(w, http.StatusBadRequest, "Factory payload is not a valid Agent Factory definition.", "INVALID_FACTORY", []factoryapi.ErrorTarget{errorTarget("form", "", "factoryDefinition")})
			return
		case errors.Is(err, apisurface.ErrFactoryActivationRequiresIdle):
			s.writeErrorWithTargets(w, http.StatusConflict, "Current factory runtime must be idle before activation.", "FACTORY_NOT_IDLE", []factoryapi.ErrorTarget{errorTarget("save", "active-work", "")})
			return
		default:
			s.logger.Error("save editable current factory definition failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to save editable current factory definition", "INTERNAL_ERROR")
			return
		}
	}

	s.writeJSON(w, http.StatusOK, saved)
}

func (s *Server) ListWork(w http.ResponseWriter, r *http.Request, params factoryapi.ListWorkParams) {
	if params.StateType != nil && !validWorkStateType(factoryapi.WorkStateType(*params.StateType)) {
		s.writeError(w, http.StatusBadRequest, "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED", "BAD_REQUEST")
		return
	}
	if params.SortBy != nil && *params.SortBy != factoryapi.ListWorkParamsSortByStateType {
		s.writeError(w, http.StatusBadRequest, "sortBy must be state.type", "BAD_REQUEST")
		return
	}

	snapshot, err := s.runtime.GetEngineStateSnapshot(r.Context())
	if err != nil {
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

func (s *Server) GetWork(w http.ResponseWriter, r *http.Request, id factoryapi.WorkOrTokenID) {
	snapshot, err := s.runtime.GetEngineStateSnapshot(r.Context())
	if err != nil {
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	token, ok := snapshot.Marking.Tokens[id]
	if !ok || !publicWorkToken(token) {
		s.writeError(w, http.StatusNotFound, "token not found", "NOT_FOUND")
		return
	}

	s.writeJSON(w, http.StatusOK, tokenToResponse(token, true))
}

// GetStatus handles GET /status as the supported runtime status read model.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.runtime.GetEngineStateSnapshot(r.Context())
	if err != nil {
		s.logger.Error("get engine state snapshot failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get engine state snapshot", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, statusFromEngineStateSnapshot(*snapshot))
}

// GetEvents handles GET /events as a canonical factory event SSE stream.
func (s *Server) GetEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported", "INTERNAL_ERROR")
		return
	}

	stream, err := s.runtime.SubscribeFactoryEvents(r.Context())
	if err != nil {
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
