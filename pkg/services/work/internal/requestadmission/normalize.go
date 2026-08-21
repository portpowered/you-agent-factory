package requestadmission

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/lineagegraph"
)

type SingleWorkTargetPreparation func(Request) (SingleWorkTarget, error)

func NewSingleWorkTargetPreparation() SingleWorkTargetPreparation {
	return func(request Request) (SingleWorkTarget, error) {
		if request.Type != RequestTypeFactoryRequestBatch {
			return SingleWorkTarget{}, fmt.Errorf("work_request: unsupported type %q", request.Type)
		}
		if len(request.Works) != 1 {
			return SingleWorkTarget{}, fmt.Errorf("work request requires exactly one work item, got %d", len(request.Works))
		}
		item := request.Works[0]
		if strings.TrimSpace(item.WorkID) == "" {
			return SingleWorkTarget{}, fmt.Errorf("work request target Work ID is required")
		}
		if strings.TrimSpace(item.WorkTypeID) == "" {
			return SingleWorkTarget{}, fmt.Errorf("work request target Work type is required")
		}
		return SingleWorkTarget{
			WorkID:     item.WorkID,
			WorkTypeID: item.WorkTypeID,
		}, nil
	}
}

func NormalizeWorkRequest(req Request, opts NormalizeOptions) ([]SubmitRequest, error) {
	if req.Type != RequestTypeFactoryRequestBatch {
		return nil, fmt.Errorf("work_request: unsupported type %q", req.Type)
	}
	if len(req.Works) == 0 {
		return nil, fmt.Errorf("work_request: works array must contain at least one item")
	}
	if req.RequestID == "" {
		requestID, err := generatedWorkIdentity("request", opts.IDGenerator)
		if err != nil {
			return nil, err
		}
		req.RequestID = requestID
	}
	workIndex, err := validateBatchWork(req, opts)
	if err != nil {
		return nil, err
	}
	relIndex, err := validateAndIndexBatchRelations(req, workIndex, opts)
	if err != nil {
		return nil, err
	}

	traceID := batchTraceID(req)
	return normalizeBatchWorks(req, opts, workIndex, relIndex, traceID)
}

func normalizeBatchWorks(
	req Request,
	opts NormalizeOptions,
	workIndex map[string]normalizedBatchWork,
	relIndex map[string][]Relation,
	traceID string,
) ([]SubmitRequest, error) {
	normalized := make([]SubmitRequest, 0, len(req.Works))
	for i, work := range req.Works {
		normalizedWork, err := normalizeBatchWork(req, opts, workIndex, relIndex, traceID, i, work)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, normalizedWork)

	}
	return normalized, nil
}

func normalizeBatchWork(
	req Request,
	opts NormalizeOptions,
	workIndex map[string]normalizedBatchWork,
	relIndex map[string][]Relation,
	traceID string,
	index int,
	work Work,
) (SubmitRequest, error) {
	workTypeID := work.WorkTypeID
	if workTypeID == "" {
		workTypeID = opts.DefaultWorkTypeID
	}
	content, payload, err := normalizeWorkContent(work.Content, work.Payload)
	if err != nil {
		return SubmitRequest{}, fmt.Errorf("work_request: works[%d] (%q) has invalid content/payload: %w", index, work.Name, err)
	}
	if err := validateWorkPayloadSize(work.Name, workIndex[work.Name].id, payload); err != nil {
		return SubmitRequest{}, err
	}

	itemCurrentChainingTraceID := ResolveWorkRequestCurrentChainingTraceID(work.CurrentChainingTraceID, work.TraceID)
	if itemCurrentChainingTraceID == "" {
		itemCurrentChainingTraceID = ResolveWorkRequestCurrentChainingTraceID(req.CurrentChainingTraceID, traceID)
	}
	itemTraceID := work.TraceID
	if itemTraceID == "" {
		itemTraceID = itemCurrentChainingTraceID
	}
	itemRequestID := work.RequestID
	if itemRequestID == "" {
		itemRequestID = req.RequestID
	}

	tags := make(map[string]string, len(work.Tags)+3)
	maps.Copy(tags, work.Tags)
	tags["_work_name"] = work.Name
	tags["_work_type"] = workTypeID
	if work.ExecutionID != "" {
		tags["_execution_id"] = work.ExecutionID
	}

	return SubmitRequest{
		RequestID:                itemRequestID,
		WorkID:                   workIndex[work.Name].id,
		Name:                     work.Name,
		WorkTypeID:               workTypeID,
		ChainingTraceDepth:       normalizeSubmitChainingTraceDepth(work.ChainingTraceDepth, itemCurrentChainingTraceID, itemTraceID),
		CurrentChainingTraceID:   itemCurrentChainingTraceID,
		PreviousChainingTraceIDs: lineagegraph.CanonicalChainingTraceIDs(work.PreviousChainingTraceIDs),
		TraceID:                  itemTraceID,
		Content:                  content,
		Payload:                  payload,
		Tags:                     tags,
		TargetState:              work.State,
		ExecutionID:              work.ExecutionID,
		Relations: appendUniquePetriRelations(
			cloneRelations(relIndex[work.Name]),
			work.RuntimeRelations,
		),
		InvocationArguments: cloneInvocationArguments(work.InvocationArguments),
	}, nil
}

