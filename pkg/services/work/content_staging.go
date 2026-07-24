package work

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

const (
	contentStagingRefPrefix = "submit-work-stage:v1:"
	contentStagingDivider   = "."
	contentStagingDirPrefix = "submit-work-stage-"
	defaultContentStageTTL  = time.Hour
)

var (
	ErrInvalidStagedContentRef = errors.New("stagedFileRef must be a backend-issued staged file reference")
	ErrStagedContentNotFound   = errors.New("stagedFileRef must reference an existing staged submit-work file")
	ErrStagedContentExpired    = errors.New("stagedFileRef has expired")
)

// ContentStagingFileSystem is the exact filesystem effect used to own staged
// submit-Work content.
type ContentStagingFileSystem interface {
	MkdirTemp(string, string) (string, error)
	WriteFile(string, []byte, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
	RemoveAll(string) error
}

// ContentStagingRandom supplies cryptographic entropy for signing keys and
// fallback file names.
type ContentStagingRandom interface {
	Read([]byte) (int, error)
}

// ContentStagingClock supplies issuance and expiry time.
type ContentStagingClock interface {
	Now() time.Time
}

// ContentStagingService is the focused Work staging role. The published peer
// root exposes the same operations on Service; transports may still inject this
// narrower role until nested IMP-WORK cuts fold injection onto the root.
type ContentStagingService interface {
	StageContent(context.Context, StageContentRequest) (StageContentResult, error)
	PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error)
	ResolveContent(context.Context, string) (ResolvedStagedContent, error)
	CleanupContent(context.Context, string) error
}

// StageContentRequest is the plain Work-owned staging request contract.
type StageContentRequest struct {
	ItemType  string
	FileName  string
	MediaType string
	Content   []byte
}

// StageContentResult is the plain Work-owned staging result. StagedFileRef is an
// opaque reference peers pass back through prepare/resolve/cleanup without
// observing staging implementation types.
type StageContentResult struct {
	StagedFileRef string
	FileName      string
	MediaType     string
	URL           string
}

type StagedSubmissionItem struct {
	ItemType      string
	Text          string
	StagedFileRef string
	FileName      string
	MediaType     string
}

type ResolvedStagedContent struct {
	Path      string
	URL       string
	ExpiresAt time.Time
}

// ContentStagingError is a customer-safe Work validation failure.
type ContentStagingError struct {
	Message string
	Cause   error
}

func (e *ContentStagingError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *ContentStagingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type contentStagingService struct {
	filesystem ContentStagingFileSystem
	random     ContentStagingRandom
	clock      ContentStagingClock
	secret     []byte
	ttl        time.Duration
}

// NewContentStagingService constructs the Work staging service from exact
// effects. Wire is the production caller.
func NewContentStagingService(
	filesystem ContentStagingFileSystem,
	random ContentStagingRandom,
	clock ContentStagingClock,
	ttl time.Duration,
) (ContentStagingService, error) {
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
	return &contentStagingService{
		filesystem: filesystem,
		random:     random,
		clock:      clock,
		secret:     secret,
		ttl:        ttl,
	}, nil
}

func (s *contentStagingService) StageContent(
	ctx context.Context,
	request StageContentRequest,
) (StageContentResult, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return StageContentResult{}, err
	}
	if err := validateStageContentRequest(request); err != nil {
		return StageContentResult{}, err
	}
	stageDir, err := s.filesystem.MkdirTemp("", contentStagingDirPrefix+"*")
	if err != nil {
		return StageContentResult{}, fmt.Errorf("create staged content directory: %w", err)
	}
	targetPath := filepath.Join(stageDir, s.safeFileName(request.FileName))
	if err := s.filesystem.WriteFile(targetPath, request.Content, 0o600); err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return StageContentResult{}, fmt.Errorf("write staged content: %w", err)
	}
	expiresAt := s.clock.Now().UTC().Add(s.ttl)
	ref, err := s.encodeReference(targetPath, expiresAt)
	if err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return StageContentResult{}, err
	}
	contentURL, err := FilesystemPathToContentURL(targetPath)
	if err != nil {
		_ = s.filesystem.RemoveAll(stageDir)
		return StageContentResult{}, fmt.Errorf("build staged content URL: %w", err)
	}
	return StageContentResult{
		StagedFileRef: ref,
		FileName:      request.FileName,
		MediaType:     request.MediaType,
		URL:           contentURL,
	}, nil
}

