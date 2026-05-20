package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorypkg "github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

const defaultMaxResults = 50

var _ factoryapi.ServerInterface = (*Server)(nil)

// --- Handlers ---

func (s *Server) requireSessionRuntime(w http.ResponseWriter) (apisurface.SessionAPISurface, bool) {
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session-scoped API is unavailable", "INTERNAL_ERROR")
		return nil, false
	}
	return s.sessionRuntime, true
}

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
	if err := validateGeneratedWorkContentAtPath(req.Content, "content"); err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
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
		Content:                workcontent.PartsFromGenerated(req.Content),
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

func (s *Server) SubmitWorkByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

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
		Content:                workcontent.PartsFromGenerated(req.Content),
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	workRequest := factorypkg.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(factoryID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("submit work failed", zap.Error(err), zap.String("factory_id", string(factoryID)))
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

func (s *Server) UpsertWorkRequestByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID, requestID string) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}

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
	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(factoryID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		s.logger.Error("upsert work request failed", zap.Error(err), zap.String("factory_id", string(factoryID)))
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

func (s *Server) ListFactorySessions(w http.ResponseWriter, r *http.Request) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	response, err := sessionRuntime.ListFactorySessions(r.Context())
	if err != nil {
		s.logger.Error("list factory sessions failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list factory sessions", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) OpenFactorySession(w http.ResponseWriter, r *http.Request) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	req, err := decodeOpenFactorySessionBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.FolderPath) == "" {
		s.writeError(w, http.StatusBadRequest, "folderPath is required", "BAD_REQUEST")
		return
	}
	response, err := sessionRuntime.OpenFactorySession(r.Context(), req)
	if err != nil {
		s.logger.Debug("open factory session rejected", zap.Error(err))
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) CloseFactorySession(w http.ResponseWriter, r *http.Request, sessionID string) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	if err := sessionRuntime.CloseFactorySession(r.Context(), sessionID); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("close factory session failed", zap.Error(err), zap.String("session_id", sessionID))
		s.writeError(w, http.StatusInternalServerError, "failed to close factory session", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetCurrentFactory(w http.ResponseWriter, r *http.Request) {
	namedFactory, ok := s.loadCurrentFactory(w, r)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, namedFactory)
}

func (s *Server) GetCurrentFactoryByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	namedFactory, err := sessionRuntime.GetCurrentNamedFactoryForSession(r.Context(), string(factoryID))
	if err != nil {
		switch {
		case errors.Is(err, apisurface.ErrFactorySessionNotFound):
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		default:
			s.logger.Error("get current factory failed", zap.Error(err), zap.String("factory_id", string(factoryID)))
			s.writeError(w, http.StatusInternalServerError, "failed to load current factory", "INTERNAL_ERROR")
			return
		}
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
	s.listWork(w, r, params, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) ListWorkByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID, params factoryapi.ListWorkByFactoryIdParams) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	legacyParams := factoryapi.ListWorkParams{
		MaxResults: params.MaxResults,
		NextToken:  params.NextToken,
		StateName:  params.StateName,
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
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(factoryID))
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
	s.getWork(w, r, id, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) GetWorkByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID, id factoryapi.WorkOrTokenID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getWork(w, r, id, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(factoryID))
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

	token, ok := snapshot.Marking.Tokens[id]
	if !ok || !publicWorkToken(token) {
		s.writeError(w, http.StatusNotFound, "token not found", "NOT_FOUND")
		return
	}

	s.writeJSON(w, http.StatusOK, tokenToResponse(token, true))
}

// GetStatus handles GET /status as the supported runtime status read model.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	s.getStatus(w, r, s.runtime.GetEngineStateSnapshot)
}

func (s *Server) GetStatusByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getStatus(w, r, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(factoryID))
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