func SubmitResultFromNormalized(requestID string, normalized []SubmitRequest) SubmitResult {
	return WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
}

func NormalizeGeneratedSubmissionBatch(batch GeneratedSubmissionBatch, opts NormalizeOptions) ([]SubmitRequest, error) {
	normalized, err := NormalizeWorkRequest(batch.Request, opts)
	if err != nil {
		return nil, err
	}
	if len(batch.Submissions) == 0 {
		return normalized, nil
	}

	usedByWorkID := map[string]bool{}
	usedByName := map[string]bool{}
	for _, submitted := range batch.Submissions {
		match := normalizedSubmissionMatch(normalized, submitted, usedByWorkID, usedByName)
		if match < 0 {
			continue
		}
		usedByWorkID[normalized[match].WorkID] = true
		usedByName[normalized[match].Name] = true

		normalized[match] = applyGeneratedSubmissionOverrides(normalized[match], submitted)
	}

	return normalized, nil
}

func WorkRequestRecordFromSubmitRequests(requestID string, source string, requests []SubmitRequest) RequestRecord {
	workItems := make([]FactoryWorkItem, 0, len(requests))
	workNamesByID := make(map[string]string, len(requests))
	traceID := ""
	for _, req := range requests {
		if traceID == "" {
			traceID = req.TraceID
		}
		name := SubmitWorkName(req)
		workNamesByID[req.WorkID] = name
		workItems = append(workItems, FactoryWorkItem{
			ID:                       req.WorkID,
			WorkTypeID:               req.WorkTypeID,
			State:                    req.TargetState,
			DisplayName:              name,
			ChainingTraceDepth:       req.ChainingTraceDepth,
			CurrentChainingTraceID:   ResolveWorkRequestCurrentChainingTraceID(req.CurrentChainingTraceID, req.TraceID),
			PreviousChainingTraceIDs: lineagegraph.CanonicalChainingTraceIDs(req.PreviousChainingTraceIDs),
			TraceID:                  req.TraceID,
			Content:                  append([]ContentPart(nil), req.Content...),
			Tags:                     maps.Clone(req.Tags),
		})
	}

	var relations []FactoryRelation
	for _, req := range requests {
		for _, relation := range req.Relations {
			relations = append(relations, FactoryRelation{
				Type:           string(relation.Type),
				SourceWorkID:   req.WorkID,
				SourceWorkName: SubmitWorkName(req),
				TargetWorkID:   relation.TargetWorkID,
				TargetWorkName: workNamesByID[relation.TargetWorkID],
				RequiredState:  relation.RequiredState,
				RequestID:      requestID,
				TraceID:        req.TraceID,
			})
		}
	}

	return RequestRecord{
		RequestID: requestID,
		Type:      RequestTypeFactoryRequestBatch,
		TraceID:   traceID,
		Source:    source,
		WorkItems: workItems,
		Relations: relations,
	}
}

func SubmitWorkName(req SubmitRequest) string {
	if req.Name != "" {
		return req.Name
	}
	if req.Tags != nil && req.Tags["_work_name"] != "" {
		return req.Tags["_work_name"]
	}
	return req.WorkID
}

type normalizedBatchWork struct {
	name       string
	id         string
	workTypeID string
	batch      bool
}

