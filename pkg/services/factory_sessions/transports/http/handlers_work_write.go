package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
)

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
)

const (
	// SubmitWorkItemTypeMetadataKey records the structured submission item kind.
	SubmitWorkItemTypeMetadataKey = submitWorkItemTypeMetadataKey
	// SubmitWorkFileNameMetadataKey records the original structured file name.
	SubmitWorkFileNameMetadataKey = submitWorkFileNameMetadataKey
)

func (s *Server) InvokeFactorySessionBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	req, err := decodeInvocationRequestBody(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if s.invocation == nil {
		s.writeError(w, http.StatusInternalServerError, "session invocation API is unavailable", "INTERNAL_ERROR")
		return
	}

	result, err := s.invocation.InvokeFactorySession(r.Context(), string(sessionID), req)
	if err != nil {
		switch typed := err.(type) {
		case *work.InputError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *work.ArgumentError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *apisurface.RequestValidationError:
			s.writeError(w, http.StatusBadRequest, typed.Message, "BAD_REQUEST")
		default:
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
				s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
				return
			}
			s.logger.Error("invoke factory session failed", zap.Error(err), zap.String("session_id", string(sessionID)))
			s.writeError(w, http.StatusInternalServerError, "failed to invoke factory session", "INTERNAL_ERROR")
		}
		return
	}

	response := apisurface.InvocationResponseFromResult(result)
	s.writeJSON(w, http.StatusOK, response)
}

func decodeInvocationRequestBody(body io.Reader) (factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody, error) {
	return decodeStrictJSON[factoryapi.InvokeFactorySessionBySessionIdJSONRequestBody](body)
}

func (s *Server) submitWorkContent(
	ctx context.Context,
	req factoryapi.SubmitWorkRequest,
) ([]work.WorkContentPart, error) {
	if req.Items == nil {
		return contentcontract.PartsFromGenerated(req.Content), nil
	}
	if s.contentStaging == nil {
		return nil, errors.New("Work content staging service is unavailable")
	}
	items, err := stagedSubmissionItemsFromGenerated(*req.Items)
	if err != nil {
		return nil, err
	}
	content, err := s.contentStaging.PrepareContent(ctx, items)
	if err != nil {
		var stagingErr *work.ContentStagingError
		if errors.As(err, &stagingErr) {
			return nil, requestFieldValidationError{message: stagingErr.Message}
		}
		return nil, err
	}
	return content, nil
}

func stagedSubmissionItemsFromGenerated(
	items []factoryapi.SubmitWorkItem,
) ([]work.StagedSubmissionItem, error) {
	result := make([]work.StagedSubmissionItem, 0, len(items))
	for i, item := range items {
		mapped, err := stagedSubmissionItemFromGenerated(item)
		if err != nil {
			return nil, requestFieldValidationError{message: fmt.Sprintf("items[%d]: %v", i, err)}
		}
		result = append(result, mapped)
	}
	return result, nil
}

func stagedSubmissionItemFromGenerated(
	item factoryapi.SubmitWorkItem,
) (work.StagedSubmissionItem, error) {
	textItem, textErr := item.AsSubmitWorkTextItem()
	if textErr == nil && textItem.Type == factoryapi.SubmitWorkItemTypeText {
		return work.StagedSubmissionItem{
			ItemType: string(textItem.Type),
			Text:     textItem.Text,
		}, nil
	}

	imageItem, imageErr := item.AsSubmitWorkImageItem()
	if imageErr == nil && imageItem.Type == factoryapi.SubmitWorkItemTypeImage {
		return stagedSubmissionFileItem(
			string(imageItem.Type), imageItem.StagedFileRef,
			imageItem.FileName, imageItem.MediaType,
		), nil
	}

	videoItem, videoErr := item.AsSubmitWorkVideoItem()
	if videoErr == nil && videoItem.Type == factoryapi.SubmitWorkItemTypeVideo {
		return stagedSubmissionFileItem(
			string(videoItem.Type), videoItem.StagedFileRef,
			videoItem.FileName, videoItem.MediaType,
		), nil
	}

	audioItem, audioErr := item.AsSubmitWorkAudioItem()
	if audioErr == nil && audioItem.Type == factoryapi.SubmitWorkItemTypeAudio {
		return stagedSubmissionFileItem(
			string(audioItem.Type), audioItem.StagedFileRef,
			audioItem.FileName, audioItem.MediaType,
		), nil
	}

	documentItem, documentErr := item.AsSubmitWorkDocumentItem()
	if documentErr == nil && documentItem.Type == factoryapi.SubmitWorkItemTypeDocument {
		return stagedSubmissionFileItem(
			string(documentItem.Type), documentItem.StagedFileRef,
			documentItem.FileName, documentItem.MediaType,
		), nil
	}

	return work.StagedSubmissionItem{}, fmt.Errorf("unsupported item type")
}