func (s *Server) GetEventsByFactoryId(w http.ResponseWriter, r *http.Request, factoryID factoryapi.FactoryID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getEvents(w, r, func(ctx context.Context) (*interfaces.FactoryEventStream, error) {
		return sessionRuntime.SubscribeFactoryEventsForSession(ctx, string(factoryID))
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

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("encode response failed", zap.Error(err))
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.writeErrorWithTargets(w, status, message, code, nil)
}

func (s *Server) writeErrorWithTargets(w http.ResponseWriter, status int, message, code string, targets []factoryapi.ErrorTarget) {
	var targetPtr *[]factoryapi.ErrorTarget
	if len(targets) > 0 {
		targetPtr = &targets
	}
	s.writeJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
		Targets: targetPtr,
	})
}

func errorTarget(kind, id, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: kind}
	if id != "" {
		target.Id = &id
	}
	if field != "" {
		target.Field = &field
	}
	return target
}

func (s *Server) writeSSEDataJSON(w http.ResponseWriter, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(*values))
	copy(out, *values)
	return out
}

func stringSlicePtrCopy(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func integerMapPtr(values map[string]int) *factoryapi.IntegerMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.IntegerMap(values)
	return &converted
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(values)
	return &converted
}

func intPtrIfPositive(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
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
	if values == nil {
		return nil
	}
	return map[string]string(*values)
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
	if len(parts) == 0 {
		return nil
	}
	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				continue
			}
		case interfaces.WorkContentPartTypeImage:
			if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
				Type: factoryapi.WorkContentPartTypeImage,
				File: part.File,
			}); err != nil {
				continue
			}
		default:
			continue
		}
		content = append(content, generated)
	}
	if len(content) == 0 {
		return nil
	}
	return &content
}

func validateGeneratedWorkContentAtPath(content *factoryapi.WorkContent, fieldPath string) error {
	if content == nil || len(*content) == 0 {
		return nil
	}

	for i, part := range *content {
		pathPrefix := fmt.Sprintf("%s[%d].", fieldPath, i)
		textPart, textErr := part.AsWorkTextContentPart()
		if textErr == nil && textPart.Type == factoryapi.WorkContentPartTypeText {
			continue
		}

		imagePart, imageErr := part.AsWorkImageContentPart()
		if imageErr == nil && imagePart.Type == factoryapi.WorkContentPartTypeImage {
			continue
		}

		return requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text or image", pathPrefix)}
	}
	return nil
}

type requestFieldValidationError struct {
	message string
}

func (e requestFieldValidationError) Error() string {
	return e.message
}

func requestFieldValidationMessage(err error) (string, bool) {
	var validationErr requestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.message, true
	}
	return "", false
}

func decodeSubmitWorkRequestBody(body io.Reader) (factoryapi.SubmitWorkJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}

	var req factoryapi.SubmitWorkJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.SubmitWorkJSONRequestBody{}, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return factoryapi.SubmitWorkJSONRequestBody{}, requestFieldValidationError{message: "name is required"}
	}
	return req, nil
}

func decodeWorkRequestBody(body io.Reader) (factoryapi.UpsertWorkRequestJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	var req factoryapi.UpsertWorkRequestJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}
	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	if req.Works == nil || len(*req.Works) == 0 {
		return req, nil
	}

	var rawRequest struct {
		Works []map[string]json.RawMessage `json:"works"`
	}
	if err := json.Unmarshal(data, &rawRequest); err != nil {
		return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
	}

	for i := range *req.Works {
		if i >= len(rawRequest.Works) {
			return req, nil
		}
		if err := validateWorkContentField(rawRequest.Works[i], fmt.Sprintf("works[%d].", i)); err != nil {
			return factoryapi.UpsertWorkRequestJSONRequestBody{}, err
		}
	}
	return req, nil
}

func decodeNamedFactoryBody(body io.Reader) (factoryapi.CreateFactoryJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.CreateFactoryJSONRequestBody
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return factoryapi.CreateFactoryJSONRequestBody{}, requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return factoryapi.CreateFactoryJSONRequestBody{}, err
	}
	return req, nil
}

func decodeOpenFactorySessionBody(body io.Reader) (factoryapi.OpenFactorySessionJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.OpenFactorySessionJSONRequestBody{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.OpenFactorySessionJSONRequestBody
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.OpenFactorySessionJSONRequestBody{}, err
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return factoryapi.OpenFactorySessionJSONRequestBody{}, err
	}
	return req, nil
}