func normalizedSubmissionMatch(
	normalized []SubmitRequest,
	submitted SubmitRequest,
	usedByWorkID map[string]bool,
	usedByName map[string]bool,
) int {
	if submitted.WorkID != "" {
		if match := matchNormalizedSubmissionByWorkID(normalized, submitted.WorkID, usedByWorkID); match >= 0 {
			return match
		}
	}
	if submitted.Name != "" {
		return matchNormalizedSubmissionByName(normalized, submitted.Name, usedByName)
	}
	return -1
}

func matchNormalizedSubmissionByWorkID(normalized []SubmitRequest, workID string, usedByWorkID map[string]bool) int {
	for i, req := range normalized {
		if usedByWorkID[req.WorkID] {
			continue
		}
		if req.WorkID == workID {
			return i
		}
	}
	return -1
}

func matchNormalizedSubmissionByName(normalized []SubmitRequest, name string, usedByName map[string]bool) int {
	for i, req := range normalized {
		if usedByName[req.Name] {
			continue
		}
		if req.Name == name {
			return i
		}
	}
	return -1
}

func applyGeneratedSubmissionOverrides(next SubmitRequest, submitted SubmitRequest) SubmitRequest {
	if submitted.TargetState != "" {
		next.TargetState = submitted.TargetState
	}
	if submitted.ExecutionID != "" {
		next.ExecutionID = submitted.ExecutionID
	}
	if len(submitted.Tags) > 0 {
		if next.Tags == nil {
			next.Tags = map[string]string{}
		}
		maps.Copy(next.Tags, submitted.Tags)
	}
	if len(submitted.Relations) > 0 {
		next.Relations = appendUniquePetriRelations(cloneRelations(submitted.Relations), next.Relations)
	}
	if len(submitted.PreviousChainingTraceIDs) > 0 {
		next.PreviousChainingTraceIDs = lineagegraph.CanonicalChainingTraceIDs(submitted.PreviousChainingTraceIDs)
	}
	return next
}

func validateBatchWork(req Request, opts NormalizeOptions) (map[string]normalizedBatchWork, error) {
	workNames := make(map[string]int, len(req.Works))
	workIndex := make(map[string]normalizedBatchWork, len(req.Works))
	for i, work := range req.Works {
		if err := validateBatchWorkName(workNames, i, work); err != nil {
			return nil, err
		}

		workTypeID := work.WorkTypeID
		if workTypeID == "" {
			workTypeID = opts.DefaultWorkTypeID
		}
		if workTypeID == "" {
			return nil, fmt.Errorf("work_request: works[%d] (%q) is missing workTypeName", i, work.Name)
		}
		if work.WorkTypeID != "" && opts.DefaultWorkTypeID != "" && work.WorkTypeID != opts.DefaultWorkTypeID {
			return nil, fmt.Errorf("work_request: works[%d] (%q) workTypeName %q conflicts with context work type %q", i, work.Name, work.WorkTypeID, opts.DefaultWorkTypeID)
		}
		if opts.ValidWorkTypes != nil && !opts.ValidWorkTypes[workTypeID] {
			return nil, fmt.Errorf("work_request: works[%d] (%q) references unknown work type %q", i, work.Name, workTypeID)
		}
		if work.State != "" && opts.ValidStatesByType != nil && !opts.ValidStatesByType[workTypeID][work.State] {
			return nil, fmt.Errorf("work_request: works[%d] (%q) references unknown state %q for work type %q", i, work.Name, work.State, workTypeID)
		}

		workID := work.WorkID
		if workID == "" {
			workID = fmt.Sprintf("batch-%s-%s", req.RequestID, work.Name)
		}
		workIndex[work.Name] = normalizedBatchWork{
			name:       work.Name,
			id:         workID,
			workTypeID: workTypeID,
			batch:      true,
		}
	}
	return workIndex, nil
}

func validateBatchWorkName(workNames map[string]int, index int, work Work) error {
	if strings.TrimSpace(work.Name) == "" {
		return fmt.Errorf("work_request: works[%d] is missing required name", index)
	}
	if original, exists := workNames[work.Name]; exists {
		return fmt.Errorf(
			"work_request: duplicate name %q: works[%d].name conflicts with works[%d].name; works[].name must be unique across the entire batch, including across different workTypeName values; rename or remove one entry",
			work.Name,
			index,
			original,
		)
	}
	workNames[work.Name] = index
	return nil
}

