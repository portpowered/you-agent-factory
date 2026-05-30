package api

import (
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
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

const (
	submitWorkItemTypeMetadataKey = "submissionItemType"
	submitWorkFileNameMetadataKey = "fileName"
	submitWorkStagedFileRefPrefix = "submit-work-stage:v1:"
	submitWorkStagedFileTokenDivider = "."
	submitWorkStageDirPrefix         = "submit-work-stage-"
)

var submitWorkStagedFileRefSecret = mustReadSubmitWorkStagedFileRefSecret()

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