func decodeSaveEditableFactoryDefinitionBody(body io.Reader) (factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody
	if err := decoder.Decode(&req); err != nil {
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return factoryapi.SaveEditableCurrentFactoryDefinitionJSONRequestBody{}, err
	}
	return req, nil
}
func decodePromptTemplateValidationRequestBody(body io.Reader) (factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var req factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody
	if err := dec.Decode(&req); err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}
	if err := ensureSingleJSONObject(dec); err != nil {
		return factoryapi.ValidateCurrentFactoryWorkstationPromptTemplateJSONRequestBody{}, err
	}

	return req, nil
}

func ensureSingleJSONObject(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return requestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return err
	}
	return nil
}

func validateCanonicalWorkRequestJSONForAPI(data []byte) error {
	if err := factorypkg.ValidateCanonicalWorkRequestJSON(data); err != nil {
		return translateCanonicalWorkRequestValidationError(err)
	}
	return nil
}

func currentFactoryWorkstation(factory factoryapi.Factory, workstationName string) (factoryapi.Workstation, bool) {
	if factory.Workstations == nil {
		return factoryapi.Workstation{}, false
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name == workstationName || stringValue(workstation.Id) == workstationName {
			return workstation, true
		}
	}
	return factoryapi.Workstation{}, false
}

func promptTemplateContractResponse(contract workers.PromptTemplateContract) factoryapi.PromptTemplateContract {
	availableVariables := make([]factoryapi.PromptTemplateVariableReference, 0, len(contract.AvailableVariables))
	for _, reference := range contract.AvailableVariables {
		availableVariables = append(availableVariables, factoryapi.PromptTemplateVariableReference{
			Category:    factoryapi.PromptTemplateVariableReferenceCategory(reference.Category),
			Description: reference.Description,
			Example:     reference.Example,
			Path:        reference.Path,
		})
	}
	unavailablePatterns := make([]factoryapi.PromptTemplateUnavailableAccessPattern, 0, len(contract.UnavailableAccessPatterns))
	for _, pattern := range contract.UnavailableAccessPatterns {
		unavailablePatterns = append(unavailablePatterns, factoryapi.PromptTemplateUnavailableAccessPattern{
			Example: pattern.Example,
			Path:    pattern.Path,
			Reason:  pattern.Reason,
		})
	}
	return factoryapi.PromptTemplateContract{
		AvailableVariables:        availableVariables,
		InputCount:                contract.InputCount,
		UnavailableAccessPatterns: unavailablePatterns,
	}
}

