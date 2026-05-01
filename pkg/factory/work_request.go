package factory

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// NormalizeWorkRequest validates a FACTORY_REQUEST_BATCH and converts it into runtime submit requests.
func NormalizeWorkRequest(req interfaces.WorkRequest, opts interfaces.WorkRequestNormalizeOptions) ([]interfaces.SubmitRequest, error) {
	if req.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		return nil, fmt.Errorf("work_request: unsupported type %q", req.Type)
	}
	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}
	if len(req.Works) == 0 {
		return nil, fmt.Errorf("work_request: works array must contain at least one item")
	}

	workIndex, err := validateBatchWork(req, opts)
	if err != nil {
		return nil, err
	}
	relIndex, err := validateAndIndexBatchRelations(req, workIndex, opts)
	if err != nil {
		return nil, err
	}
	if err := rejectDependencyCycles(req.Relations); err != nil {
		return nil, err
	}

	traceID := batchTraceID(req)
	normalized := make([]interfaces.SubmitRequest, 0, len(req.Works))
	for i, work := range req.Works {
		workTypeID := work.WorkTypeID
		if workTypeID == "" {
			workTypeID = opts.DefaultWorkTypeID
		}
		payload, err := rawWorkPayload(work.Payload)
		if err != nil {
			return nil, fmt.Errorf("work_request: works[%d] (%q) has invalid payload: %w", i, work.Name, err)
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

		tags := make(map[string]string, len(work.Tags)+2)
		maps.Copy(tags, work.Tags)
		tags["_work_name"] = work.Name
		tags["_work_type"] = workTypeID

		normalized = append(normalized, interfaces.SubmitRequest{
			RequestID:                itemRequestID,
			WorkID:                   workIndex[work.Name].id,
			Name:                     work.Name,
			WorkTypeID:               workTypeID,
			CurrentChainingTraceID:   itemCurrentChainingTraceID,
			PreviousChainingTraceIDs: interfaces.CanonicalChainingTraceIDs(work.PreviousChainingTraceIDs),
			TraceID:                  itemTraceID,
			Payload:                  payload,
			Tags:                     tags,
			TargetState:              work.State,
			ExecutionID:              work.ExecutionID,
			Relations: appendUniquePetriRelations(
				clonePetriRelations(relIndex[work.Name]),
				work.RuntimeRelations,
			),
		})
	}
	return normalized, nil
}

// NormalizeGeneratedSubmissionBatch validates the canonical generated request
// and merges optional runtime submission fields onto the matching work items.
func NormalizeGeneratedSubmissionBatch(batch interfaces.GeneratedSubmissionBatch, opts interfaces.WorkRequestNormalizeOptions) ([]interfaces.SubmitRequest, error) {
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
		match := -1
		if submitted.WorkID != "" {
			for i, req := range normalized {
				if usedByWorkID[req.WorkID] {
					continue
				}
				if req.WorkID == submitted.WorkID {
					match = i
					break
				}
			}
		}
		if match == -1 && submitted.Name != "" {
			for i, req := range normalized {
				if usedByName[req.Name] {
					continue
				}
				if req.Name == submitted.Name {
					match = i
					break
				}
			}
		}
		if match < 0 {
			continue
		}
		usedByWorkID[normalized[match].WorkID] = true
		usedByName[normalized[match].Name] = true

		next := normalized[match]
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
			next.Relations = appendUniquePetriRelations(clonePetriRelations(submitted.Relations), next.Relations)
		}
		if len(submitted.PreviousChainingTraceIDs) > 0 {
			next.PreviousChainingTraceIDs = interfaces.CanonicalChainingTraceIDs(submitted.PreviousChainingTraceIDs)
		}
		normalized[match] = next
	}

	return normalized, nil
}

