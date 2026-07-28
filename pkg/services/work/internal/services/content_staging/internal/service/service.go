package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	contentstaging "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_staging"
)

const (
	contentStagingRefPrefix = "submit-work-stage:v1:"
	contentStagingDivider   = "."
	contentStagingDirPrefix = "submit-work-stage-"
	defaultContentStageTTL  = time.Hour
)

type Service struct {
	filesystem contentstaging.FileSystem
	random     contentstaging.Random
	clock      contentstaging.Clock
	secret     []byte
	ttl        time.Duration
}

var _ contentstaging.Service = (*Service)(nil)

// New constructs the nested content_staging implementation from exact effects.
func New(
	filesystem contentstaging.FileSystem,
	random contentstaging.Random,
	clock contentstaging.Clock,
	ttl time.Duration,
) (*Service, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("content staging filesystem is required")
	case random == nil:
		return nil, errors.New("content staging randomness is required")
	case clock == nil:
		return nil, errors.New("content staging clock is required")
	}
	if ttl <= 0 {
		ttl = defaultContentStageTTL
	}
	secret := make([]byte, 32)
	if _, err := random.Read(secret); err != nil {
		return nil, fmt.Errorf("generate staged content signing secret: %w", err)
	}
	return &Service{
		filesystem: filesystem,
		random:     random,
		clock:      clock,
		secret:     secret,
		ttl:        ttl,
	}, nil
}

func (s *Service) StageContent(
	ctx context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return work.StageContentResult{}, err
	}
	if err := validateStageContentRequest(request); err != nil {
		return work.StageContentResult{}, err
	}
	stageDir, err := s.filesystem.MkdirTemp("", contentStagingDirPrefix+"*")
	if err != nil {
		return work.StageContentResult{}, fmt.Errorf("create staged content directory: %w", err)
	}
	targetPath := filepath.Join(stageDir, s.safeFileName(request.FileName))
	if err := s.filesystem.WriteFile(targetPath, request.Content, 0o600); err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return work.StageContentResult{}, fmt.Errorf("write staged content: %w", err)
	}
	expiresAt := s.clock.Now().UTC().Add(s.ttl)
	ref, err := s.encodeReference(targetPath, expiresAt)
	if err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return work.StageContentResult{}, err
	}
	contentURL, err := work.FilesystemPathToContentURL(targetPath)
	if err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return work.StageContentResult{}, fmt.Errorf("build staged content URL: %w", err)
	}
	return work.StageContentResult{
		StagedFileRef: ref,
		FileName:      request.FileName,
		MediaType:     request.MediaType,
		URL:           contentURL,
	}, nil
}

func (s *Service) PrepareContent(
	ctx context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []work.WorkContentPart{}, nil
	}
	content := make([]work.WorkContentPart, 0, len(items))
	meaningful := false
	for index, item := range items {
		part, itemMeaningful, err := s.prepareItem(ctx, item)
		if err != nil {
			return nil, &work.ContentStagingError{
				Message: fmt.Sprintf("items[%d]: %v", index, err),
				Cause:   err,
			}
		}
		content = append(content, part)
		meaningful = meaningful || itemMeaningful
	}
	if !meaningful {
		return nil, &work.ContentStagingError{Message: "items must contain at least one non-empty item"}
	}
	return content, nil
}

func (s *Service) prepareItem(
	ctx context.Context,
	item work.StagedSubmissionItem,
) (work.WorkContentPart, bool, error) {
	switch strings.ToLower(strings.TrimSpace(item.ItemType)) {
	case "text":
		return work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: item.Text},
			strings.TrimSpace(item.Text) != "", nil
	case "image", "video", "audio", "document":
	default:
		return work.WorkContentPart{}, false, errors.New("unsupported item type")
	}
	if strings.TrimSpace(item.FileName) == "" {
		return work.WorkContentPart{}, false, errors.New("fileName must identify a file")
	}
	if err := validateStagedContentMediaType(item.ItemType, item.MediaType); err != nil {
		return work.WorkContentPart{}, false, err
	}
	resolved, err := s.ResolveContent(ctx, item.StagedFileRef)
	if err != nil {
		return work.WorkContentPart{}, false, err
	}
	partType := work.WorkContentPartTypeBinary
	switch strings.ToLower(item.ItemType) {
	case "image":
		partType = work.WorkContentPartTypeImage
	case "audio":
		partType = work.WorkContentPartTypeAudio
	}
	return work.WorkContentPart{
		Type:        partType,
		URL:         resolved.URL,
		ContentType: item.MediaType,
		Metadata: map[string]any{
			"submissionItemType": item.ItemType,
			"fileName":           item.FileName,
		},
	}, true, nil
}