func promptTemplateValidationResultResponse(result workers.PromptTemplateValidationResult) factoryapi.PromptTemplateValidationResult {
	diagnostics := make([]factoryapi.PromptTemplateDiagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		diagnostics = append(diagnostics, factoryapi.PromptTemplateDiagnostic{
			EndOffset:   diagnostic.EndOffset,
			Kind:        factoryapi.PromptTemplateDiagnosticKind(diagnostic.Kind),
			Message:     diagnostic.Message,
			Path:        diagnostic.Path,
			SourceText:  diagnostic.SourceText,
			StartOffset: diagnostic.StartOffset,
		})
	}
	return factoryapi.PromptTemplateValidationResult{
		Diagnostics: diagnostics,
		Valid:       result.Valid,
	}
}
func validateWorkContentField(fields map[string]json.RawMessage, prefix string) error {
	contentRaw, ok := fields["content"]
	if !ok {
		return nil
	}

	var partPayloads []json.RawMessage
	if err := json.Unmarshal(contentRaw, &partPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%scontent must be an array", prefix)}
	}
	for i, payload := range partPayloads {
		var partFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &partFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%scontent[%d] must be an object", prefix, i)}
		}
		if _, err := validatedRawWorkContentPart(partFields, fmt.Sprintf("%scontent[%d].", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func validatedRawWorkContentPart(fields map[string]json.RawMessage, prefix string) (interfaces.WorkContentPart, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var partType string
	if err := json.Unmarshal(typeRaw, &partType); err != nil || partType == "" {
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	switch interfaces.WorkContentPartType(partType) {
	case interfaces.WorkContentPartTypeText:
		if err := requireOnlyFields(fields, prefix, "type", "text"); err != nil {
			return interfaces.WorkContentPart{}, err
		}
		textRaw, ok := fields["text"]
		if !ok {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stext is required for text content parts", prefix)}
		}
		var text string
		if err := json.Unmarshal(textRaw, &text); err != nil {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stext must be a string", prefix)}
		}
		return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeText, Text: text}, nil
	case interfaces.WorkContentPartTypeImage:
		if err := requireOnlyFields(fields, prefix, "type", "file"); err != nil {
			return interfaces.WorkContentPart{}, err
		}
		fileRaw, ok := fields["file"]
		if !ok {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sfile is required for image content parts", prefix)}
		}
		var file string
		if err := json.Unmarshal(fileRaw, &file); err != nil || file == "" {
			return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%sfile must be a non-empty string", prefix)}
		}
		return interfaces.WorkContentPart{Type: interfaces.WorkContentPartTypeImage, File: file}, nil
	default:
		return interfaces.WorkContentPart{}, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text or image", prefix)}
	}
}

func requireOnlyFields(fields map[string]json.RawMessage, prefix string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		return requestFieldValidationError{message: fmt.Sprintf("%s%s is not supported", prefix, field)}
	}
	return nil
}

func applyStableTraceToWorkRequest(req *interfaces.WorkRequest) {
	if req == nil || len(req.Works) == 0 {
		return
	}
	traceID := ""
	if req.CurrentChainingTraceID != "" {
		traceID = req.CurrentChainingTraceID
	}
	if traceID == "" {
		for _, work := range req.Works {
			if work.CurrentChainingTraceID != "" {
				traceID = work.CurrentChainingTraceID
				break
			}
			if work.TraceID != "" {
				traceID = work.TraceID
				break
			}
		}
	}
	if traceID == "" {
		traceID = "trace-" + req.RequestID
	}
	if req.CurrentChainingTraceID == "" {
		req.CurrentChainingTraceID = traceID
	}
	for i := range req.Works {
		if req.Works[i].CurrentChainingTraceID == "" {
			if req.Works[i].TraceID != "" {
				req.Works[i].CurrentChainingTraceID = req.Works[i].TraceID
			} else {
				req.Works[i].CurrentChainingTraceID = traceID
			}
		}
		if req.Works[i].TraceID == "" {
			req.Works[i].TraceID = req.Works[i].CurrentChainingTraceID
		}
	}
}

func generatedPayloadToRawMessage(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}

func submitWorkBadRequestMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := err.Error()
	if strings.HasPrefix(message, "work_request:") {
		return submitWorkTypeNameMessage(message), true
	}
	if strings.Contains(message, "unknown work type") || strings.Contains(message, "work type") && strings.Contains(message, "not found") {
		return submitWorkTypeNameMessage(message), true
	}
	return "", false
}

func submitWorkTypeNameMessage(message string) string {
	message = strings.ReplaceAll(message, "work_type_name", "workTypeName")
	message = strings.ReplaceAll(message, "work_type_id", "workTypeName")
	if strings.Contains(message, "work type name") {
		return message
	}
	return strings.ReplaceAll(message, "work type", "work type name")
}

func translateCanonicalWorkRequestValidationError(err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	message = strings.TrimPrefix(message, "work request batch ")
	message = strings.ReplaceAll(message, " uses retired work_type_id field; use workTypeName", ".work_type_id is not supported; use workTypeName")
	message = strings.ReplaceAll(message, " uses retired target_state field; use state", ".target_state is not supported; use state")
	if strings.HasPrefix(message, "works[") && strings.Contains(message, "] ") {
		message = strings.Replace(message, "] ", "].", 1)
	}
	if strings.HasSuffix(message, ".work_type_id is not supported; use workTypeName") ||
		strings.HasSuffix(message, ".target_state is not supported; use state") {
		return requestFieldValidationError{message: message}
	}
	switch message {
	case "uses retired work_type_id field; use workTypeName":
		return requestFieldValidationError{message: "work_type_id is not supported; use workTypeName"}
	case "uses retired target_state field; use state":
		return requestFieldValidationError{message: "target_state is not supported; use state"}
	default:
		return requestFieldValidationError{message: message}
	}
}
