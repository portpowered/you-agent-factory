package work

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

// Admission typed failures peers can distinguish on the root Service admission
// slice (SubmitWorkRequestForSession). Implementations may wrap these sentinels
// with additional context; peers should branch with errors.Is.
var (
	// ErrInvalidWorkRequest reports that admission rejected a Work Request
	// because required identity or payload fields failed Work-owned validation.
	ErrInvalidWorkRequest = errors.New("invalid Work Request")

	// ErrWorkRequestConflict reports that admission conflicted with an already
	// applied request identity or incompatible admission state.
	ErrWorkRequestConflict = errors.New("Work Request admission conflict")

	// ErrWorkRequestRejected reports that admission policy rejected a Work
	// Request without accepting it into a Factory Session.
	ErrWorkRequestRejected = errors.New("Work Request rejected")
)

// ErrConflictingWorkRequestTraceFields reports conflicting current and legacy
// trace identities on one Work request.
var ErrConflictingWorkRequestTraceFields = requestadmission.ErrConflictingWorkRequestTraceFields

// PreparedFactoryRequestBatch is a detached, validated canonical batch and
// its original representation bytes.
type PreparedFactoryRequestBatch struct {
	Request       WorkRequest
	CanonicalJSON []byte
}

// FactoryRequestBatchPreparation is the exact Work-owned decode and admission
// role consumed by batch transports.
type FactoryRequestBatchPreparation interface {
	PrepareFactoryRequestBatch(context.Context, []byte) (PreparedFactoryRequestBatch, error)
}

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
// available, its original public JSON.
type WorkRequestPreparation struct {
	Request           WorkRequest
	CanonicalJSON     []byte
	DefaultWorkTypeID string
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

// SingleWorkTarget is the stable identity of the sole Work item in a Work
// Request.
type SingleWorkTarget struct {
	WorkID     string
	WorkTypeID string
}

// SingleWorkTargetPreparation is the Work-owned operation for validating a
// request and selecting its sole Work target.
type SingleWorkTargetPreparation func(WorkRequest) (SingleWorkTarget, error)

// FileSource reads request or payload files for transport edges.
type FileSource interface {
	ReadFile(string) ([]byte, error)
}

// RequestFileLoader loads and parses a canonical Work Request from a file path.
type RequestFileLoader func(string) (WorkRequest, error)

// PayloadFileReader reads raw payload bytes from a file path.
type PayloadFileReader func(string) ([]byte, error)

// NewFactoryRequestBatchPreparation constructs the pure canonical batch
// preparation role.
func NewFactoryRequestBatchPreparation() FactoryRequestBatchPreparation {
	return factoryRequestBatchPreparationAdapter{}
}

type factoryRequestBatchPreparationAdapter struct{}

func (factoryRequestBatchPreparationAdapter) PrepareFactoryRequestBatch(
	ctx context.Context,
	data []byte,
) (PreparedFactoryRequestBatch, error) {
	prepared, err := requestadmission.NewFactoryRequestBatchPreparation().PrepareFactoryRequestBatch(ctx, data)
	if err != nil {
		return PreparedFactoryRequestBatch{}, err
	}
	return PreparedFactoryRequestBatch{
		Request:       workRequestFromAdmission(prepared.Request),
		CanonicalJSON: prepared.CanonicalJSON,
	}, nil
}

// NewContentPreparation constructs canonical Work-content admission.
func NewContentPreparation() ContentPreparation {
	return contentPreparationAdapter{}
}

type contentPreparationAdapter struct{}

func (contentPreparationAdapter) PrepareWorkContent(
	ctx context.Context,
	content []WorkContentPart,
) ([]WorkContentPart, error) {
	prepared, err := requestadmission.NewContentPreparation().PrepareWorkContent(
		ctx,
		workContentPartsToAdmission(content),
	)
	if err != nil {
		return nil, mapRequestPreparationError(err)
	}
	return workContentPartsFromAdmission(prepared), nil
}

// NewRequestPreparationService constructs the pure Work Request admission
// service.
func NewRequestPreparationService(content ContentPreparation) (RequestPreparationService, error) {
	innerContent, err := requestPreparationContentAdapter(content)
	if err != nil {
		return nil, err
	}
	inner, err := requestadmission.NewRequestPreparationService(innerContent)
	if err != nil {
		return nil, err
	}
	return requestPreparationServiceAdapter{inner: inner}, nil
}

type requestPreparationServiceAdapter struct {
	inner requestadmission.RequestPreparationService
}

func (a requestPreparationServiceAdapter) PrepareWorkRequest(
	ctx context.Context,
	input WorkRequestPreparation,
) (WorkRequest, error) {
	prepared, err := a.inner.PrepareWorkRequest(ctx, requestadmission.WorkRequestPreparation{
		Request:           workRequestToAdmission(input.Request),
		CanonicalJSON:     input.CanonicalJSON,
		DefaultWorkTypeID: input.DefaultWorkTypeID,
	})
	if err != nil {
		return WorkRequest{}, mapRequestPreparationError(err)
	}
	return workRequestFromAdmission(prepared), nil
}

// NewRequestFileLoader constructs a path-backed Work Request file loader.
func NewRequestFileLoader(source FileSource) RequestFileLoader {
	loader := requestadmission.NewRequestFileLoader(fileSourceAdapter{source: source})
	return func(path string) (WorkRequest, error) {
		request, err := loader(path)
		if err != nil {
			return WorkRequest{}, err
		}
		return workRequestFromAdmission(request), nil
	}
}

// NewPayloadFileReader constructs a path-backed payload file reader.
func NewPayloadFileReader(source FileSource) PayloadFileReader {
	reader := requestadmission.NewPayloadFileReader(fileSourceAdapter{source: source})
	return func(path string) ([]byte, error) {
		return reader(path)
	}
}

// NewSingleWorkTargetPreparation constructs strict target preparation.
func NewSingleWorkTargetPreparation() SingleWorkTargetPreparation {
	prepare := requestadmission.NewSingleWorkTargetPreparation()
	return func(request WorkRequest) (SingleWorkTarget, error) {
		target, err := prepare(workRequestToAdmission(request))
		if err != nil {
			return SingleWorkTarget{}, err
		}
		return SingleWorkTarget{WorkID: target.WorkID, WorkTypeID: target.WorkTypeID}, nil
	}
}

// ParseCanonicalWorkRequestJSON parses the public Work request contract.
func ParseCanonicalWorkRequestJSON(data []byte) (WorkRequest, error) {
	request, err := requestadmission.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return WorkRequest{}, err
	}
	return workRequestFromAdmission(request), nil
}