func stagedSubmissionFileItem(
	itemType string,
	stagedFileRef string,
	fileName string,
	mediaType string,
) work.StagedSubmissionItem {
	return work.StagedSubmissionItem{
		ItemType: itemType, StagedFileRef: stagedFileRef,
		FileName: fileName, MediaType: mediaType,
	}
}

func validateSubmitWorkStructuredInputFields(fields map[string]json.RawMessage) error {
	if _, ok := fields["items"]; !ok {
		return nil
	}
	return validateSubmitWorkItemsField(fields, "")
}

func validateSubmitWorkItemsField(fields map[string]json.RawMessage, prefix string) error {
	itemsRaw, ok := fields["items"]
	if !ok {
		return nil
	}

	var itemPayloads []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &itemPayloads); err != nil {
		return requestFieldValidationError{message: fmt.Sprintf("%sitems must be an array", prefix)}
	}
	if len(itemPayloads) == 0 {
		return nil
	}

	for i, payload := range itemPayloads {
		var itemFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &itemFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%sitems[%d] must be an object", prefix, i)}
		}
		_, err := validateSubmitWorkItemField(itemFields, fmt.Sprintf("%sitems[%d].", prefix, i))
		if err != nil {
			return err
		}
	}

	return nil
}

func validateSubmitWorkItemField(fields map[string]json.RawMessage, prefix string) (bool, error) {
	itemType, err := requiredSubmitWorkItemType(fields, prefix)
	if err != nil {
		return false, err
	}

	switch itemType {
	case factoryapi.SubmitWorkItemTypeText:
		if err := requireOnlyFields(fields, prefix, "type", "text"); err != nil {
			return false, err
		}
		_, err := requiredStringField(fields, prefix, "text", "text items")
		if err != nil {
			return false, err
		}
		return true, nil
	case factoryapi.SubmitWorkItemTypeImage, factoryapi.SubmitWorkItemTypeVideo, factoryapi.SubmitWorkItemTypeAudio, factoryapi.SubmitWorkItemTypeDocument:
		if err := requireOnlyFields(fields, prefix, "type", "url", "stagedFileRef", "fileName", "mediaType"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "url", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "stagedFileRef", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "fileName", string(itemType)+" items"); err != nil {
			return false, err
		}
		if _, err := requiredNonEmptyStringField(fields, prefix, "mediaType", string(itemType)+" items"); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

func requiredSubmitWorkItemType(fields map[string]json.RawMessage, prefix string) (factoryapi.SubmitWorkItemType, error) {
	typeRaw, ok := fields["type"]
	if !ok {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype is required", prefix)}
	}

	var itemType string
	if err := json.Unmarshal(typeRaw, &itemType); err != nil || itemType == "" {
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be a non-empty string", prefix)}
	}

	switch factoryapi.SubmitWorkItemType(itemType) {
	case factoryapi.SubmitWorkItemTypeText,
		factoryapi.SubmitWorkItemTypeImage,
		factoryapi.SubmitWorkItemTypeVideo,
		factoryapi.SubmitWorkItemTypeAudio,
		factoryapi.SubmitWorkItemTypeDocument:
		return factoryapi.SubmitWorkItemType(itemType), nil
	default:
		return "", requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, video, audio, or document", prefix)}
	}
}

func submitWorkResponseFromResult(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	resp := factoryapi.SubmitWorkResponse{
		TraceId:   result.TraceID,
		RequestId: result.RequestID,
		Accepted:  result.Accepted,
	}
	if result.WorkID != "" {
		resp.WorkId = &result.WorkID
	}
	if result.Name != "" {
		resp.Name = &result.Name
	}
	if result.WorkTypeName != "" {
		resp.WorkTypeName = &result.WorkTypeName
	}
	if sessionID != "" {
		resp.SessionId = &sessionID
	}
	return resp
}

// SubmitWorkResponseFromResult maps a canonical submission result to the
// generated HTTP response.
func SubmitWorkResponseFromResult(result work.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
	return submitWorkResponseFromResult(result, sessionID)
}

func (s *Server) SubmitWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	workAPI, ok := s.requireWorkAPI(w)
	if !ok {
		return
	}

	decoded, err := decodeSubmitWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}

	s.submitWorkCore(w, r, decoded.Request, decoded.CanonicalJSON, string(sessionID), func(ctx context.Context, workRequest workdomain.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return workAPI.SubmitWorkRequestForSession(ctx, string(sessionID), workRequest)
	})
}