func validateAndIndexBatchRelations(req Request, workIndex map[string]normalizedBatchWork, opts NormalizeOptions) (map[string][]Relation, error) {
	relIndex := make(map[string][]Relation)
	seen := map[string]int{}
	parentTargets := make(map[string]normalizedBatchWork)
	dependencyGraph := make(map[string][]string)
	for i, rel := range req.Relations {
		sourceWork, err := validateBatchRelationSource(i, rel, workIndex)
		if err != nil {
			return nil, err
		}
		targetWork, err := resolveBatchRelationTarget(i, rel, workIndex, opts.ExistingWorks)
		if err != nil {
			return nil, err
		}
		if rel.Type == WorkRelationParentChild && !targetWork.batch {
			return nil, fmt.Errorf(
				"work_request: relations[%d] PARENT_CHILD target %s=%q must be declared in this batch's works[]; PARENT_CHILD endpoints cannot reference previously submitted Work",
				i,
				relationTargetField(rel),
				relationTargetValue(rel),
			)
		}
		normalized, key, err := normalizeBatchRelation(i, rel, sourceWork, targetWork, opts)
		if err != nil {
			return nil, err
		}
		if rel.Type == WorkRelationParentChild {
			if existingTarget, ok := parentTargets[sourceWork.id]; ok && existingTarget.id != targetWork.id {
				return nil, fmt.Errorf(
					"work_request: relations[%d] assigns multiple PARENT_CHILD parents to %q (%q and %q)",
					i,
					rel.SourceWorkName,
					existingTarget.name,
					targetWork.name,
				)
			}
			parentTargets[sourceWork.id] = targetWork
		}
		if original, duplicate := seen[key]; duplicate {
			if rel.Type == WorkRelationDependsOn {
				return nil, fmt.Errorf(
					"work_request: relations[%d] duplicates relations[%d] (%q %q -> %q with requiredState %q)",
					i,
					original,
					rel.Type,
					rel.SourceWorkName,
					rel.TargetWorkName,
					normalized.RequiredState,
				)
			}
			return nil, fmt.Errorf(
				"work_request: relations[%d] duplicates relations[%d] (%q %q -> %q)",
				i,
				original,
				rel.Type,
				rel.SourceWorkName,
				rel.TargetWorkName,
			)
		}
		seen[key] = i
		relIndex[rel.SourceWorkName] = append(relIndex[rel.SourceWorkName], normalized)
		if rel.Type == WorkRelationDependsOn {
			dependencyGraph[sourceWork.id] = append(dependencyGraph[sourceWork.id], targetWork.id)
		}
	}
	if err := rejectDependencyCycles(dependencyGraph); err != nil {
		return nil, err
	}
	return relIndex, nil
}

func validateBatchRelationSource(i int, rel WorkRelation, workIndex map[string]normalizedBatchWork) (normalizedBatchWork, error) {
	if strings.TrimSpace(rel.SourceWorkName) == "" {
		return normalizedBatchWork{}, missingBatchRelationEndpointError(i, rel, "sourceWorkName")
	}
	sourceWork, ok := workIndex[rel.SourceWorkName]
	if !ok {
		return normalizedBatchWork{}, missingBatchRelationEndpointError(i, rel, "sourceWorkName")
	}
	return sourceWork, nil
}

// validateBatchRelationShape checks the parts that can be known without a
// Factory Session. DEPENDS_ON target existence is deliberately deferred to
// live admission because its target may already be on the selected board.
func validateBatchRelationShape(i int, rel WorkRelation, workIndex map[string]normalizedBatchWork) error {
	if _, err := validateBatchRelationSource(i, rel, workIndex); err != nil {
		return err
	}
	if strings.TrimSpace(rel.TargetWorkName) == "" && strings.TrimSpace(rel.TargetWorkID) == "" {
		return missingBatchRelationEndpointError(i, rel, "targetWorkName")
	}
	if rel.Type == WorkRelationParentChild {
		if targetName := strings.TrimSpace(rel.TargetWorkName); targetName != "" {
			if _, ok := workIndex[rel.TargetWorkName]; !ok {
				return missingBatchRelationEndpointError(i, rel, "targetWorkName")
			}
		} else {
			found := false
			for _, work := range workIndex {
				if work.id == strings.TrimSpace(rel.TargetWorkID) {
					found = true
					break
				}
			}
			if !found {
				return missingBatchRelationEndpointError(i, rel, "targetWorkId")
			}
		}
	}
	return nil
}