// ValidateCanonicalWorkRequestJSON validates public Work request field names
// and trace aliases without constructing runtime state.
func ValidateCanonicalWorkRequestJSON(data []byte) error {
	return requestadmission.ValidateCanonicalWorkRequestJSON(data)
}

// ResolveWorkRequestCurrentChainingTraceID returns the effective chaining
// trace, preferring the current field while preserving traceId fallback.
func ResolveWorkRequestCurrentChainingTraceID(current, legacy string) string {
	return requestadmission.ResolveWorkRequestCurrentChainingTraceID(current, legacy)
}

// ValidateWorkRequestTraceFields rejects conflicting current and legacy trace
// values when both are populated.
func ValidateWorkRequestTraceFields(current, legacy string) error {
	return requestadmission.ValidateWorkRequestTraceFields(current, legacy)
}

// ValidateWorkRequestTraceFieldAliases validates the public and retired JSON
// spellings before the request is decoded.
func ValidateWorkRequestTraceFieldAliases(currentRaw, legacyCurrentRaw, traceRaw, legacyTraceRaw json.RawMessage) error {
	return requestadmission.ValidateWorkRequestTraceFieldAliases(currentRaw, legacyCurrentRaw, traceRaw, legacyTraceRaw)
}

// WorkRequestFromSubmitRequests projects normalized submissions into the
// canonical FACTORY_REQUEST_BATCH contract.
func WorkRequestFromSubmitRequests(requests []SubmitRequest) WorkRequest {
	return workRequestFromAdmission(requestadmission.WorkRequestFromSubmitRequests(submitRequestsToAdmission(requests)))
}

