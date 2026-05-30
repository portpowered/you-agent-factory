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
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
	"go.uber.org/zap"
)

const (
	defaultMaxResults                = 50
	submitWorkStagedFileRefPrefix    = "submit-work-stage:v1:"
	submitWorkStagedFileTokenDivider = "."
	submitWorkStageDirPrefix         = "submit-work-stage-"
)

var submitWorkStagedFileRefSecret = mustReadSubmitWorkStagedFileRefSecret()

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
func domainWorkContentToGeneratedPtr(parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	return workcontent.GeneratedPtrFromParts(parts)
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
