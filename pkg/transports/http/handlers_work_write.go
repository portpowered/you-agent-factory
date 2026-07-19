package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workdomain "github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/work/content"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	"go.uber.org/zap"
)

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
)

func (s *Server) InvokeFactorySessionBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID) {
	req, err := decodeInvocationRequestBody(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	if s.sessionRuntime == nil {
		s.writeError(w, http.StatusInternalServerError, "session invocation API is unavailable", "INTERNAL_ERROR")
		return
	}

	result, err := s.sessionRuntime.InvokeFactorySession(r.Context(), string(sessionID), req)
	if err != nil {
		switch typed := err.(type) {
		case *invocations.InputError:
			s.writeError(w, http.StatusBadRequest, typed.Message, string(typed.Code))
		case *invocations.ArgumentError:
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

func submitWorkContent(req factoryapi.SubmitWorkRequest) ([]work.WorkContentPart, error) {
	if req.Items == nil {
		return contentcontract.PartsFromGenerated(req.Content), nil
	}
	return submitWorkItemsToContent(*req.Items)
}

func submitWorkItemsToContent(items []factoryapi.SubmitWorkItem) ([]work.WorkContentPart, error) {
	if len(items) == 0 {
		return []work.WorkContentPart{}, nil
	}

	content := make([]work.WorkContentPart, 0, len(items))
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

func submitWorkItemToContentPart(item factoryapi.SubmitWorkItem) (work.WorkContentPart, bool, error) {
	textItem, textErr := item.AsSubmitWorkTextItem()
	if textErr == nil && textItem.Type == factoryapi.SubmitWorkItemTypeText {
		return work.WorkContentPart{
			Type: work.WorkContentPartTypeText,
			Text: textItem.Text,
		}, strings.TrimSpace(textItem.Text) != "", nil
	}

	imageItem, imageErr := item.AsSubmitWorkImageItem()
	if imageErr == nil && imageItem.Type == factoryapi.SubmitWorkItemTypeImage {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(imageItem.StagedFileRef)
		if err != nil {
			return work.WorkContentPart{}, false, err
		}
		part, err := submitWorkStagedFileItemContentPart(
			work.WorkContentPartTypeImage,
			string(imageItem.Type),
			stagedFilePath,
			imageItem.FileName,
			imageItem.MediaType,
		)
		return part, true, err
	}

	videoItem, videoErr := item.AsSubmitWorkVideoItem()
	if videoErr == nil && videoItem.Type == factoryapi.SubmitWorkItemTypeVideo {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(videoItem.StagedFileRef)
		if err != nil {
			return work.WorkContentPart{}, false, err
		}
		part, err := submitWorkStagedFileItemContentPart(
			work.WorkContentPartTypeBinary,
			string(videoItem.Type),
			stagedFilePath,
			videoItem.FileName,
			videoItem.MediaType,
		)
		return part, true, err
	}

	audioItem, audioErr := item.AsSubmitWorkAudioItem()
	if audioErr == nil && audioItem.Type == factoryapi.SubmitWorkItemTypeAudio {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(audioItem.StagedFileRef)
		if err != nil {
			return work.WorkContentPart{}, false, err
		}
		part, err := submitWorkStagedFileItemContentPart(
			work.WorkContentPartTypeAudio,
			string(audioItem.Type),
			stagedFilePath,
			audioItem.FileName,
			audioItem.MediaType,
		)
		return part, true, err
	}

	documentItem, documentErr := item.AsSubmitWorkDocumentItem()
	if documentErr == nil && documentItem.Type == factoryapi.SubmitWorkItemTypeDocument {
		stagedFilePath, err := resolveSubmitWorkStagedFileRef(documentItem.StagedFileRef)
		if err != nil {
			return work.WorkContentPart{}, false, err
		}
		part, err := submitWorkStagedFileItemContentPart(
			work.WorkContentPartTypeBinary,
			string(documentItem.Type),
			stagedFilePath,
			documentItem.FileName,
			documentItem.MediaType,
		)
		return part, true, err
	}

	return work.WorkContentPart{}, false, fmt.Errorf("unsupported item type")
}

func submitWorkStagedFileItemContentPart(
	partType work.WorkContentPartType,
	itemType string,
	stagedFilePath string,
	fileName string,
	mediaType string,
) (work.WorkContentPart, error) {
	contentURL, err := content.FilesystemPathToContentURL(stagedFilePath)
	if err != nil {
		return work.WorkContentPart{}, err
	}
	return work.WorkContentPart{
		Type:        partType,
		URL:         contentURL,
		ContentType: mediaType,
		Metadata: map[string]any{
			submitWorkItemTypeMetadataKey: itemType,
			submitWorkFileNameMetadataKey: fileName,
		},
	}, nil
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
		if err := requireOnlyFields(fields, prefix, "type", "url", "stagedFileRef", "fileName", "mediaType"); err != nil {
			return false, err
		}
		contentURL, err := requiredNonEmptyStringField(fields, prefix, "url", string(itemType)+" items")
		if err != nil {
			return false, err
		}
		if err := content.ValidateContentURL(contentURL); err != nil {
			return false, requestFieldValidationError{message: fmt.Sprintf("%surl %s", prefix, err.Error())}
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

	s.submitWorkCore(w, r, req, string(sessionID), func(ctx context.Context, workRequest workdomain.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return sessionRuntime.SubmitWorkRequestForSession(ctx, string(sessionID), workRequest)
	})
}

func submitWorkRequestFromDecoded(req factoryapi.SubmitWorkBySessionIdJSONRequestBody) (workdomain.WorkRequest, error) {
	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		return workdomain.WorkRequest{}, err
	}
	content, err := submitWorkContent(req)
	if err != nil {
		return workdomain.WorkRequest{}, err
	}

	submitReq := workdomain.SubmitRequest{
		Name:                   strings.TrimSpace(stringValue(req.Name)),
		WorkTypeID:             req.WorkTypeName,
		CurrentChainingTraceID: stringValue(req.CurrentChainingTraceId),
		TraceID:                factoryrequests.ResolveWorkRequestCurrentChainingTraceID(stringValue(req.CurrentChainingTraceId), stringValue(req.TraceId)),
		Content:                content,
		Payload:                payload,
		Tags:                   generatedStringMap(req.Tags),
		Relations:              generatedSubmitRelations(req.Relations),
	}
	return factoryrequests.WorkRequestFromSubmitRequests([]workdomain.SubmitRequest{submitReq}), nil
}

func (s *Server) submitWorkCore(
	w http.ResponseWriter,
	r *http.Request,
	req factoryapi.SubmitWorkBySessionIdJSONRequestBody,
	sessionID string,
	submit func(context.Context, workdomain.WorkRequest) (work.WorkRequestSubmitResult, error),
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

	s.upsertWorkRequestCore(w, r, req, string(sessionID), func(ctx context.Context, workRequest workdomain.WorkRequest) (work.WorkRequestSubmitResult, error) {
		return sessionRuntime.SubmitWorkRequestForSession(ctx, string(sessionID), workRequest)
	})
}

func (s *Server) upsertWorkRequestCore(
	w http.ResponseWriter,
	r *http.Request,
	req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody,
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
	applyStableTraceToWorkRequest(&workRequest)

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
		for i, work := range *req.Works {
			if err := validateGeneratedWorkContentAtPath(work.Content, fmt.Sprintf("works[%d].content", i)); err != nil {
				return workdomain.WorkRequest{}, err
			}
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

func validateGeneratedWorkContentAtPath(content *factoryapi.WorkContent, fieldPath string) error {
	if content == nil || len(*content) == 0 {
		return nil
	}

	for i, part := range *content {
		pathPrefix := fmt.Sprintf("%s[%d].", fieldPath, i)
		if _, ok := contentcontract.PartFromGenerated(part); ok {
			continue
		}

		return requestFieldValidationError{message: fmt.Sprintf("%stype must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY", pathPrefix)}
	}
	return nil
}

func decodeSubmitWorkRequestBody(body io.Reader) (factoryapi.SubmitWorkBySessionIdJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, err
	}

	var req factoryapi.SubmitWorkBySessionIdJSONRequestBody
	if err := json.Unmarshal(data, &req); err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, err
	}
	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, requestFieldValidationError{message: err.Error()}
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, err
	}
	if err := validateSubmitWorkStructuredInputFields(fields); err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.SubmitWorkBySessionIdJSONRequestBody{}, err
	}
	return req, nil
}

func decodeWorkRequestBody(body io.Reader) (factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, err
	}
	if !json.Valid(data) {
		var invalid any
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, json.Unmarshal(data, &invalid)
	}

	if err := validateCanonicalWorkRequestJSONForAPI(data); err != nil {
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, requestFieldValidationError{message: err.Error()}
	}
	decodedData, err := normalizeWorkRequestStateJSON(data)
	if err != nil {
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, err
	}

	var req factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody
	if err := json.Unmarshal(decodedData, &req); err != nil {
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, err
	}

	if req.Works == nil || len(*req.Works) == 0 {
		return req, nil
	}

	var rawRequest struct {
		Works []map[string]json.RawMessage `json:"works"`
	}
	if err := json.Unmarshal(data, &rawRequest); err != nil {
		return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, err
	}

	for i := range *req.Works {
		if i >= len(rawRequest.Works) {
			return req, nil
		}
		if err := validateWorkContentField(rawRequest.Works[i], fmt.Sprintf("works[%d].", i)); err != nil {
			return factoryapi.UpsertWorkRequestBySessionIdJSONRequestBody{}, err
		}
	}
	return req, nil
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

func applyStableTraceToWorkRequest(req *workdomain.WorkRequest) {
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

func (s *Server) MoveWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.handleMoveWork(
		w,
		r,
		string(id),
		func(ctx context.Context, workID, stateName, requestID string) (work.OperatorMoveResult, error) {
			return sessionRuntime.MoveWorkForSession(ctx, string(sessionID), workID, stateName, requestID)
		},
		func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
			return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
		},
	)
}

type moveWorkInvoker func(ctx context.Context, workID, stateName, requestID string) (work.OperatorMoveResult, error)

type moveWorkSnapshotLoader func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)

func (s *Server) handleMoveWork(
	w http.ResponseWriter,
	r *http.Request,
	workID string,
	invoke moveWorkInvoker,
	loadSnapshot moveWorkSnapshotLoader,
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
	if _, err := invoke(r.Context(), workID, stateName, requestID); err != nil {
		if status, message, code, ok := moveWorkHTTPError(err); ok {
			s.writeError(w, status, message, code)
			return
		}
		s.logger.Error("move work failed", zap.Error(err), zap.String("work_id", workID))
		s.writeError(w, http.StatusInternalServerError, "failed to move work", "INTERNAL_ERROR")
		return
	}

	snapshot, err := loadSnapshot(r.Context())
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("get engine state snapshot after move failed", zap.Error(err), zap.String("work_id", workID))
		s.writeError(w, http.StatusInternalServerError, "failed to get work after move", "INTERNAL_ERROR")
		return
	}

	materialized := materialize.CollectPublicWorkTokens(&snapshot.Marking, snapshot.Dispatches)
	token, inFlightOnly, ok := findPublicWorkToken(materialized, workID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
		return
	}
	workNamesByID := publicWorkNamesByID(materialized.Tokens)
	work := tokenToWork(token, snapshot.Topology, inFlightOnly)
	work.Relations = generatedWorkRelations(token, work.Name, workNamesByID)
	s.writeJSON(w, http.StatusOK, work)
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
	case errors.Is(err, engine.ErrMoveWorkNotFound):
		return http.StatusNotFound, "work not found", "NOT_FOUND", true
	case errors.Is(err, apisurface.ErrFactorySessionNotFound):
		return http.StatusNotFound, "factory session not found", "NOT_FOUND", true
	case errors.Is(err, engine.ErrMoveWorkInvalidState):
		return http.StatusBadRequest, "invalid target state for work type", "BAD_REQUEST", true
	case errors.Is(err, engine.ErrMoveWorkInFlightDispatch):
		return http.StatusBadRequest, "work is in an active dispatch", "BAD_REQUEST", true
	case errors.Is(err, engine.ErrMoveWorkEngineTerminated):
		return http.StatusBadRequest, "engine has terminated", "BAD_REQUEST", true
	case errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied):
		return http.StatusConflict, "Operator move request was already applied.", "MOVE_WORK_REQUEST_ALREADY_APPLIED", true
	default:
		return 0, "", "", false
	}
}
