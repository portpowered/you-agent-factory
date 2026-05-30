package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
)

func submitWorkContent(req factoryapi.SubmitWorkRequest) ([]interfaces.WorkContentPart, error) {
	if req.Items == nil {
		return workcontent.PartsFromGenerated(req.Content), nil
	}
	return submitWorkItemsToContent(*req.Items)
}

func submitWorkItemsToContent(items []factoryapi.SubmitWorkItem) ([]interfaces.WorkContentPart, error) {
	if len(items) == 0 {
		return []interfaces.WorkContentPart{}, nil
	}

	content := make([]interfaces.WorkContentPart, 0, len(items))
	hasMeaningfulItem := false
	for i, item := range items {
		part, meaningful, err := submitWorkItemToContentPart(item)
		if err != nil {
			return nil, requestFieldValidationError{message: fmt.Sprintf("items[%d]: %v", i, err)}
		}
		content = append(content, part)
		hasMeaningfulItem = hasMeaningfulItem || meaningful
	}
	if !hasMeaningfulItem {
		return nil, requestFieldValidationError{message: "items must contain at least one non-empty item"}
	}

	return content, nil
}

func submitWorkItemToContentPart(item factoryapi.SubmitWorkItem) (interfaces.WorkContentPart, bool, error) {
	textItem, textErr := item.AsSubmitWorkTextItem()
	if textErr == nil && textItem.Type == factoryapi.SubmitWorkItemTypeText {
		return interfaces.WorkContentPart{
			Type: interfaces.WorkContentPartTypeText,
			Text: textItem.Text,
		}, strings.TrimSpace(textItem.Text) != "", nil
	}

	imageItem, imageErr := item.AsSubmitWorkImageItem()
	if imageErr == nil && imageItem.Type == factoryapi.SubmitWorkItemTypeImage {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(imageItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeImage,
			string(imageItem.Type),
			stagedFilePath,
			imageItem.FileName,
			imageItem.MediaType,
		), true, nil
	}

	videoItem, videoErr := item.AsSubmitWorkVideoItem()
	if videoErr == nil && videoItem.Type == factoryapi.SubmitWorkItemTypeVideo {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(videoItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeBinary,
			string(videoItem.Type),
			stagedFilePath,
			videoItem.FileName,
			videoItem.MediaType,
		), true, nil
	}

	audioItem, audioErr := item.AsSubmitWorkAudioItem()
	if audioErr == nil && audioItem.Type == factoryapi.SubmitWorkItemTypeAudio {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(audioItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeAudio,
			string(audioItem.Type),
			stagedFilePath,
			audioItem.FileName,
			audioItem.MediaType,
		), true, nil
	}

	documentItem, documentErr := item.AsSubmitWorkDocumentItem()
	if documentErr == nil && documentItem.Type == factoryapi.SubmitWorkItemTypeDocument {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(documentItem.StagedFileRef)
		if err != nil {
			return interfaces.WorkContentPart{}, false, err
		}
		return submitWorkFileItemContentPart(
			interfaces.WorkContentPartTypeBinary,
			string(documentItem.Type),
			stagedFilePath,
			documentItem.FileName,
			documentItem.MediaType,
		), true, nil
	}

	return interfaces.WorkContentPart{}, false, fmt.Errorf("unsupported item type")
}

func submitWorkFileItemContentPart(
	partType interfaces.WorkContentPartType,
	itemType string,
	stagedFileRef string,
	fileName string,
	mediaType string,
) interfaces.WorkContentPart {
	return interfaces.WorkContentPart{
		Type:        partType,
		File:        stagedFileRef,
		ContentType: mediaType,
		Metadata: map[string]any{
			submitWorkItemTypeMetadataKey: itemType,
			submitWorkFileNameMetadataKey: fileName,
		},
	}
}

func validateSubmitWorkStructuredInputFields(fields map[string]json.RawMessage) error {
	if _, ok := fields["items"]; !ok {
		return nil
	}
	if _, ok := fields["content"]; ok {
		return requestFieldValidationError{message: "items cannot be combined with content"}
	}
	if _, ok := fields["payload"]; ok {
		return requestFieldValidationError{message: "items cannot be combined with payload"}
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

	hasMeaningfulItem := false
	for i, payload := range itemPayloads {
		var itemFields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &itemFields); err != nil {
			return requestFieldValidationError{message: fmt.Sprintf("%sitems[%d] must be an object", prefix, i)}
		}
		meaningful, err := validateSubmitWorkItemField(itemFields, fmt.Sprintf("%sitems[%d].", prefix, i))
		if err != nil {
			return err
		}
		hasMeaningfulItem = hasMeaningfulItem || meaningful
	}
	if !hasMeaningfulItem {
		return requestFieldValidationError{message: fmt.Sprintf("%sitems must contain at least one non-empty item", prefix)}
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
		text, err := requiredStringField(fields, prefix, "text", "text items")
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(text) != "", nil
	case factoryapi.SubmitWorkItemTypeImage, factoryapi.SubmitWorkItemTypeVideo, factoryapi.SubmitWorkItemTypeAudio, factoryapi.SubmitWorkItemTypeDocument:
		if err := requireOnlyFields(fields, prefix, "type", "stagedFileRef", "fileName", "mediaType"); err != nil {
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

func submitWorkResponseFromResult(result interfaces.WorkRequestSubmitResult, sessionID string) factoryapi.SubmitWorkResponse {
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

	s.submitWorkCore(w, r, req, factorysessions.DefaultSessionID, s.runtime.SubmitWorkRequest)
}

func (s *Server) SubmitWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
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

	s.submitWorkCore(w, r, req, string(sessionID), func(ctx context.Context, workRequest interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
		return sessionRuntime.SubmitWorkRequestForSession(ctx, string(sessionID), workRequest)
	})
}

func submitWorkRequestFromDecoded(req factoryapi.SubmitWorkJSONRequestBody) (interfaces.WorkRequest, error) {
	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		return interfaces.WorkRequest{}, err
	}
	content, err := submitWorkContent(req)
	if err != nil {
		return interfaces.WorkRequest{}, err
	}

	submitReq := interfaces.SubmitRequest{
		Name:                   strings.TrimSpace(req.Name),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                factoryrequests.ResolveWorkRequestCurrentChainingTraceID(stringValue(req.CurrentChainingTraceId), stringValue(req.TraceId)),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	return factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq}), nil
}

func (s *Server) submitWorkCore(
	w http.ResponseWriter,
	r *http.Request,
	req factoryapi.SubmitWorkJSONRequestBody,
	sessionID string,
	submit func(context.Context, interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error),
) {
	workRequest, err := submitWorkRequestFromDecoded(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
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

	s.writeJSON(w, http.StatusCreated, upsertWorkRequestResponse(result))
}

func (s *Server) UpsertWorkRequestBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, requestID string) {
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
	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if strings.HasPrefix(err.Error(), "work_request:") {
			s.writeError(w, http.StatusBadRequest, submitWorkTypeNameMessage(err.Error()), "BAD_REQUEST")
			return
		}
		s.logger.Error("upsert work request failed", zap.Error(err), zap.String("session_id", string(sessionID)))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work request", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, upsertWorkRequestResponse(result))
}

func upsertWorkRequestResponse(result interfaces.WorkRequestSubmitResult) factoryapi.UpsertWorkRequestResponse {
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
	if err := validateSubmitWorkStructuredInputFields(fields); err != nil {
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