func resolveBatchRelationTarget(
	i int,
	rel WorkRelation,
	workIndex map[string]normalizedBatchWork,
	existing []ExistingWork,
) (normalizedBatchWork, error) {
	targetName := strings.TrimSpace(rel.TargetWorkName)
	targetID := strings.TrimSpace(rel.TargetWorkID)
	if targetName == "" && targetID == "" {
		return normalizedBatchWork{}, missingBatchRelationEndpointError(i, rel, "targetWorkName")
	}

	byName := relationTargetCandidates(workIndex, existing, targetName, "name")
	byID := relationTargetCandidates(workIndex, existing, targetID, "id")
	if targetName != "" && len(byName) == 0 && targetID == "" {
		return normalizedBatchWork{}, unknownRelationTargetError(i, rel, "targetWorkName", rel.TargetWorkName)
	}
	if targetName != "" && targetID == "" && len(byName) > 1 {
		return normalizedBatchWork{}, ambiguousRelationTargetNameError(i, rel)
	}
	if targetID != "" && len(byID) == 0 {
		return normalizedBatchWork{}, unknownRelationTargetError(i, rel, "targetWorkId", rel.TargetWorkID)
	}
	if targetID != "" && len(byID) > 1 {
		return normalizedBatchWork{}, fmt.Errorf(
			"work_request: relations[%d] targetWorkId=%q identifies multiple Work items on this Factory Session board",
			i,
			rel.TargetWorkID,
		)
	}

	if targetName != "" && targetID != "" {
		for _, candidate := range byName {
			if candidate.id == byID[0].id {
				return byID[0], nil
			}
		}
		return normalizedBatchWork{}, fmt.Errorf(
			"work_request: relations[%d] targetWorkName=%q and targetWorkId=%q identify different Work items; provide matching references",
			i,
			rel.TargetWorkName,
			rel.TargetWorkID,
		)
	}
	if targetName != "" {
		return byName[0], nil
	}
	return byID[0], nil
}

func relationTargetCandidates(
	workIndex map[string]normalizedBatchWork,
	existing []ExistingWork,
	value string,
	field string,
) []normalizedBatchWork {
	if value == "" {
		return nil
	}
	var candidates []normalizedBatchWork
	for _, target := range workIndex {
		if (field == "name" && target.name == value) || (field == "id" && target.id == value) {
			candidates = appendUniqueRelationTarget(candidates, target)
		}
	}
	for _, target := range existing {
		candidate := normalizedBatchWork{
			name:       strings.TrimSpace(target.Name),
			id:         strings.TrimSpace(target.WorkID),
			workTypeID: target.WorkTypeID,
		}
		if candidate.id == "" || (field == "name" && candidate.name != value) || (field == "id" && candidate.id != value) {
			continue
		}
		candidates = appendUniqueRelationTarget(candidates, candidate)
	}
	return candidates
}

func appendUniqueRelationTarget(candidates []normalizedBatchWork, candidate normalizedBatchWork) []normalizedBatchWork {
	for _, existing := range candidates {
		if existing.id == candidate.id {
			return candidates
		}
	}
	return append(candidates, candidate)
}

func relationTargetField(rel WorkRelation) string {
	if strings.TrimSpace(rel.TargetWorkID) != "" {
		return "targetWorkId"
	}
	return "targetWorkName"
}

func relationTargetValue(rel WorkRelation) string {
	if strings.TrimSpace(rel.TargetWorkID) != "" {
		return rel.TargetWorkID
	}
	return rel.TargetWorkName
}

func unknownRelationTargetError(i int, rel WorkRelation, field, value string) error {
	return fmt.Errorf(
		"work_request: relations[%d] relation type %q has sourceWorkName %q; unknown %s=%q does not identify a Work on this Factory Session board; correct %s or provide targetWorkId",
		i,
		rel.Type,
		rel.SourceWorkName,
		field,
		value,
		field,
	)
}

