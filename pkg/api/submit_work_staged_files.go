package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"go.uber.org/zap"
)

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
	stageDir, err := os.MkdirTemp("", "submit-work-stage-*")
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(stageDir, safeSubmitWorkFileName(fileName))
	if err := os.WriteFile(targetPath, content, 0o600); err != nil {
		return "", err
	}
	return targetPath, nil
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