// WorkRequestSubmitResultFromNormalized builds accepted-request metadata from
// normalized submit requests.
func WorkRequestSubmitResultFromNormalized(requestID string, work []SubmitRequest, accepted bool) WorkRequestSubmitResult {
	return submitResultFromAdmission(requestadmission.WorkRequestSubmitResultFromNormalized(requestID, submitRequestsToAdmission(work), accepted))
}

// NormalizeWorkRequest validates a FACTORY_REQUEST_BATCH and converts it into runtime submit requests.
func NormalizeWorkRequest(req WorkRequest, opts WorkRequestNormalizeOptions) ([]SubmitRequest, error) {
	normalized, err := requestadmission.NormalizeWorkRequest(workRequestToAdmission(req), normalizeOptionsToAdmission(opts))
	if err != nil {
		return nil, err
	}
	return submitRequestsFromAdmission(normalized), nil
}

// SubmitResultFromNormalized builds accepted batch metadata from normalized submit requests.
func SubmitResultFromNormalized(requestID string, normalized []SubmitRequest) WorkRequestSubmitResult {
	return WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
}

// NormalizeGeneratedSubmissionBatch validates the canonical generated request
// and merges optional runtime submission fields onto the matching work items.
func NormalizeGeneratedSubmissionBatch(batch GeneratedSubmissionBatch, opts WorkRequestNormalizeOptions) ([]SubmitRequest, error) {
	normalized, err := requestadmission.NormalizeGeneratedSubmissionBatch(
		generatedSubmissionBatchToAdmission(batch),
		normalizeOptionsToAdmission(opts),
	)
	if err != nil {
		return nil, err
	}
	return submitRequestsFromAdmission(normalized), nil
}

// WorkRequestRecordFromSubmitRequests builds the canonical request-history
// record for a normalized batch submission.
func WorkRequestRecordFromSubmitRequests(requestID string, source string, requests []SubmitRequest) WorkRequestRecord {
	record := requestadmission.WorkRequestRecordFromSubmitRequests(requestID, source, submitRequestsToAdmission(requests))
	return workRequestRecordFromAdmission(record)
}

// SubmitWorkName returns the canonical display name for a submit request.
func SubmitWorkName(req SubmitRequest) string {
	return requestadmission.SubmitWorkName(submitRequestToAdmission(req))
}

type fileSourceAdapter struct {
	source FileSource
}

func (a fileSourceAdapter) ReadFile(path string) ([]byte, error) {
	if a.source == nil {
		return nil, nil
	}
	return a.source.ReadFile(path)
}

func requestPreparationContentAdapter(content ContentPreparation) (requestadmission.ContentPreparation, error) {
	if content == nil {
		return nil, errors.New("Work content preparation is required")
	}
	return requestPreparationContentBridge{content: content}, nil
}

type requestPreparationContentBridge struct {
	content ContentPreparation
}

func (b requestPreparationContentBridge) PrepareWorkContent(
	ctx context.Context,
	content []requestadmission.ContentPart,
) ([]requestadmission.ContentPart, error) {
	prepared, err := b.content.PrepareWorkContent(ctx, workContentPartsFromAdmission(content))
	if err != nil {
		return nil, err
	}
	return workContentPartsToAdmission(prepared), nil
}

func mapRequestPreparationError(err error) error {
	if err == nil {
		return nil
	}
	var inner *requestadmission.RequestPreparationError
	if errors.As(err, &inner) {
		return &RequestPreparationError{Message: inner.Message, Cause: inner.Cause}
	}
	return err
}

