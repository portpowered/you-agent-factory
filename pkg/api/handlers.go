// backendsizecheck:ignore-file this legacy API transport surface stays centralized until dedicated handler-splitting work lands.
// pkgmaintcheck:ignore-file-lines legacy API transport handlers still share generated-surface wiring; split by route family in dedicated follow-up work to avoid transport regressions.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/optional"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

var _ factoryapi.ServerInterface = (*Server)(nil)

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

func decodeModelInvocationRequestBody(body io.Reader) (factoryapi.ModelInvocationRequest, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return factoryapi.ModelInvocationRequest{}, err
		}
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return factoryapi.ModelInvocationRequest{}, requestFieldValidationError{message: "request body is required"}
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}

	var request factoryapi.ModelInvocationRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	return request, nil
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
