package http

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
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/http/moveprojection"
	"github.com/portpowered/infinite-you/pkg/transports/http/workstationprojection"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workcontent "github.com/portpowered/infinite-you/pkg/work/content"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	workquery "github.com/portpowered/infinite-you/pkg/work/query"
	"go.uber.org/zap"
)

const (
	defaultMaxResults                = 50
	submitWorkStagedFileRefPrefix    = "submit-work-stage:v1:"
	submitWorkStagedFileTokenDivider = "."
	submitWorkStageDirPrefix         = "submit-work-stage-"
)

var submitWorkStagedFileRefSecret = mustReadSubmitWorkStagedFileRefSecret()

// BuildFactoryWorldWorkstationRequestProjectionSlice delegates to the focused
// workstation projection package while preserving the HTTP transport entrypoint.
func BuildFactoryWorldWorkstationRequestProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	return workstationprojection.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

// BuildFactoryWorldWorkMoveOperationProjectionSlice delegates to the focused
// move projection package while preserving the HTTP transport entrypoint.
func BuildFactoryWorldWorkMoveOperationProjectionSlice(
	state interfaces.FactoryWorldState,
) factoryapi.FactoryWorldWorkMoveOperationProjectionSlice {
	return moveprojection.BuildFactoryWorldWorkMoveOperationProjectionSlice(state)
}

func (s *Server) ListWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, params factoryapi.ListWorkBySessionIdParams) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.listWork(w, r, params, func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
		return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
	})
}

