package requestadmission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/contenturl"
)

type RequestPreparationService interface {
	PrepareWorkRequest(context.Context, WorkRequestPreparation) (Request, error)
}

type ContentPreparation interface {
	PrepareWorkContent(context.Context, []ContentPart) ([]ContentPart, error)
}

type RequestPreparationError struct {
	Message string
	Cause   error
}

func (e *RequestPreparationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *RequestPreparationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type contentPreparationService struct{}

func NewContentPreparation() ContentPreparation { return contentPreparationService{} }

func (contentPreparationService) PrepareWorkContent(
	ctx context.Context,
	content []ContentPart,
) ([]ContentPart, error) {
	if ctx == nil {
		return nil, errors.New("Work content preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := prepareCanonicalWorkContent(content)
	if err != nil {
		return nil, requestPreparationError(err)
	}
	return prepared, nil
}

type requestPreparationService struct {
	content ContentPreparation
}

func NewRequestPreparationService(content ContentPreparation) (RequestPreparationService, error) {
	if content == nil {
		return nil, errors.New("Work content preparation is required")
	}
	return requestPreparationService{content: content}, nil
}

func (s requestPreparationService) PrepareWorkRequest(
	ctx context.Context,
	input WorkRequestPreparation,
) (Request, error) {
	if ctx == nil {
		return Request{}, errors.New("Work Request preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return Request{}, err
	}
	if len(input.CanonicalJSON) > 0 {
		if err := validateWorkRequestPublicJSON(input.CanonicalJSON); err != nil {
			return Request{}, requestPreparationError(err)
		}
	}

	request, err := cloneRequest(input.Request)
	if err != nil {
		return Request{}, requestPreparationError(fmt.Errorf("payload: %w", err))
	}
	applyDefaultBatchWorkTypes(&request, input.DefaultWorkTypeID)
	applyStableWorkRequestLineage(&request)
	for index := range request.Works {
		request.Works[index].Name = strings.TrimSpace(request.Works[index].Name)
		if request.Works[index].Name == "" {
			if request.RequestID == "" && len(request.Works) == 1 {
				return Request{}, requestPreparationError(errors.New("name is required"))
			}
			return Request{}, requestPreparationError(fmt.Errorf(
				"work_request: works[%d] is missing required name", index,
			))
		}
		if strings.TrimSpace(request.Works[index].WorkTypeID) == "" {
			if request.RequestID == "" && len(request.Works) == 1 {
				return Request{}, requestPreparationError(errors.New("workTypeName is required"))
			}
			return Request{}, requestPreparationError(fmt.Errorf(
				"work_request: works[%d] (%q) is missing workTypeName",
				index,
				request.Works[index].Name,
			))
		}
		content, err := s.content.PrepareWorkContent(ctx, request.Works[index].Content)
		if err != nil {
			return Request{}, requestPreparationError(fmt.Errorf(
				"works[%d].%s", index, err.Error(),
			))
		}
		request.Works[index].Content = content
	}
	return request, nil
}

type publicWorkRequestSubmissionShape struct {
	Content json.RawMessage `json:"content"`
	Items   json.RawMessage `json:"items"`
	Payload json.RawMessage `json:"payload"`
}

func validateWorkRequestPublicJSON(data []byte) error {
	if err := ValidateCanonicalWorkRequestJSON(data); err != nil {
		return err
	}
	var shape publicWorkRequestSubmissionShape
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	if shape.Items != nil && shape.Content != nil {
		return errors.New("items cannot be combined with content")
	}
	if shape.Items != nil && shape.Payload != nil {
		return errors.New("items cannot be combined with payload")
	}
	return nil
}

func prepareCanonicalWorkContent(content []ContentPart) ([]ContentPart, error) {
	if len(content) == 0 {
		return nil, nil
	}
	prepared := cloneContentParts(content)
	meaningful := false
	for index := range prepared {
		part, partMeaningful, err := prepareCanonicalWorkContentPart(prepared[index])
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", index, err)
		}
		prepared[index] = part
		meaningful = meaningful || partMeaningful
	}
	if !meaningful {
		return nil, errors.New("content must contain at least one non-empty part")
	}
	return prepared, nil
}

func prepareCanonicalWorkContentPart(
	part ContentPart,
) (ContentPart, bool, error) {
	part.Type = part.Type.Normalized()
	switch part.Type {
	case ContentPartTypeText:
		return part, strings.TrimSpace(part.Text) != "", nil
	case ContentPartTypeImage, ContentPartTypeAudio, ContentPartTypeBinary:
		normalized, err := normalizeFileBackedContentPart(part)
		if err != nil {
			return ContentPart{}, false, err
		}
		if err := contenturl.Validate(normalized.URL); err != nil {
			return ContentPart{}, false, err
		}
		if err := validateCanonicalContentMediaType(normalized); err != nil {
			return ContentPart{}, false, err
		}
		return normalized, true, nil
	case ContentPartTypeJSON:
		if len(part.JSON) == 0 || !json.Valid(part.JSON) {
			return ContentPart{}, false, errors.New("json must contain valid JSON")
		}
		return part, true, nil
	default:
		return ContentPart{}, false, fmt.Errorf(
			"type must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY",
		)
	}
}

func validateCanonicalContentMediaType(part ContentPart) error {
	mediaType := strings.TrimSpace(part.ContentType)
	if mediaType == "" {
		return nil
	}
	switch part.Type {
	case ContentPartTypeImage:
		if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			return errors.New("contentType must start with image/ for image content")
		}
	case ContentPartTypeAudio:
		if !strings.HasPrefix(strings.ToLower(mediaType), "audio/") {
			return errors.New("contentType must start with audio/ for audio content")
		}
	}
	return nil
}

func applyDefaultBatchWorkTypes(request *Request, defaultWorkTypeID string) {
	if request == nil || strings.TrimSpace(defaultWorkTypeID) == "" {
		return
	}
	for index := range request.Works {
		if strings.TrimSpace(request.Works[index].WorkTypeID) != "" {
			continue
		}
		request.Works[index].WorkTypeID = defaultWorkTypeID
	}
}

func applyStableWorkRequestLineage(request *Request) {
	if request == nil || len(request.Works) == 0 {
		return
	}
	traceID := request.CurrentChainingTraceID
	if traceID == "" {
		for _, item := range request.Works {
			traceID = ResolveWorkRequestCurrentChainingTraceID(
				item.CurrentChainingTraceID,
				item.TraceID,
			)
			if traceID != "" {
				break
			}
		}
	}
	if traceID == "" && request.RequestID != "" {
		traceID = "trace-" + request.RequestID
	}
	if traceID == "" {
		return
	}
	if request.CurrentChainingTraceID == "" {
		request.CurrentChainingTraceID = traceID
	}
	for index := range request.Works {
		item := &request.Works[index]
		if item.CurrentChainingTraceID == "" {
			item.CurrentChainingTraceID = ResolveWorkRequestCurrentChainingTraceID(
				item.TraceID,
				traceID,
			)
		}
		if item.TraceID == "" {
			item.TraceID = item.CurrentChainingTraceID
		}
	}
}

func requestPreparationError(err error) error {
	if err == nil {
		return nil
	}
	return &RequestPreparationError{Message: err.Error(), Cause: err}
}