func workRequestToAdmission(req WorkRequest) requestadmission.Request {
	inner := requestadmission.Request{
		RequestID:              req.RequestID,
		CurrentChainingTraceID: req.CurrentChainingTraceID,
		Type:                   requestadmission.RequestType(req.Type),
	}
	for _, work := range req.Works {
		inner.Works = append(inner.Works, workToAdmission(work))
	}
	for _, rel := range req.Relations {
		inner.Relations = append(inner.Relations, workRelationToAdmission(rel))
	}
	return inner
}

func workRequestFromAdmission(req requestadmission.Request) WorkRequest {
	outer := WorkRequest{
		RequestID:              req.RequestID,
		CurrentChainingTraceID: req.CurrentChainingTraceID,
		Type:                   WorkRequestType(req.Type),
	}
	for _, work := range req.Works {
		outer.Works = append(outer.Works, workFromAdmission(work))
	}
	for _, rel := range req.Relations {
		outer.Relations = append(outer.Relations, workRelationFromAdmission(rel))
	}
	return outer
}

func workToAdmission(work Work) requestadmission.Work {
	return requestadmission.Work{
		Name:                     work.Name,
		WorkID:                   work.WorkID,
		RequestID:                work.RequestID,
		WorkTypeID:               work.WorkTypeID,
		State:                    work.State,
		ChainingTraceDepth:       work.ChainingTraceDepth,
		CurrentChainingTraceID:   work.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(work.PreviousChainingTraceIDs),
		TraceID:                  work.TraceID,
		Content:                  workContentPartsToAdmission(work.Content),
		Payload:                  work.Payload,
		Tags:                     cloneStringMap(work.Tags),
		ExecutionID:              work.ExecutionID,
		RuntimeRelations:         relationsToAdmission(work.RuntimeRelations),
		InvocationArguments:      invocationArgumentsToAdmission(work.InvocationArguments),
	}
}

func workFromAdmission(work requestadmission.Work) Work {
	return Work{
		Name:                     work.Name,
		WorkID:                   work.WorkID,
		RequestID:                work.RequestID,
		WorkTypeID:               work.WorkTypeID,
		State:                    work.State,
		ChainingTraceDepth:       work.ChainingTraceDepth,
		CurrentChainingTraceID:   work.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(work.PreviousChainingTraceIDs),
		TraceID:                  work.TraceID,
		Content:                  workContentPartsFromAdmission(work.Content),
		Payload:                  work.Payload,
		Tags:                     cloneStringMap(work.Tags),
		ExecutionID:              work.ExecutionID,
		RuntimeRelations:         relationsFromAdmission(work.RuntimeRelations),
		InvocationArguments:      invocationArgumentsFromAdmission(work.InvocationArguments),
	}
}

func workRelationToAdmission(rel WorkRelation) requestadmission.WorkRelation {
	return requestadmission.WorkRelation{
		Type:           requestadmission.WorkRelationType(rel.Type),
		SourceWorkName: rel.SourceWorkName,
		TargetWorkName: rel.TargetWorkName,
		RequiredState:  rel.RequiredState,
	}
}

func workRelationFromAdmission(rel requestadmission.WorkRelation) WorkRelation {
	return WorkRelation{
		Type:           WorkRelationType(rel.Type),
		SourceWorkName: rel.SourceWorkName,
		TargetWorkName: rel.TargetWorkName,
		RequiredState:  rel.RequiredState,
	}
}

func submitRequestToAdmission(req SubmitRequest) requestadmission.SubmitRequest {
	return requestadmission.SubmitRequest{
		RequestID:                req.RequestID,
		WorkID:                   req.WorkID,
		Name:                     req.Name,
		WorkTypeID:               req.WorkTypeID,
		TargetState:              req.TargetState,
		ChainingTraceDepth:       req.ChainingTraceDepth,
		CurrentChainingTraceID:   req.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(req.PreviousChainingTraceIDs),
		TraceID:                  req.TraceID,
		Content:                  workContentPartsToAdmission(req.Content),
		Payload:                  ClonePayload(req.Payload),
		Tags:                     cloneStringMap(req.Tags),
		Relations:                relationsToAdmission(req.Relations),
		ExecutionID:              req.ExecutionID,
		InvocationArguments:      invocationArgumentsToAdmission(req.InvocationArguments),
	}
}

