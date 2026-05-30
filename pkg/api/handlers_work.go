// backendsizecheck:ignore-file consolidated work route handlers (submit, upsert, list, get, staged files); story 006 splits read vs write surfaces to satisfy per-file limits.
// pkgmaintcheck:ignore-file-lines work route handlers extracted from legacy handlers.go during S5 decomposition.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

const defaultMaxResults = 50

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

const (
	submitWorkStagedFileRefPrefix    = "submit-work-stage:v1:"
	submitWorkStagedFileTokenDivider = "."
	submitWorkStageDirPrefix         = "submit-work-stage-"
)

var submitWorkStagedFileRefSecret = mustReadSubmitWorkStagedFileRefSecret()

func (s *Server) StageSubmitWorkFile(w http.ResponseWriter, r *http.Request) {
	response, err := stageSubmitWorkFileRequest(r)
	if err != nil {
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

func (s *Server) StageSubmitWorkFileBySessionId(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	if _, err := sessionRuntime.GetCurrentFactoryForSession(r.Context(), string(sessionID)); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		s.logger.Error("stage submit-work file failed", zap.Error(err))
		s.writeError(w, http.StatusInternalServerError, "failed to stage submit-work file", "INTERNAL_ERROR")
		return
	}

	response, err := stageSubmitWorkFileRequest(r)
	if err != nil {
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

func stageSubmitWorkFileRequest(r *http.Request) (factoryapi.StageSubmitWorkFileResponse, error) {
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

	stagedFileRef, err := writeStagedSubmitWorkFile(content, req.FileName)
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, fmt.Errorf("write staged submit-work file: %w", err)
	}

	return factoryapi.StageSubmitWorkFileResponse{
		FileName:      req.FileName,
		MediaType:     req.MediaType,
		StagedFileRef: stagedFileRef,
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

	fileName, err := requiredNonEmptyStringField(fields, "", "fileName", "submit-work staged files")
	if err != nil {
		return err
	}
	if filepath.Base(fileName) == "." {
		return requestFieldValidationError{message: "fileName must identify a file"}
	}

	mediaType, err := requiredNonEmptyStringField(fields, "", "mediaType", "submit-work staged files")
	if err != nil {
		return err
	}
	if err := validateStageSubmitWorkMediaType(itemType, mediaType); err != nil {
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

func validateStageSubmitWorkMediaType(itemType factoryapi.SubmitWorkItemType, mediaType string) error {
	switch itemType {
	case factoryapi.SubmitWorkItemTypeImage:
		if len(mediaType) >= len("image/") && mediaType[:len("image/")] == "image/" {
			return nil
		}
		return requestFieldValidationError{message: "mediaType must start with image/ for image items"}
	case factoryapi.SubmitWorkItemTypeVideo:
		if len(mediaType) >= len("video/") && mediaType[:len("video/")] == "video/" {
			return nil
		}
		return requestFieldValidationError{message: "mediaType must start with video/ for video items"}
	case factoryapi.SubmitWorkItemTypeAudio:
		if len(mediaType) >= len("audio/") && mediaType[:len("audio/")] == "audio/" {
			return nil
		}
		return requestFieldValidationError{message: "mediaType must start with audio/ for audio items"}
	case factoryapi.SubmitWorkItemTypeDocument:
		if mediaType == "" {
			return requestFieldValidationError{message: "mediaType must be a non-empty string"}
		}
		return nil
	default:
		return requestFieldValidationError{message: "itemType must be one of image, video, audio, or document"}
	}
}

func writeStagedSubmitWorkFile(content []byte, fileName string) (string, error) {
	stageDir, err := os.MkdirTemp("", submitWorkStageDirPrefix+"*")
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(stageDir, safeSubmitWorkFileName(fileName))
	if err := os.WriteFile(targetPath, content, 0o600); err != nil {
		return "", err
	}
	return encodeSubmitWorkStagedFileRef(targetPath), nil
}

func safeSubmitWorkFileName(fileName string) string {
	base := filepath.Base(fileName)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return randomSubmitWorkFileName()
	}
	return base
}

func randomSubmitWorkFileName() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "submit-work-file.bin"
	}
	return "submit-work-" + hex.EncodeToString(buf) + ".bin"
}

func mustReadSubmitWorkStagedFileRefSecret() []byte {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("generate submit-work staged file secret: %v", err))
	}
	return buf
}

func encodeSubmitWorkStagedFileRef(path string) string {
	cleanPath := filepath.Clean(path)
	payload := base64.RawURLEncoding.EncodeToString([]byte(cleanPath))
	signature := submitWorkStagedFileRefSignature(cleanPath)
	return submitWorkStagedFileRefPrefix + payload + submitWorkStagedFileTokenDivider + signature
}

func resolveSubmitWorkStagedFileRef(ref string) (string, error) {
	const invalidMessage = "stagedFileRef must be a backend-issued staged file reference"

	if !strings.HasPrefix(ref, submitWorkStagedFileRefPrefix) {
		return "", requestFieldValidationError{message: invalidMessage}
	}

	unsignedRef := strings.TrimPrefix(ref, submitWorkStagedFileRefPrefix)
	payload, signature, ok := strings.Cut(unsignedRef, submitWorkStagedFileTokenDivider)
	if !ok || payload == "" || signature == "" {
		return "", requestFieldValidationError{message: invalidMessage}
	}

	pathBytes, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", requestFieldValidationError{message: invalidMessage}
	}
	path := string(pathBytes)
	if path == "" {
		return "", requestFieldValidationError{message: invalidMessage}
	}
	if signature != submitWorkStagedFileRefSignature(path) {
		return "", requestFieldValidationError{message: invalidMessage}
	}

	cleanPath := filepath.Clean(path)
	if cleanPath != path || !filepath.IsAbs(cleanPath) {
		return "", requestFieldValidationError{message: invalidMessage}
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(cleanPath)), submitWorkStageDirPrefix) {
		return "", requestFieldValidationError{message: invalidMessage}
	}

	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", requestFieldValidationError{message: "stagedFileRef must reference an existing staged submit-work file"}
	}
	return cleanPath, nil
}

func submitWorkStagedFileRefSignature(path string) string {
	mac := hmac.New(sha256.New, submitWorkStagedFileRefSecret)
	_, _ = mac.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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

	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	content, err := submitWorkContent(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
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
	workRequest := factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

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

	s.writeJSON(w, http.StatusCreated, submitWorkResponseFromResult(result, factorysessions.DefaultSessionID))
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

	payload, err := generatedPayloadToRawMessage(req.Payload)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	content, err := submitWorkContent(req)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
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
	workRequest := factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{submitReq})

	result, err := sessionRuntime.SubmitWorkRequestForSession(r.Context(), string(sessionID), workRequest)
	if err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
			return
		}
		if message, ok := submitWorkBadRequestMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.logger.Error("submit work failed", zap.Error(err), zap.String("session_id", string(sessionID)))
		s.writeError(w, http.StatusInternalServerError, "failed to submit work", "INTERNAL_ERROR")
		return
	}

	s.writeJSON(w, http.StatusCreated, submitWorkResponseFromResult(result, string(sessionID)))
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