func (s *Server) submitWorkRequestFromDecoded(
	ctx context.Context,
	req factoryapi.SubmitWorkBySessionIdJSONRequestBody,
) (workdomain.WorkRequest, error) {
	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		return workdomain.WorkRequest{}, err
	}
	content, err := s.submitWorkContent(ctx, req)
	if err != nil {
		return workdomain.WorkRequest{}, err
	}

	submitReq := workdomain.SubmitRequest{
		Name:                   strings.TrimSpace(stringValue(req.Name)),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                stringValue(req.TraceId),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	return workdomain.WorkRequestFromSubmitRequests([]workdomain.SubmitRequest{submitReq}), nil
}

func (s *Server) submitWorkCore(
	w http.ResponseWriter,
	r *http.Request,
	req factoryapi.SubmitWorkBySessionIdJSONRequestBody,
	canonicalJSON []byte,
	sessionID string,
	submit func(context.Context, workdomain.WorkRequest) (work.WorkRequestSubmitResult, error),
) {
	workRequest, err := s.submitWorkRequestFromDecoded(r.Context(), req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	workRequest, err = s.prepareWorkRequest(r.Context(), workRequest, canonicalJSON)
	if err != nil {
		s.writeWorkRequestPreparationError(w, err)
		return
	}

	result, err := submit(r.Context(), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		logFields := []zap.Field{zap.Error(err)}
		if sessionID != "" && sessionID != factorysessions.DefaultSessionID {
			logFields = append(logFields, zap.String("session_id", sessionID))
		}
		s.logger.Error("submit work failed", logFields...)
		s.writeError(w, http.StatusInternalServerError, "failed to submit work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, submitWorkResponseFromResult(result, sessionID))
}

func (s *Server) UpsertWorkRequestBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, requestID string) {
	workAPI, ok := s.requireWorkAPI(w)
	if !ok {
		return
	}

	decoded, err := decodeWorkRequestBody(r.Body)
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
	if decoded.Request.RequestId == "" {
		s.writeError(w, http.StatusBadRequest, "requestId is required", "BAD_REQUEST")
		return
	}
	if decoded.Request.RequestId != requestID {
		s.writeError(w, http.StatusBadRequest, "request_id path and requestId body must match", "BAD_REQUEST")
		return
	}

	s.upsertWorkRequestCore(w, r, decoded.Request, decoded.CanonicalJSON, string(sessionID), func(ctx context.Context, workRequest workdomain.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return workAPI.SubmitWorkRequestForSession(ctx, string(sessionID), workRequest)
	})
}

func (s *Server) upsertWorkRequestCore(
	w http.ResponseWriter,
	r *http.Request,
	req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody,
	canonicalJSON []byte,
	sessionID string,
	submit func(context.Context, workdomain.WorkRequest) (work.WorkRequestSubmitResult, error),
) {
	workRequest, err := generatedWorkRequestToDomain(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	workRequest, err = s.prepareWorkRequest(r.Context(), workRequest, canonicalJSON)
	if err != nil {
		s.writeWorkRequestPreparationError(w, err)
		return
	}

	result, err := submit(r.Context(), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		logFields := []zap.Field{zap.Error(err)}
		if sessionID != "" && sessionID != factorysessions.DefaultSessionID {
			logFields = append(logFields, zap.String("session_id", sessionID))
		}
		s.logger.Error("upsert work request failed", logFields...)
		s.writeError(w, http.StatusInternalServerError, "failed to submit work request", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, upsertWorkRequestResponse(result))
}

func upsertWorkRequestResponse(result work.WorkRequestSubmitResult) factoryapi.UpsertWorkRequestResponse {
	works := make([]factoryapi.UpsertWorkRequestSubmittedWork, 0, len(result.Works))
	for _, work := range result.Works {
		works = append(works, factoryapi.UpsertWorkRequestSubmittedWork{
			Name:         work.Name,
			WorkTypeName: work.WorkTypeName,
			WorkId:       work.WorkID,
		})
	}
	return factoryapi.UpsertWorkRequestResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Works:     works,
	}
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}
func generatedSubmitRelations(values *[]factoryapi.SubmitRelation) []work.Relation {
	if values == nil || len(*values) == 0 {
		return nil
	}
	relations := make([]work.Relation, 0, len(*values))
	for _, relation := range *values {
		relations = append(relations, work.Relation{
			Type:          work.RelationType(relation.Type),
			TargetWorkID:  relation.TargetWorkId,
			RequiredState: stringValue(relation.RequiredState),
		})
	}
	return relations
}

func generatedWorkRequestToDomain(req factoryapi.WorkRequest) (workdomain.WorkRequest, error) {
	workRequest := workdomain.WorkRequest{
		RequestID:              req.RequestId,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		Type:                   workdomain.WorkRequestType(req.Type),
	}
	if req.Works != nil {
		workRequest.Works = make([]workdomain.Work, 0, len(*req.Works))
		for _, work := range *req.Works {
			workRequest.Works = append(workRequest.Works, workdomain.Work{
				Name:                     work.Name,
				WorkID:                   stringValue(work.WorkId),
				RequestID:                stringValue(work.RequestId),
				WorkTypeID:               stringValue(work.WorkTypeName),
				State:                    generatedWorkStateName(work.State),
				ChainingTraceDepth:       intValue(work.ChainingTraceDepth),
				CurrentChainingTraceID:   stringValue(work.CurrentChainingTraceId),
				PreviousChainingTraceIDs: stringSliceValue(work.PreviousChainingTraceIds),
				TraceID:                  stringValue(work.TraceId),
				Content:                  contentcontract.PartsFromGenerated(work.Content),
				Payload:                  work.Payload,
				Tags:                     generatedStringMap(work.Tags),
			})
		}
	}
	if req.Relations != nil {
		workRequest.Relations = make([]work.WorkRelation, 0, len(*req.Relations))
		for _, relation := range *req.Relations {
			workRequest.Relations = append(workRequest.Relations, work.WorkRelation{
				Type:           work.WorkRelationType(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  stringValue(relation.RequiredState),
			})
		}
	}
	return workRequest, nil
}

type decodedSubmitWorkRequest struct {
	Request       factoryapi.SubmitWorkBySessionIdJSONRequestBody
	CanonicalJSON []byte
}

func decodeSubmitWorkRequestBody(body io.Reader) (decodedSubmitWorkRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return decodedSubmitWorkRequest{}, err
	}

	var req factoryapi.SubmitWorkBySessionIdJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	if err := validateSubmitWorkStructuredInputFields(fields); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return decodedSubmitWorkRequest{}, err
	}
	return decodedSubmitWorkRequest{
		Request: req, CanonicalJSON: append([]byte(nil), data...),
	}, nil
}

type decodedUpsertWorkRequest struct {
	Request       factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody
	CanonicalJSON []byte
}

func decodeWorkRequestBody(body io.Reader) (decodedUpsertWorkRequest, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	if !json.Valid(data) {
		var invalid any
		return decodedUpsertWorkRequest{}, json.Unmarshal(data, &invalid)
	}
	decodedData, err := normalizeWorkRequestStateJSON(data)
	if err != nil {
		return decodedUpsertWorkRequest{}, err
	}

	var req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody
	if err := json.Unmarshal(decodedData, &req); err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	var rawRequest struct {
		Works []map[string]json.RawMessage `json:"works"`
	}
	if err := json.Unmarshal(data, &rawRequest); err != nil {
		return decodedUpsertWorkRequest{}, err
	}
	for index := range rawRequest.Works {
		if err := validateWorkContentField(
			rawRequest.Works[index],
			fmt.Sprintf("works[%d].", index),
		); err != nil {
			return decodedUpsertWorkRequest{}, err
		}
	}
	return decodedUpsertWorkRequest{
		Request: req, CanonicalJSON: append([]byte(nil), data...),
	}, nil
}

// normalizeWorkRequestStateJSON preserves the supported legacy string form at
// the handwritten transport boundary while generated contracts remain purely
// mechanical and expose the canonical structured WorkState representation.
func normalizeWorkRequestStateJSON(data []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}

	var works []map[string]json.RawMessage
	if rawWorks, ok := request["works"]; ok {
		if err := json.Unmarshal(rawWorks, &works); err != nil {
			return nil, err
		}
	}
	for _, work := range works {
		rawState, ok := work["state"]
		if !ok {
			continue
		}
		var stateName string
		if err := json.Unmarshal(rawState, &stateName); err != nil {
			continue
		}
		canonicalState, err := json.Marshal(factoryapi.WorkState{
			Name: stateName,
			Type: factoryapi.WorkStateTypePROCESSING,
		})
		if err != nil {
			return nil, err
		}
		work["state"] = canonicalState
	}
	if len(works) > 0 {
		normalizedWorks, err := json.Marshal(works)
		if err != nil {
			return nil, err
		}
		request["works"] = normalizedWorks
	}
	return json.Marshal(request)
}

func (s *Server) prepareWorkRequest(
	ctx context.Context,
	request workdomain.WorkRequest,
	canonicalJSON []byte,
) (workdomain.WorkRequest, error) {
	if s.requestPreparation == nil {
		return workdomain.WorkRequest{}, errors.New("Work Request preparation service is unavailable")
	}
	return s.requestPreparation.PrepareWorkRequest(ctx, work.WorkRequestPreparation{
		Request: request, CanonicalJSON: canonicalJSON,
	})
}

func (s *Server) writeWorkRequestPreparationError(w http.ResponseWriter, err error) {
	var validation *work.RequestPreparationError
	if errors.As(err, &validation) {
		s.writeError(w, http.StatusBadRequest, validation.Message, "BAD_REQUEST")
		return
	}
	s.logger.Error("prepare Work Request failed", zap.Error(err))
	s.writeError(w, http.StatusInternalServerError, "failed to prepare Work Request", "INTERNAL_ERROR")
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

func (s *Server) MoveWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	workAPI, ok := s.requireWorkReadAPI(w)
	if !ok {
		return
	}
	s.handleMoveWork(
		w,
		r,
		string(id),
		func(ctx context.Context, workID, stateName, requestID string) (work.ReadModel, error) {
			return workAPI.MoveWorkAndRead(ctx, string(sessionID), workID, stateName, requestID)
		},
	)
}

type moveWorkInvoker func(ctx context.Context, workID, stateName, requestID string) (work.ReadModel, error)

func (s *Server) handleMoveWork(
	w http.ResponseWriter,
	r *http.Request,
	workID string,
	invoke moveWorkInvoker,
) {
	req, err := decodeMoveWorkRequestBody(r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	stateName := strings.TrimSpace(req.StateName)
	if stateName == "" {
		s.writeError(w, http.StatusBadRequest, "stateName is required", "BAD_REQUEST")
		return
	}

	requestID := strings.TrimSpace(stringValue(req.RequestId))
	result, err := invoke(r.Context(), workID, stateName, requestID)
	if err != nil {
		if status, message, code, ok := moveWorkHTTPError(err); ok {
			s.writeError(w, status, message, code)
			return
		}
		s.logger.Error("move work failed", zap.Error(err), zap.String("work_id", workID))
		s.writeError(w, http.StatusInternalServerError, "failed to move work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusOK, workReadModelToGenerated(result))
}

func decodeMoveWorkRequestBody(body io.Reader) (factoryapi.MoveWorkRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	if len(payload) == 0 {
		return factoryapi.MoveWorkRequest{}, errors.New("request body is required")
	}
	var req factoryapi.MoveWorkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	return req, nil
}

func moveWorkHTTPError(err error) (status int, message, code string, ok bool) {
	switch {
	case errors.Is(err, state.ErrMoveWorkNotFound):
		return http.StatusNotFound, "work not found", "NOT_FOUND", true
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return http.StatusNotFound, "factory session not found", "NOT_FOUND", true
	case errors.Is(err, state.ErrMoveWorkInvalidState):
		return http.StatusBadRequest, "invalid target state for work type", "BAD_REQUEST", true
	case errors.Is(err, state.ErrMoveWorkInFlightDispatch):
		return http.StatusBadRequest, "work is in an active dispatch", "BAD_REQUEST", true
	case errors.Is(err, state.ErrMoveWorkEngineTerminated):
		return http.StatusBadRequest, "engine has terminated", "BAD_REQUEST", true
	case errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied):
		return http.StatusConflict, "Operator move request was already applied.", "MOVE_WORK_REQUEST_ALREADY_APPLIED", true
	default:
		return 0, "", "", false
	}
}