func submitRequestsToAdmission(requests []SubmitRequest) []requestadmission.SubmitRequest {
	if len(requests) == 0 {
		return nil
	}
	converted := make([]requestadmission.SubmitRequest, len(requests))
	for i, req := range requests {
		converted[i] = submitRequestToAdmission(req)
	}
	return converted
}

func submitRequestsFromAdmission(requests []requestadmission.SubmitRequest) []SubmitRequest {
	if len(requests) == 0 {
		return nil
	}
	converted := make([]SubmitRequest, len(requests))
	for i, req := range requests {
		converted[i] = submitRequestFromAdmission(req)
	}
	return converted
}

func submitRequestFromAdmission(req requestadmission.SubmitRequest) SubmitRequest {
	return SubmitRequest{
		RequestID:                req.RequestID,
		WorkID:                   req.WorkID,
		Name:                     req.Name,
		WorkTypeID:               req.WorkTypeID,
		TargetState:              req.TargetState,
		ChainingTraceDepth:       req.ChainingTraceDepth,
		CurrentChainingTraceID:   req.CurrentChainingTraceID,
		PreviousChainingTraceIDs: cloneStringSlice(req.PreviousChainingTraceIDs),
		TraceID:                  req.TraceID,
		Content:                  workContentPartsFromAdmission(req.Content),
		Payload:                  ClonePayload(req.Payload),
		Tags:                     cloneStringMap(req.Tags),
		Relations:                relationsFromAdmission(req.Relations),
		ExecutionID:              req.ExecutionID,
		InvocationArguments:      invocationArgumentsFromAdmission(req.InvocationArguments),
	}
}

func normalizeOptionsToAdmission(opts WorkRequestNormalizeOptions) requestadmission.NormalizeOptions {
	return requestadmission.NormalizeOptions{
		DefaultWorkTypeID: opts.DefaultWorkTypeID,
		ValidWorkTypes:    opts.ValidWorkTypes,
		ValidStatesByType: opts.ValidStatesByType,
		IDGenerator:       requestadmission.IDGenerator(opts.IDGenerator),
	}
}

func generatedSubmissionBatchToAdmission(batch GeneratedSubmissionBatch) requestadmission.GeneratedSubmissionBatch {
	inner := requestadmission.GeneratedSubmissionBatch{
		Request: workRequestToAdmission(batch.Request),
		Metadata: requestadmission.GeneratedSubmissionBatchMetadata{
			Source:        batch.Metadata.Source,
			ParentLineage: cloneStringSlice(batch.Metadata.ParentLineage),
		},
	}
	inner.Submissions = submitRequestsToAdmission(batch.Submissions)
	return inner
}

func submitResultFromAdmission(result requestadmission.SubmitResult) WorkRequestSubmitResult {
	outer := WorkRequestSubmitResult{
		RequestID:    result.RequestID,
		TraceID:      result.TraceID,
		WorkID:       result.WorkID,
		Name:         result.Name,
		WorkTypeName: result.WorkTypeName,
		Accepted:     result.Accepted,
	}
	for _, work := range result.Works {
		outer.Works = append(outer.Works, WorkRequestSubmittedWork{
			Name:         work.Name,
			WorkTypeName: work.WorkTypeName,
			WorkID:       work.WorkID,
		})
	}
	return outer
}