func ambiguousRelationTargetNameError(i int, rel WorkRelation) error {
	return fmt.Errorf(
		"work_request: relations[%d] targetWorkName=%q is ambiguous on this Factory Session board; use targetWorkId to select one Work",
		i,
		rel.TargetWorkName,
	)
}

func missingBatchRelationEndpointError(i int, rel WorkRelation, endpointField string) error {
	endpointValue := rel.SourceWorkName
	targetDescription := fmt.Sprintf("targetWorkName %q", rel.TargetWorkName)
	if endpointField == "targetWorkName" {
		endpointValue = rel.TargetWorkName
	}
	if endpointField == "targetWorkId" {
		endpointValue = rel.TargetWorkID
		targetDescription = fmt.Sprintf("targetWorkId %q", rel.TargetWorkID)
	}
	return fmt.Errorf(
		"work_request: relations[%d] relation type %q has sourceWorkName %q and %s; endpoint %s=%q is missing from this batch; relation endpoints must name Work declared in this batch's works[] (not previously submitted Work); add the named Work to works[] or correct %s",
		i,
		rel.Type,
		rel.SourceWorkName,
		targetDescription,
		endpointField,
		endpointValue,
		endpointField,
	)
}

func normalizeBatchRelation(i int, rel WorkRelation, sourceWork, targetWork normalizedBatchWork, opts NormalizeOptions) (Relation, string, error) {
	switch rel.Type {
	case WorkRelationDependsOn:
		return normalizeDependsOnRelation(i, rel, sourceWork, targetWork, opts)
	case WorkRelationParentChild:
		return normalizeParentChildRelation(i, rel, sourceWork, targetWork)
	default:
		return Relation{}, "", fmt.Errorf("work_request: relations[%d] has unsupported type %q", i, rel.Type)
	}
}

func normalizeDependsOnRelation(i int, rel WorkRelation, sourceWork, targetWork normalizedBatchWork, opts NormalizeOptions) (Relation, string, error) {
	if sourceWork.id == targetWork.id {
		return Relation{}, "", fmt.Errorf("work_request: relations[%d] has self-dependency on %q", i, rel.SourceWorkName)
	}
	requiredState := rel.RequiredState
	if requiredState == "" {
		requiredState = "complete"
	}
	if opts.ValidStatesByType != nil && !opts.ValidStatesByType[targetWork.workTypeID][requiredState] {
		return Relation{}, "", fmt.Errorf(
			"work_request: relations[%d] references unknown requiredState %q for target work type %q",
			i,
			requiredState,
			targetWork.workTypeID,
		)
	}
	return Relation{
		Type:          RelationDependsOn,
		TargetWorkID:  targetWork.id,
		RequiredState: requiredState,
	}, relationValidationKey(rel.Type, sourceWork.id, targetWork.id, requiredState), nil
}

func normalizeParentChildRelation(i int, rel WorkRelation, sourceWork, targetWork normalizedBatchWork) (Relation, string, error) {
	if sourceWork.id == targetWork.id {
		return Relation{}, "", fmt.Errorf("work_request: relations[%d] has self-parenting on %q", i, rel.SourceWorkName)
	}
	if rel.RequiredState != "" {
		return Relation{}, "", fmt.Errorf("work_request: relations[%d] must not set requiredState for PARENT_CHILD", i)
	}
	return Relation{
		Type:         RelationParentChild,
		TargetWorkID: targetWork.id,
	}, relationValidationKey(rel.Type, sourceWork.id, targetWork.id, ""), nil
}

func relationValidationKey(relType WorkRelationType, sourceWorkName string, targetWorkName string, requiredState string) string {
	return fmt.Sprintf("%s|%s|%s|%s", relType, sourceWorkName, targetWorkName, requiredState)
}

func rejectDependencyCycles(graph map[string][]string) error {
	for name := range graph {
		sort.Strings(graph[name])
	}

	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(string) bool
	visit = func(name string) bool {
		if visiting[name] {
			return true
		}
		if visited[name] {
			return false
		}
		visiting[name] = true
		for _, target := range graph[name] {
			if visit(target) {
				return true
			}
		}
		visiting[name] = false
		visited[name] = true
		return false
	}

	names := make([]string, 0, len(graph))
	for name := range graph {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if visit(name) {
			return fmt.Errorf("work_request: dependency cycle detected involving %q", name)
		}
	}
	return nil
}