func (s *Server) listWork(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.ListWorkBySessionIdParams,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
) {
	selectionOptions := listWorkSelectionOptions(params)
	selection, err := workquery.NewSelection(selectionOptions)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
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
	materialized := materialize.CollectPublicWorkTokens(&snapshot.Marking, snapshot.Dispatches)
	workNamesByID := publicWorkNamesByID(materialized.Tokens)
	itemsByID := make(map[string]listWorkItem, len(materialized.Tokens))
	queryItems := make([]workquery.Item, 0, len(materialized.Tokens))
	for _, t := range materialized.Tokens {
		_, inFlightOnly := materialized.InFlightOnlyByID[t.ID]
		work := tokenToWork(t, snapshot.Topology, inFlightOnly)
		work.Relations = generatedWorkRelations(t, work.Name, workNamesByID)
		item := listWorkItem{cursorID: t.ID, work: work}
		itemsByID[t.ID] = item
		queryItems = append(queryItems, listWorkQueryItem(item))
	}
	selected := selection.Apply(queryItems)
	items := make([]listWorkItem, 0, len(selected))
	for _, item := range selected {
		items = append(items, itemsByID[item.ID])
	}

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

type listWorkItem struct {
	cursorID string
	work     factoryapi.Work
}

func listWorkSelectionOptions(params factoryapi.ListWorkBySessionIdParams) workquery.SelectionOptions {
	return workquery.SelectionOptions{
		StateName:    listWorkOptionalString(params.StateName),
		StateType:    listWorkOptionalString(params.StateType),
		Name:         listWorkOptionalString(params.Name),
		WorkTypeName: listWorkOptionalString(params.WorkTypeName),
		TraceID:      listWorkOptionalString(params.TraceId),
		SortBy:       listWorkString(params.SortBy),
	}
}

func listWorkOptionalString[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

func listWorkString[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func listWorkQueryItem(item listWorkItem) workquery.Item {
	work := item.work
	queryItem := workquery.Item{
		ID:                     item.cursorID,
		Name:                   work.Name,
		WorkTypeName:           stringValue(work.WorkTypeName),
		TraceID:                stringValue(work.TraceId),
		CurrentChainingTraceID: stringValue(work.CurrentChainingTraceId),
	}
	if work.State != nil {
		queryItem.State = &workquery.State{Name: work.State.Name, Type: string(work.State.Type)}
	}
	return queryItem
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

func (s *Server) GetWorkBySessionId(w http.ResponseWriter, r *http.Request, sessionID factoryapi.SessionID, id factoryapi.WorkOrTokenID) {
	sessionRuntime, ok := s.requireSessionRuntime(w)
	if !ok {
		return
	}
	s.getWork(
		w,
		r,
		string(sessionID),
		id,
		func(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
			return sessionRuntime.GetEngineStateSnapshotForSession(ctx, string(sessionID))
		},
		func(ctx context.Context) (factoryapi.FactorySession, error) {
			return sessionRuntime.GetFactorySession(ctx, string(sessionID))
		},
	)
}

func (s *Server) getWork(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	id factoryapi.WorkOrTokenID,
	loadSnapshot func(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error),
	loadSession func(context.Context) (factoryapi.FactorySession, error),
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

	materialized := materialize.CollectPublicWorkTokens(&snapshot.Marking, snapshot.Dispatches)
	token, inFlightOnly, ok := findPublicWorkToken(materialized, string(id))
	if !ok {
		s.writeError(w, http.StatusNotFound, "work not found", "NOT_FOUND")
		return
	}

	workNamesByID := publicWorkNamesByID(materialized.Tokens)
	work := tokenToWork(token, snapshot.Topology, inFlightOnly)
	work.Relations = generatedWorkRelations(token, work.Name, workNamesByID)
	work.StopSummary = apisurface.BuildWorkStopSummary(sessionID, snapshot, token, loadSessionStopSummary(r.Context(), loadSession))
	s.writeJSON(w, http.StatusOK, work)
}

func loadSessionStopSummary(
	ctx context.Context,
	loadSession func(context.Context) (factoryapi.FactorySession, error),
) *factoryapi.FactoryStopSummary {
	if loadSession == nil {
		return nil
	}
	session, err := loadSession(ctx)
	if err != nil {
		return nil
	}
	if session.Runtime.StopSummary == nil {
		return nil
	}
	summary := *session.Runtime.StopSummary
	return &summary
}

func findPublicWorkToken(materialized materialize.PublicWorkTokens, id string) (*factorytoken.Token, bool, bool) {
	for _, token := range materialized.Tokens {
		if token.ID == id && publicWorkToken(token) {
			_, inFlightOnly := materialized.InFlightOnlyByID[token.ID]
			return token, inFlightOnly, true
		}
	}
	for _, token := range materialized.Tokens {
		if !publicWorkToken(token) {
			continue
		}
		if token.Color.WorkID == id {
			_, inFlightOnly := materialized.InFlightOnlyByID[token.ID]
			return token, inFlightOnly, true
		}
	}
	return nil, false, false
}
func tokenToWork(t *factorytoken.Token, net *state.Net, inFlightOnly bool) factoryapi.Work {
	name := firstNonEmptyString(t.Color.Name, t.Color.WorkID, t.ID)
	return factoryapi.Work{
		Name:                     name,
		WorkId:                   stringPtrIfNotEmpty(t.Color.WorkID),
		WorkTypeName:             stringPtrIfNotEmpty(t.Color.WorkTypeID),
		State:                    workStateForMaterializedToken(t, net, inFlightOnly),
		ChainingTraceDepth:       intPtrIfPositive(t.Color.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(firstNonEmptyString(t.Color.CurrentChainingTraceID, t.Color.TraceID)),
		PreviousChainingTraceIds: stringSlicePtrCopy(t.Color.PreviousChainingTraceIDs),
		TraceId:                  stringPtrIfNotEmpty(t.Color.TraceID),
		Content:                  domainWorkContentToGeneratedPtr(t.Color.Content),
		Tags:                     stringMapPtr(t.Color.Tags),
	}
}

func publicWorkNamesByID(tokens []*factorytoken.Token) map[string]string {
	names := make(map[string]string, len(tokens))
	for _, token := range tokens {
		if !materialize.IsPublicWorkToken(token) || token.Color.WorkID == "" {
			continue
		}
		names[token.Color.WorkID] = firstNonEmptyString(token.Color.Name, token.Color.WorkID, token.ID)
	}
	return names
}

func generatedWorkRelations(token *factorytoken.Token, sourceWorkName string, workNamesByID map[string]string) *[]factoryapi.Relation {
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

func workStateForMaterializedToken(t *factorytoken.Token, net *state.Net, inFlightOnly bool) *factoryapi.WorkState {
	if inFlightOnly {
		return workStateForInFlightToken(t, net)
	}
	return workStateForToken(t, net)
}

func workStateForInFlightToken(t *factorytoken.Token, net *state.Net) *factoryapi.WorkState {
	if t == nil {
		return nil
	}
	_, stateName := state.SplitPlaceID(t.PlaceID)
	if net != nil {
		if place, ok := net.Places[t.PlaceID]; ok && place.State != "" {
			stateName = place.State
		}
	}
	if stateName == "" {
		return nil
	}
	return &factoryapi.WorkState{
		Name: stateName,
		Type: factoryapi.WorkStateTypePROCESSING,
	}
}

func workStateForToken(t *factorytoken.Token, net *state.Net) *factoryapi.WorkState {
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

func publicWorkToken(token *factorytoken.Token) bool {
	return materialize.IsPublicWorkToken(token)
}
func domainWorkContentToGeneratedPtr(parts []work.WorkContentPart) *factoryapi.WorkContent {
	return contentcontract.GeneratedPtrFromParts(parts)
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

	stagedFileRef, stagedPath, err := writeStagedSubmitWorkFile(content, req.FileName)
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, fmt.Errorf("write staged submit-work file: %w", err)
	}
	contentURL, err := workcontent.FilesystemPathToContentURL(stagedPath)
	if err != nil {
		return factoryapi.StageSubmitWorkFileResponse{}, fmt.Errorf("stage submit-work content url: %w", err)
	}

	return factoryapi.StageSubmitWorkFileResponse{
		FileName:      req.FileName,
		MediaType:     req.MediaType,
		StagedFileRef: stagedFileRef,
		Url:           factoryapi.SubmitWorkContentURLProperty(contentURL),
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

func writeStagedSubmitWorkFile(content []byte, fileName string) (stagedFileRef string, stagedPath string, err error) {
	stageDir, err := os.MkdirTemp("", submitWorkStageDirPrefix+"*")
	if err != nil {
		return "", "", err
	}

	targetPath := filepath.Join(stageDir, safeSubmitWorkFileName(fileName))
	if err := os.WriteFile(targetPath, content, 0o600); err != nil {
		return "", "", err
	}
	return encodeSubmitWorkStagedFileRef(targetPath), targetPath, nil
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