func workRequestRecordFromAdmission(record requestadmission.RequestRecord) WorkRequestRecord {
	outer := WorkRequestRecord{
		RequestID: record.RequestID,
		Type:      WorkRequestType(record.Type),
		TraceID:   record.TraceID,
		Source:    record.Source,
	}
	for _, item := range record.WorkItems {
		outer.WorkItems = append(outer.WorkItems, FactoryWorkItem{
			ID:                       item.ID,
			WorkTypeID:               item.WorkTypeID,
			State:                    item.State,
			DisplayName:              item.DisplayName,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   item.CurrentChainingTraceID,
			PreviousChainingTraceIDs: cloneStringSlice(item.PreviousChainingTraceIDs),
			TraceID:                  item.TraceID,
			Content:                  workContentPartsFromAdmission(item.Content),
			Tags:                     cloneStringMap(item.Tags),
		})
	}
	for _, rel := range record.Relations {
		outer.Relations = append(outer.Relations, FactoryRelation{
			Type:           rel.Type,
			SourceWorkID:   rel.SourceWorkID,
			SourceWorkName: rel.SourceWorkName,
			TargetWorkID:   rel.TargetWorkID,
			TargetWorkName: rel.TargetWorkName,
			RequiredState:  rel.RequiredState,
			RequestID:      rel.RequestID,
			TraceID:        rel.TraceID,
		})
	}
	return outer
}

func workContentPartsToAdmission(parts []WorkContentPart) []requestadmission.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]requestadmission.ContentPart, len(parts))
	for i, part := range parts {
		converted[i] = requestadmission.ContentPart{
			Type:        requestadmission.ContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func workContentPartsFromAdmission(parts []requestadmission.ContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	converted := make([]WorkContentPart, len(parts))
	for i, part := range parts {
		converted[i] = WorkContentPart{
			Type:        WorkContentPartType(part.Type),
			Text:        part.Text,
			URL:         part.URL,
			File:        part.File,
			JSON:        append(json.RawMessage(nil), part.JSON...),
			Slot:        part.Slot,
			Label:       part.Label,
			Role:        part.Role,
			ContentType: part.ContentType,
			ArtifactID:  part.ArtifactID,
			Metadata:    cloneAnyMap(part.Metadata),
		}
	}
	return converted
}

func relationsToAdmission(relations []Relation) []requestadmission.Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]requestadmission.Relation, len(relations))
	for i, rel := range relations {
		converted[i] = requestadmission.Relation{
			Type:          requestadmission.RelationType(rel.Type),
			TargetWorkID:  rel.TargetWorkID,
			RequiredState: rel.RequiredState,
		}
	}
	return converted
}

func relationsFromAdmission(relations []requestadmission.Relation) []Relation {
	if len(relations) == 0 {
		return nil
	}
	converted := make([]Relation, len(relations))
	for i, rel := range relations {
		converted[i] = Relation{
			Type:          RelationType(rel.Type),
			TargetWorkID:  rel.TargetWorkID,
			RequiredState: rel.RequiredState,
		}
	}
	return converted
}

func invocationArgumentsToAdmission(args *InvocationArguments) *requestadmission.InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &requestadmission.InvocationArguments{
		Arguments: make(map[string]requestadmission.InvocationArgument, len(args.Arguments)),
	}
	for name, argument := range args.Arguments {
		next := requestadmission.InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]requestadmission.InvocationArgumentSource(nil), invocationArgumentSourcesToAdmission(argument.Sources)...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func invocationArgumentsFromAdmission(args *requestadmission.InvocationArguments) *InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &InvocationArguments{
		Arguments: make(map[string]InvocationArgument, len(args.Arguments)),
	}
	for name, argument := range args.Arguments {
		next := InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]InvocationArgumentSource(nil), invocationArgumentSourcesFromAdmission(argument.Sources)...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func invocationArgumentSourcesToAdmission(sources []InvocationArgumentSource) []requestadmission.InvocationArgumentSource {
	converted := make([]requestadmission.InvocationArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = requestadmission.InvocationArgumentSource{
			Kind:   source.Kind,
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func invocationArgumentSourcesFromAdmission(sources []requestadmission.InvocationArgumentSource) []InvocationArgumentSource {
	converted := make([]InvocationArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = InvocationArgumentSource{
			Kind:   source.Kind,
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}