func (s *contentStagingService) PrepareContent(
	ctx context.Context,
	items []StagedSubmissionItem,
) ([]WorkContentPart, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []WorkContentPart{}, nil
	}
	content := make([]WorkContentPart, 0, len(items))
	meaningful := false
	for index, item := range items {
		part, itemMeaningful, err := s.prepareItem(ctx, item)
		if err != nil {
			return nil, &ContentStagingError{
				Message: fmt.Sprintf("items[%d]: %v", index, err),
				Cause:   err,
			}
		}
		content = append(content, part)
		meaningful = meaningful || itemMeaningful
	}
	if !meaningful {
		return nil, &ContentStagingError{Message: "items must contain at least one non-empty item"}
	}
	return content, nil
}

func (s *contentStagingService) prepareItem(
	ctx context.Context,
	item StagedSubmissionItem,
) (WorkContentPart, bool, error) {
	switch strings.ToLower(strings.TrimSpace(item.ItemType)) {
	case "text":
		return WorkContentPart{Type: WorkContentPartTypeText, Text: item.Text},
			strings.TrimSpace(item.Text) != "", nil
	case "image", "video", "audio", "document":
	default:
		return WorkContentPart{}, false, errors.New("unsupported item type")
	}
	if strings.TrimSpace(item.FileName) == "" {
		return WorkContentPart{}, false, errors.New("fileName must identify a file")
	}
	if err := validateStagedContentMediaType(item.ItemType, item.MediaType); err != nil {
		return WorkContentPart{}, false, err
	}
	resolved, err := s.ResolveContent(ctx, item.StagedFileRef)
	if err != nil {
		return WorkContentPart{}, false, err
	}
	partType := WorkContentPartTypeBinary
	switch strings.ToLower(item.ItemType) {
	case "image":
		partType = WorkContentPartTypeImage
	case "audio":
		partType = WorkContentPartTypeAudio
	}
	return WorkContentPart{
		Type:        partType,
		URL:         resolved.URL,
		ContentType: item.MediaType,
		Metadata: map[string]any{
			"submissionItemType": item.ItemType,
			"fileName":           item.FileName,
		},
	}, true, nil
}

func (s *contentStagingService) ResolveContent(
	ctx context.Context,
	ref string,
) (ResolvedStagedContent, error) {
	if err := requireContentStagingContext(ctx); err != nil {
		return ResolvedStagedContent{}, err
	}
	payload, err := s.decodeReference(ref)
	if err != nil {
		return ResolvedStagedContent{}, err
	}
	if !payload.ExpiresAt.After(s.clock.Now().UTC()) {
		_ = s.filesystem.RemoveAll(filepath.Dir(payload.Path))
		return ResolvedStagedContent{}, &ContentStagingError{
			Message: ErrStagedContentExpired.Error(),
			Cause:   ErrStagedContentExpired,
		}
	}
	info, err := s.filesystem.Stat(payload.Path)
	if err != nil || info.IsDir() {
		return ResolvedStagedContent{}, &ContentStagingError{
			Message: ErrStagedContentNotFound.Error(),
			Cause:   ErrStagedContentNotFound,
		}
	}
	contentURL, err := FilesystemPathToContentURL(payload.Path)
	if err != nil {
		return ResolvedStagedContent{}, fmt.Errorf("build staged content URL: %w", err)
	}
	return ResolvedStagedContent{
		Path: payload.Path, URL: contentURL, ExpiresAt: payload.ExpiresAt,
	}, nil
}

func (s *contentStagingService) CleanupContent(ctx context.Context, ref string) error {
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

func (s *contentStagingService) encodeReference(path string, expiresAt time.Time) (string, error) {
	payload, err := json.Marshal(stagedContentReferencePayload{
		Path: filepath.Clean(path), ExpiresAt: expiresAt.UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("encode staged content reference: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return contentStagingRefPrefix + encoded + contentStagingDivider + s.signature(encoded), nil
}

func (s *contentStagingService) decodeReference(ref string) (stagedContentReferencePayload, error) {
	invalid := func() (stagedContentReferencePayload, error) {
		return stagedContentReferencePayload{}, &ContentStagingError{
			Message: ErrInvalidStagedContentRef.Error(),
			Cause:   ErrInvalidStagedContentRef,
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

func (s *contentStagingService) signature(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *contentStagingService) safeFileName(fileName string) string {
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

func validateStageContentRequest(request StageContentRequest) error {
	itemType := strings.ToLower(strings.TrimSpace(request.ItemType))
	switch itemType {
	case "image", "video", "audio", "document":
	default:
		return &ContentStagingError{Message: "itemType must be one of image, video, audio, or document"}
	}
	if strings.TrimSpace(request.FileName) == "" || filepath.Base(request.FileName) == "." {
		return &ContentStagingError{Message: "fileName must identify a file"}
	}
	if err := validateStagedContentMediaType(itemType, request.MediaType); err != nil {
		return &ContentStagingError{Message: err.Error(), Cause: err}
	}
	if len(request.Content) == 0 {
		return &ContentStagingError{Message: "contentBase64 must decode to a non-empty file payload"}
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