func batchTraceID(req Request) string {
	if req.CurrentChainingTraceID != "" {
		return req.CurrentChainingTraceID
	}
	for _, work := range req.Works {
		if work.CurrentChainingTraceID != "" {
			return work.CurrentChainingTraceID
		}
		if work.TraceID != "" {
			return work.TraceID
		}
	}
	return "trace-" + req.RequestID
}

func generatedWorkIdentity(prefix string, generateID IDGenerator) (string, error) {
	if generateID == nil {
		return "", fmt.Errorf("work_request: ID generator is required")
	}
	identity := strings.TrimSpace(generateID())
	if identity == "" {
		return "", fmt.Errorf("work_request: ID generator returned an empty identity")
	}
	return prefix + "-" + identity, nil
}

func rawWorkPayload(payload any) ([]byte, error) {
	switch value := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), value...), nil
	case json.RawMessage:
		return append([]byte(nil), value...), nil
	case string:
		return []byte(value), nil
	default:
		return json.Marshal(value)
	}
}

func normalizeWorkContent(content []ContentPart, payload any) ([]ContentPart, []byte, error) {
	rawPayload, err := rawWorkPayload(payload)
	if err != nil {
		return nil, nil, err
	}

	if len(content) == 0 {
		return canonicalContentFromLegacyPayload(rawPayload), rawPayload, nil
	}

	canonical, err := normalizeFileBackedContent(content)
	if err != nil {
		return nil, nil, err
	}
	legacyText, hasTextProjection, err := legacyTextPayloadFromCanonicalContent(canonical)
	if err != nil {
		return nil, nil, err
	}
	if len(rawPayload) == 0 {
		if hasTextProjection {
			return canonical, []byte(legacyText), nil
		}
		return canonical, nil, nil
	}

	payloadText, ok := legacyPayloadAsText(rawPayload)
	if !ok {
		return nil, nil, fmt.Errorf("payload conflicts with explicit content")
	}
	if !hasTextProjection {
		return nil, nil, fmt.Errorf("payload cannot be combined with image-only canonical content")
	}
	if payloadText != legacyText {
		return nil, nil, fmt.Errorf("payload conflicts with explicit content")
	}
	return canonical, []byte(legacyText), nil
}

func canonicalContentFromLegacyPayload(rawPayload []byte) []ContentPart {
	payloadText, ok := legacyPayloadAsText(rawPayload)
	if !ok {
		return nil
	}
	return []ContentPart{{
		Type: ContentPartTypeText,
		Text: payloadText,
	}}
}

func legacyTextPayloadFromCanonicalContent(content []ContentPart) (string, bool, error) {
	var builder strings.Builder
	hasText := false
	for _, part := range content {
		switch part.Type.Normalized() {
		case ContentPartTypeText:
			hasText = true
			builder.WriteString(part.Text)
		case ContentPartTypeImage:
			continue
		default:
			continue
		}
	}
	return builder.String(), hasText, nil
}

func legacyPayloadAsText(rawPayload []byte) (string, bool) {
	if len(rawPayload) == 0 {
		return "", false
	}

	var text string
	if err := json.Unmarshal(rawPayload, &text); err == nil {
		return text, true
	}
	if json.Valid(rawPayload) {
		return "", false
	}
	return string(rawPayload), true
}

func normalizeSubmitChainingTraceDepth(depth int, currentChainingTraceID string, traceID string) int {
	if depth > 0 {
		return depth
	}
	if ResolveWorkRequestCurrentChainingTraceID(currentChainingTraceID, traceID) != "" {
		return 1
	}
	return 0
}

func appendUniquePetriRelations(base []Relation, extra []Relation) []Relation {
	for _, relation := range extra {
		if hasPetriRelation(base, relation) {
			continue
		}
		base = append(base, relation)
	}
	return base
}

func hasPetriRelation(relations []Relation, candidate Relation) bool {
	for _, relation := range relations {
		if relation.Type == candidate.Type &&
			relation.TargetWorkID == candidate.TargetWorkID &&
			relation.RequiredState == candidate.RequiredState {
			return true
		}
	}
	return false
}
