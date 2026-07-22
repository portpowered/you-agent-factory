package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// RequestPreparationService owns transport-independent admission policy for a
// canonical Work Request before it is submitted to a Factory Session.
type RequestPreparationService interface {
	PrepareWorkRequest(context.Context, WorkRequestPreparation) (WorkRequest, error)
}

// ContentPreparation is the exact Work-owned admission role for canonical
// content parts mapped by an application edge.
type ContentPreparation interface {
	PrepareWorkContent(context.Context, []WorkContentPart) ([]WorkContentPart, error)
}

// WorkRequestPreparation carries the mapped canonical request and, when
// available, its original public JSON so Work can enforce canonical aliases
// and mutually exclusive submission fields without depending on a transport.
type WorkRequestPreparation struct {
	Request       WorkRequest
	CanonicalJSON []byte
}

// RequestPreparationError is a customer-safe Work Request admission failure.
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

// NewContentPreparation constructs canonical Work-content admission. Wire is
// the production caller.
func NewContentPreparation() ContentPreparation { return contentPreparationService{} }

func (contentPreparationService) PrepareWorkContent(
	ctx context.Context,
	content []WorkContentPart,
) ([]WorkContentPart, error) {
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

// NewRequestPreparationService constructs the pure Work Request admission
// service. Wire is the production caller.
func NewRequestPreparationService(content ContentPreparation) (RequestPreparationService, error) {
	if content == nil {
		return nil, errors.New("Work content preparation is required")
	}
	return requestPreparationService{content: content}, nil
}

func (s requestPreparationService) PrepareWorkRequest(
	ctx context.Context,
	input WorkRequestPreparation,
) (WorkRequest, error) {
	if ctx == nil {
		return WorkRequest{}, errors.New("Work Request preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return WorkRequest{}, err
	}
	if len(input.CanonicalJSON) > 0 {
		if err := validateWorkRequestPublicJSON(input.CanonicalJSON); err != nil {
			return WorkRequest{}, requestPreparationError(err)
		}
	}

	request, err := cloneWorkRequest(input.Request)
	if err != nil {
		return WorkRequest{}, requestPreparationError(fmt.Errorf("payload: %w", err))
	}
	applyStableWorkRequestLineage(&request)
	for index := range request.Works {
		request.Works[index].Name = strings.TrimSpace(request.Works[index].Name)
		if request.Works[index].Name == "" {
			if request.RequestID == "" && len(request.Works) == 1 {
				return WorkRequest{}, requestPreparationError(errors.New("name is required"))
			}
			return WorkRequest{}, requestPreparationError(fmt.Errorf(
				"work_request: works[%d] is missing required name", index,
			))
		}
		if strings.TrimSpace(request.Works[index].WorkTypeID) == "" {
			if request.RequestID == "" && len(request.Works) == 1 {
				return WorkRequest{}, requestPreparationError(errors.New("workTypeName is required"))
			}
			return WorkRequest{}, requestPreparationError(fmt.Errorf(
				"work_request: works[%d] (%q) is missing workTypeName",
				index,
				request.Works[index].Name,
			))
		}
		content, err := s.content.PrepareWorkContent(ctx, request.Works[index].Content)
		if err != nil {
			return WorkRequest{}, requestPreparationError(fmt.Errorf(
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

func prepareCanonicalWorkContent(content []WorkContentPart) ([]WorkContentPart, error) {
	if len(content) == 0 {
		return nil, nil
	}
	prepared := CloneWorkContentParts(content)
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
	part WorkContentPart,
) (WorkContentPart, bool, error) {
	part.Type = part.Type.Normalized()
	switch part.Type {
	case WorkContentPartTypeText:
		return part, strings.TrimSpace(part.Text) != "", nil
	case WorkContentPartTypeImage, WorkContentPartTypeAudio, WorkContentPartTypeBinary:
		normalized, err := NormalizeFileBackedContentPart(part)
		if err != nil {
			return WorkContentPart{}, false, err
		}
		if err := ValidateContentURL(normalized.URL); err != nil {
			return WorkContentPart{}, false, err
		}
		if err := validateCanonicalContentMediaType(normalized); err != nil {
			return WorkContentPart{}, false, err
		}
		return normalized, true, nil
	case WorkContentPartTypeJSON:
		if len(part.JSON) == 0 || !json.Valid(part.JSON) {
			return WorkContentPart{}, false, errors.New("json must contain valid JSON")
		}
		return part, true, nil
	default:
		return WorkContentPart{}, false, fmt.Errorf(
			"type must be one of text, image, TEXT, IMAGE, AUDIO, JSON, or BINARY",
		)
	}
}

func validateCanonicalContentMediaType(part WorkContentPart) error {
	mediaType := strings.TrimSpace(part.ContentType)
	if mediaType == "" {
		return nil
	}
	switch part.Type {
	case WorkContentPartTypeImage:
		if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
			return errors.New("contentType must start with image/ for image content")
		}
	case WorkContentPartTypeAudio:
		if !strings.HasPrefix(strings.ToLower(mediaType), "audio/") {
			return errors.New("contentType must start with audio/ for audio content")
		}
	}
	return nil
}

func applyStableWorkRequestLineage(request *WorkRequest) {
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

func cloneWorkRequest(request WorkRequest) (WorkRequest, error) {
	cloned := request
	cloned.Relations = append([]WorkRelation(nil), request.Relations...)
	cloned.Works = make([]Work, len(request.Works))
	for index, item := range request.Works {
		clonedItem := item
		clonedItem.PreviousChainingTraceIDs = append(
			[]string(nil),
			item.PreviousChainingTraceIDs...,
		)
		clonedItem.Content = CloneWorkContentParts(item.Content)
		payload, err := cloneWorkRequestPayload(item.Payload)
		if err != nil {
			return WorkRequest{}, err
		}
		clonedItem.Payload = payload
		clonedItem.Tags = maps.Clone(item.Tags)
		clonedItem.RuntimeRelations = CloneRelations(item.RuntimeRelations)
		clonedItem.InvocationArguments = CloneInvocationArguments(item.InvocationArguments)
		cloned.Works[index] = clonedItem
	}
	return cloned, nil
}

func cloneWorkRequestPayload(payload any) (any, error) {
	switch value := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), value...), nil
	case json.RawMessage:
		return append(json.RawMessage(nil), value...), nil
	case string, bool, float64:
		return value, nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var cloned any
		if err := json.Unmarshal(encoded, &cloned); err != nil {
			return nil, err
		}
		return cloned, nil
	}
}

func requestPreparationError(err error) error {
	if err == nil {
		return nil
	}
	return &RequestPreparationError{Message: err.Error(), Cause: err}
}
