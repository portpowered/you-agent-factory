package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
)

const defaultMaxResults = work.DefaultListMaxResults

func (s *Server) ListWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListWorkBySessionIdParams) {
	workAPI, ok := s.requireWorkReadAPI(w)
	if !ok {
		return
	}
	result, err := workAPI.ListWork(r.Context(), string(sessionID), listWorkOptions(params))
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		var validation *work.ValidationError
		if errors.As(err, &validation) {
			s.writeError(w, http.StatusBadRequest, validation.Message, "BAD_REQUEST")
			return
		}
		s.logger.Error("list Work failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to list Work", "INTERNAL_ERROR")
		return
	}
	results := make([]factoryapi.Work, 0, len(result.Results))
	for _, item := range result.Results {
		results = append(results, workReadModelToGenerated(item))
	}
	response := factoryapi.ListWorkResponse{Results: results, PaginationContext: &factoryapi.PaginationContext{MaxResults: result.MaxResults}}
	if result.NextToken != "" {
		response.PaginationContext.NextToken = &result.NextToken
	}
	s.writeJSON(w, http.StatusOK, response)
}

func listWorkOptions(params factoryapi.ListWorkBySessionIdParams) work.ListOptions {
	return work.ListOptions{StateName: stringValue(params.StateName), StateType: listParamString(params.StateType), Name: stringValue(params.Name), WorkTypeName: stringValue(params.WorkTypeName), TraceID: stringValue(params.TraceId), SortBy: listParamString(params.SortBy), MaxResults: intValue(params.MaxResults), NextToken: stringValue(params.NextToken)}
}

func listParamString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func (s *Server) GetWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	workAPI, ok := s.requireWorkReadAPI(w)
	if !ok {
		return
	}
	result, err := workAPI.GetWork(r.Context(), string(sessionID), string(id))
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if errors.Is(err, work.ErrWorkNotFound) {
			s.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get Work failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to get Work", "INTERNAL_ERROR")
		return
	}
	s.writeJSON(w, http.StatusOK, workReadModelToGenerated(result))
}

func workReadModelToGenerated(item work.ReadModel) factoryapi.Work {
	result := factoryapi.Work{Name: item.Name, WorkId: stringPtrIfNotEmpty(item.WorkID), WorkTypeName: stringPtrIfNotEmpty(item.WorkTypeName), ChainingTraceDepth: intPtrIfPositive(item.ChainingTraceDepth), CurrentChainingTraceId: stringPtrIfNotEmpty(item.CurrentChainingTraceID), PreviousChainingTraceIds: stringSlicePtrCopy(item.PreviousChainingTraceIDs), TraceId: stringPtrIfNotEmpty(item.TraceID), Content: domainWorkContentToGeneratedPtr(item.Content), Tags: stringMapPtr(item.Tags), StopSummary: workStopSummaryToGenerated(item.StopSummary)}
	if item.State != nil {
		result.State = &factoryapi.WorkState{Name: item.State.Name, Type: factoryapi.WorkStateType(item.State.Type)}
	}
	if len(item.Relations) > 0 {
		relations := make([]factoryapi.Relation, 0, len(item.Relations))
		for _, relation := range item.Relations {
			relations = append(relations, factoryapi.Relation{Type: factoryapi.RelationType(relation.Type), SourceWorkName: relation.SourceWorkName, TargetWorkName: relation.TargetWorkName, TargetWorkId: stringPtrIfNotEmpty(relation.TargetWorkID), RequiredState: stringPtrIfNotEmpty(relation.RequiredState)})
		}
		result.Relations = &relations
	}
	return result
}

// WorkReadModelToGenerated maps the canonical Work read model to the generated
// HTTP representation.
func WorkReadModelToGenerated(item work.ReadModel) factoryapi.Work {
	return workReadModelToGenerated(item)
}

func workStopSummaryToGenerated(summary *work.StopSummary) *factoryapi.FactoryStopSummary {
	if summary == nil {
		return nil
	}
	result := &factoryapi.FactoryStopSummary{SessionId: summary.SessionID, StopKind: factoryapi.FactoryStopKind(summary.StopKind), WorkId: summary.WorkID, WorkName: summary.WorkName, WorkTypeName: summary.WorkTypeName, WorkState: summary.WorkState, LatestResultSummary: summary.LatestResultSummary, SuggestedRecoverySurface: summary.SuggestedRecoverySurface, SuggestedRecoveryAction: summary.SuggestedRecoveryAction}
	if summary.SessionLifecycleStatus != nil {
		status := factoryapi.FactorySessionDurableLifecycleStatus(*summary.SessionLifecycleStatus)
		result.SessionLifecycleStatus = &status
	}
	if summary.LatestDispatch != nil {
		result.LatestDispatch = &factoryapi.FactoryStopDispatchSummary{DispatchId: summary.LatestDispatch.DispatchID, Status: factoryapi.FactoryDispatchStatus(summary.LatestDispatch.Status), DispatchKind: factoryapi.FactoryDispatchKind(summary.LatestDispatch.DispatchKind), WorkstationName: summary.LatestDispatch.WorkstationName}
		if summary.LatestDispatch.FailureDetail != nil {
			result.LatestDispatch.FailureDetail = &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureType(summary.LatestDispatch.FailureDetail.Reason), Message: summary.LatestDispatch.FailureDetail.Message}
		}
	}
	return result
}
func domainWorkContentToGeneratedPtr(parts []work.WorkContentPart) *factoryapi.WorkContent {
	return contentcontract.GeneratedPtrFromParts(parts)
}

func (s *Server) StageSubmitWorkFileBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	definitions, ok := s.requireFactoryDefinitionAPI(w)
	if !ok {
		return
	}
	if _, err := definitions.GetCurrentFactoryForSession(r.Context(), string(sessionID)); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("stage submit-work file failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to stage submit-work file", "INTERNAL_ERROR")
		return
	}

	response, err := s.stageSubmitWorkFileRequest(r.Context(), r)
	if err != nil {
		var stagingErr *work.ContentStagingError
		if errors.As(err, &stagingErr) {
			s.writeError(w, http.StatusBadRequest, stagingErr.Message, "BAD_REQUEST")
			return
		}
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("stage submit-work file failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to stage submit-work file", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, response)
}

func (s *Server) stageSubmitWorkFileRequest(
	ctx context.Context,
	r *http.Request,
) (factoryapi.StageSubmitWorkFileResponse, error) {
	req, err := decodeStageSubmitWorkFileRequestBody(r.Body)
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, err
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, requestFieldValidationError{
			message: "contentBase64 must be valid base64",
		}
	}
	if len(content) == 0 {
		return factoryapi.StageSubmitWorkFileResponse{}, requestFieldValidationError{
			message: "contentBase64 must decode to a non-empty file payload",
		}
	}

	if s.workService == nil {
		return factoryapi.StageSubmitWorkFileResponse{}, errors.New("Work service is unavailable")
	}
	result, err := s.workService.StageContent(ctx, work.StageContentRequest{
		ItemType:  string(req.ItemType),
		FileName:  req.FileName,
		MediaType: req.MediaType,
		Content:   content,
	})
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, err
	}

	return factoryapi.StageSubmitWorkFileResponse{
		FileName:      result.FileName,
		MediaType:     result.MediaType,
		StagedFileRef: result.StagedFileRef,
		Url:           factoryapi.SubmitWorkContentURLProperty(result.URL),
	}, nil
}

func decodeStageSubmitWorkFileRequestBody(body io.Reader) (factoryapi.StageSubmitWorkFileRequest, error) {
	var rawFields map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&rawFields); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	if err := validateStageSubmitWorkFileRequestFields(rawFields); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}

	payload, err := json.Marshal(rawFields)
	if err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	var req factoryapi.StageSubmitWorkFileRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.StageSubmitWorkFileRequest{}, err
	}
	return req, nil
}

func validateStageSubmitWorkFileRequestFields(fields map[string]json.RawMessage) error {
	if err := requireOnlyFields(fields, "", "itemType", "fileName", "mediaType", "contentBase64"); err != nil {
		return err
	}

	itemType, err := requiredStageSubmitWorkItemType(fields)
	if err != nil {
		return err
	}
	switch itemType {
	case factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
	default:
		return requestFieldValidationError{message: "itemType must be one of image, video, audio, or document"}
	}

	if _, err := requiredNonEmptyStringField(fields, "", "fileName", "submit-work staged files"); err != nil {
		return err
	}

	if _, err := requiredNonEmptyStringField(fields, "", "mediaType", "submit-work staged files"); err != nil {
		return err
	}

	if _, err := requiredNonEmptyStringField(fields, "", "contentBase64", "submit-work staged files"); err != nil {
		return err
	}
	return nil
}

func requiredStageSubmitWorkItemType(
	fields map[string]json.RawMessage,
) (factoryapi.SubmitWorkItemType, error) {
	itemTypeRaw, ok := fields["itemType"]
	if !ok {
		return "", requestFieldValidationError{message: "itemType is required"}
	}

	var itemType string
	if err := json.Unmarshal(itemTypeRaw, &itemType); err != nil || itemType == "" {
		return "", requestFieldValidationError{message: "itemType must be a non-empty string"}
	}

	switch factoryapi.SubmitWorkItemType(itemType) {
	case factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
		return factoryapi.SubmitWorkItemType(itemType), nil
	case factoryapi.SubmitWorkItemTypeText:
		return "", requestFieldValidationError{
			message: "itemType must be one of image, video, audio, or document",
		}
	default:
		return "", requestFieldValidationError{
			message: "itemType must be one of image, video, audio, or document",
		}
	}
}