func (s *Service) ResolveContent(
	ctx context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return work.ResolvedStagedContent{}, err
	}
	payload, err := s.decodeReference(ref)
	if err != nil {
		return work.ResolvedStagedContent{}, err
	}
	if !payload.ExpiresAt.After(s.clock.Now().UTC()) {
		_ = s.filesystem.RemoveAll(filepath.Dir(payload.Path))
		return work.ResolvedStagedContent{}, &work.ContentStagingError{
			Message: work.ErrStagedContentExpired.Error(),
			Cause:   work.ErrStagedContentExpired,
		}
	}
	info, err := s.filesystem.Stat(payload.Path)
	if err != nil || info.IsDir() {
		return work.ResolvedStagedContent{}, &work.ContentStagingError{
			Message: work.ErrStagedContentNotFound.Error(),
			Cause:   work.ErrStagedContentNotFound,
		}
	}
	contentURL, err := work.FilesystemPathToContentURL(payload.Path)
	if err != nil {
		return work.ResolvedStagedContent{}, fmt.Errorf("build staged content URL: %w", err)
	}
	return work.ResolvedStagedContent{
		Path: payload.Path, URL: contentURL, ExpiresAt: payload.ExpiresAt,
	}, nil
}

func (s *Service) CleanupContent(ctx context.Context, ref string) error {
	if err := requireContentStagingContext(ctx); err != nil {
		return err
	}
	payload, err := s.decodeReference(ref)
	if err != nil {
		return err
	}
	if err := s.filesystem.RemoveAll(filepath.Dir(payload.Path)); err != nil {
		return fmt.Errorf("cleanup staged content: %w", err)
	}
	return nil
}

type stagedContentReferencePayload struct {
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (s *Service) encodeReference(path string, expiresAt time.Time) (string, error) {
	payload, err := json.Marshal(stagedContentReferencePayload{
		Path: filepath.Clean(path), ExpiresAt: expiresAt.UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("encode staged content reference: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return contentStagingRefPrefix + encoded + contentStagingDivider + s.signature(encoded), nil
}

func (s *Service) decodeReference(ref string) (stagedContentReferencePayload, error) {
	invalid := func() (stagedContentReferencePayload, error) {
		return stagedContentReferencePayload{}, &work.ContentStagingError{
			Message: work.ErrInvalidStagedContentRef.Error(),
			Cause:   work.ErrInvalidStagedContentRef,
		}
	}
	if !strings.HasPrefix(ref, contentStagingRefPrefix) {
		return invalid()
	}
	unsigned := strings.TrimPrefix(ref, contentStagingRefPrefix)
	encoded, signature, ok := strings.Cut(unsigned, contentStagingDivider)
	if !ok || encoded == "" || signature == "" ||
		!hmac.Equal([]byte(signature), []byte(s.signature(encoded))) {
		return invalid()
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return invalid()
	}
	var payload stagedContentReferencePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return invalid()
	}
	cleanPath := filepath.Clean(payload.Path)
	if payload.Path == "" || cleanPath != payload.Path || !filepath.IsAbs(cleanPath) ||
		!strings.HasPrefix(filepath.Base(filepath.Dir(cleanPath)), contentStagingDirPrefix) ||
		payload.ExpiresAt.IsZero() {
		return invalid()
	}
	return payload, nil
}

func (s *Service) signature(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) safeFileName(fileName string) string {
	base := filepath.Base(fileName)
	if base != "." && base != string(filepath.Separator) && base != "" {
		return base
	}
	buffer := make([]byte, 8)
	if _, err := s.random.Read(buffer); err != nil {
		return "submit-work-file.bin"
	}
	return "submit-work-" + hex.EncodeToString(buffer) + ".bin"
}

func validateStageContentRequest(request work.StageContentRequest) error {
	itemType := strings.ToLower(strings.TrimSpace(request.ItemType))
	switch itemType {
	case "image", "video", "audio", "document":
	default:
		return &work.ContentStagingError{Message: "itemType must be one of image, video, audio, or document"}
	}
	if strings.TrimSpace(request.FileName) == "" || filepath.Base(request.FileName) == "." {
		return &work.ContentStagingError{Message: "fileName must identify a file"}
	}
	if err := validateStagedContentMediaType(itemType, request.MediaType); err != nil {
		return &work.ContentStagingError{Message: err.Error(), Cause: err}
	}
	if len(request.Content) == 0 {
		return &work.ContentStagingError{Message: "contentBase64 must decode to a non-empty file payload"}
	}
	return nil
}

func validateStagedContentMediaType(itemType, rawMediaType string) error {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	mediaType := strings.TrimSpace(rawMediaType)
	switch {
	case mediaType == "":
		return errors.New("mediaType must be a non-empty string")
	case itemType == "image" && !strings.HasPrefix(mediaType, "image/"):
		return errors.New("mediaType must start with image/ for image items")
	case itemType == "video" && !strings.HasPrefix(mediaType, "video/"):
		return errors.New("mediaType must start with video/ for video items")
	case itemType == "audio" && !strings.HasPrefix(mediaType, "audio/"):
		return errors.New("mediaType must start with audio/ for audio items")
	}
	return nil
}

func requireContentStagingContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("content staging context is required")
	}
	return ctx.Err()
}