// WorkRequestRecordFromSubmitRequests builds the canonical request-history
// record for a normalized batch submission.
func WorkRequestRecordFromSubmitRequests(requestID string, source string, requests []interfaces.SubmitRequest) interfaces.WorkRequestRecord {
	workItems := make([]interfaces.FactoryWorkItem, 0, len(requests))
	workNamesByID := make(map[string]string, len(requests))
	traceID := ""
	for _, req := range requests {
		if traceID == "" {
			traceID = req.TraceID
		}
		name := SubmitWorkName(req)
		workNamesByID[req.WorkID] = name
		workItems = append(workItems, interfaces.FactoryWorkItem{
			ID:                       req.WorkID,
			WorkTypeID:               req.WorkTypeID,
			State:                    req.TargetState,
			DisplayName:              name,
			CurrentChainingTraceID:   ResolveWorkRequestCurrentChainingTraceID(req.CurrentChainingTraceID, req.TraceID),
			PreviousChainingTraceIDs: interfaces.CanonicalChainingTraceIDs(req.PreviousChainingTraceIDs),
			TraceID:                  req.TraceID,
			Tags:                     maps.Clone(req.Tags),
		})
	}

	var relations []interfaces.FactoryRelation
	for _, req := range requests {
		for _, relation := range req.Relations {
			relations = append(relations, interfaces.FactoryRelation{
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

	return interfaces.WorkRequestRecord{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   traceID,
		Source:    source,
		WorkItems: workItems,
		Relations: relations,
	}
}

// SubmitWorkName returns the canonical display name for a submit request.
func SubmitWorkName(req interfaces.SubmitRequest) string {
	if req.Name != "" {
		return req.Name
	}
	if req.Tags != nil && req.Tags["_work_name"] != "" {
		return req.Tags["_work_name"]
	}
	return req.WorkID
}

type normalizedBatchWork struct {
	id         string
	workTypeID string
}

func validateBatchWork(req interfaces.WorkRequest, opts interfaces.WorkRequestNormalizeOptions) (map[string]normalizedBatchWork, error) {
	workNames := make(map[string]bool, len(req.Works))
	workIndex := make(map[string]normalizedBatchWork, len(req.Works))
	for i, work := range req.Works {
		if strings.TrimSpace(work.Name) == "" {
			return nil, fmt.Errorf("work_request: works[%d] is missing required name", i)
		}
		if workNames[work.Name] {
			return nil, fmt.Errorf("work_request: works[%d] has duplicate name %q", i, work.Name)
		}
		workNames[work.Name] = true

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
			id:         workID,
			workTypeID: workTypeID,
		}
	}
	return workIndex, nil
}

func validateAndIndexBatchRelations(req interfaces.WorkRequest, workIndex map[string]normalizedBatchWork, opts interfaces.WorkRequestNormalizeOptions) (map[string][]interfaces.Relation, error) {
	relIndex := make(map[string][]interfaces.Relation)
	seen := map[string]int{}
	parentTargets := make(map[string]string)
	for i, rel := range req.Relations {
		targetWork, err := validateBatchRelationEndpoints(i, rel, workIndex)
		if err != nil {
			return nil, err
		}
		normalized, key, err := normalizeBatchRelation(i, rel, targetWork, opts)
		if err != nil {
			return nil, err
		}
		if rel.Type == interfaces.WorkRelationParentChild {
			if existingTarget, ok := parentTargets[rel.SourceWorkName]; ok && existingTarget != rel.TargetWorkName {
				return nil, fmt.Errorf(
					"work_request: relations[%d] assigns multiple PARENT_CHILD parents to %q (%q and %q)",
					i,
					rel.SourceWorkName,
					existingTarget,
					rel.TargetWorkName,
				)
			}
			parentTargets[rel.SourceWorkName] = rel.TargetWorkName
		}
		if err := rejectDuplicateBatchRelation(i, rel, normalized.RequiredState, key, seen); err != nil {
			return nil, err
		}
		seen[key] = i
		relIndex[rel.SourceWorkName] = append(relIndex[rel.SourceWorkName], normalized)
	}
	return relIndex, nil
}

func validateBatchRelationEndpoints(i int, rel interfaces.WorkRelation, workIndex map[string]normalizedBatchWork) (normalizedBatchWork, error) {
	if strings.TrimSpace(rel.SourceWorkName) == "" {
		return normalizedBatchWork{}, fmt.Errorf("work_request: relations[%d] is missing sourceWorkName", i)
	}
	if strings.TrimSpace(rel.TargetWorkName) == "" {
		return normalizedBatchWork{}, fmt.Errorf("work_request: relations[%d] is missing targetWorkName", i)
	}
	if _, ok := workIndex[rel.SourceWorkName]; !ok {
		return normalizedBatchWork{}, fmt.Errorf("work_request: relations[%d] references unknown sourceWorkName %q", i, rel.SourceWorkName)
	}
	targetWork, ok := workIndex[rel.TargetWorkName]
	if !ok {
		return normalizedBatchWork{}, fmt.Errorf("work_request: relations[%d] references unknown targetWorkName %q", i, rel.TargetWorkName)
	}
	return targetWork, nil
}

func normalizeBatchRelation(i int, rel interfaces.WorkRelation, targetWork normalizedBatchWork, opts interfaces.WorkRequestNormalizeOptions) (interfaces.Relation, string, error) {
	switch rel.Type {
	case interfaces.WorkRelationDependsOn:
		return normalizeDependsOnRelation(i, rel, targetWork, opts)
	case interfaces.WorkRelationParentChild:
		return normalizeParentChildRelation(i, rel, targetWork)
	default:
		return interfaces.Relation{}, "", fmt.Errorf("work_request: relations[%d] has unsupported type %q", i, rel.Type)
	}
}

func normalizeDependsOnRelation(i int, rel interfaces.WorkRelation, targetWork normalizedBatchWork, opts interfaces.WorkRequestNormalizeOptions) (interfaces.Relation, string, error) {
	if rel.SourceWorkName == rel.TargetWorkName {
		return interfaces.Relation{}, "", fmt.Errorf("work_request: relations[%d] has self-dependency on %q", i, rel.SourceWorkName)
	}
	requiredState := rel.RequiredState
	if requiredState == "" {
		requiredState = "complete"
	}
	if opts.ValidStatesByType != nil && !opts.ValidStatesByType[targetWork.workTypeID][requiredState] {
		return interfaces.Relation{}, "", fmt.Errorf(
			"work_request: relations[%d] references unknown requiredState %q for target work type %q",
			i,
			requiredState,
			targetWork.workTypeID,
		)
	}
	return interfaces.Relation{
		Type:          interfaces.RelationDependsOn,
		TargetWorkID:  targetWork.id,
		RequiredState: requiredState,
	}, relationValidationKey(rel.Type, rel.SourceWorkName, rel.TargetWorkName, requiredState), nil
}

func normalizeParentChildRelation(i int, rel interfaces.WorkRelation, targetWork normalizedBatchWork) (interfaces.Relation, string, error) {
	if rel.SourceWorkName == rel.TargetWorkName {
		return interfaces.Relation{}, "", fmt.Errorf("work_request: relations[%d] has self-parenting on %q", i, rel.SourceWorkName)
	}
	if rel.RequiredState != "" {
		return interfaces.Relation{}, "", fmt.Errorf("work_request: relations[%d] must not set requiredState for PARENT_CHILD", i)
	}
	return interfaces.Relation{
		Type:         interfaces.RelationParentChild,
		TargetWorkID: targetWork.id,
	}, relationValidationKey(rel.Type, rel.SourceWorkName, rel.TargetWorkName, ""), nil
}

func rejectDuplicateBatchRelation(i int, rel interfaces.WorkRelation, requiredState string, key string, seen map[string]int) error {
	original, duplicate := seen[key]
	if !duplicate {
		return nil
	}
	if rel.Type == interfaces.WorkRelationDependsOn {
		return fmt.Errorf(
			"work_request: relations[%d] duplicates relations[%d] (%q %q -> %q with requiredState %q)",
			i,
			original,
			rel.Type,
			rel.SourceWorkName,
			rel.TargetWorkName,
			requiredState,
		)
	}
	return fmt.Errorf(
		"work_request: relations[%d] duplicates relations[%d] (%q %q -> %q)",
		i,
		original,
		rel.Type,
		rel.SourceWorkName,
		rel.TargetWorkName,
	)
}

func relationValidationKey(relType interfaces.WorkRelationType, sourceWorkName string, targetWorkName string, requiredState string) string {
	return fmt.Sprintf("%s|%s|%s|%s", relType, sourceWorkName, targetWorkName, requiredState)
}

func rejectDependencyCycles(relations []interfaces.WorkRelation) error {
	graph := make(map[string][]string)
	for _, rel := range relations {
		if rel.Type == interfaces.WorkRelationDependsOn {
			graph[rel.SourceWorkName] = append(graph[rel.SourceWorkName], rel.TargetWorkName)
		}
	}
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

func batchTraceID(req interfaces.WorkRequest) string {
	if req.CurrentChainingTraceID != "" {
		return req.CurrentChainingTraceID
	}
	works := req.Works
	for _, work := range works {
		if work.CurrentChainingTraceID != "" {
			return work.CurrentChainingTraceID
		}
		if work.TraceID != "" {
			return work.TraceID
		}
	}
	return newTraceID()
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

func clonePetriRelations(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.Relation, len(relations))
	copy(out, relations)
	return out
}

func appendUniquePetriRelations(base []interfaces.Relation, extra []interfaces.Relation) []interfaces.Relation {
	for _, relation := range extra {
		if hasPetriRelation(base, relation) {
			continue
		}
		base = append(base, relation)
	}
	return base
}

func hasPetriRelation(relations []interfaces.Relation, candidate interfaces.Relation) bool {
	for _, relation := range relations {
		if relation.Type == candidate.Type &&
			relation.TargetWorkID == candidate.TargetWorkID &&
			relation.RequiredState == candidate.RequiredState {
			return true
		}
	}
	return false
}
