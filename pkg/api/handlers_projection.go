package api

import (
	"sort"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func tokenToResponse(t *interfaces.Token, includeHistory bool) factoryapi.TokenResponse {
	resp := factoryapi.TokenResponse{
		Id:                       t.ID,
		PlaceId:                  t.PlaceID,
		WorkId:                   t.Color.WorkID,
		WorkType:                 t.Color.WorkTypeID,
		ChainingTraceDepth:       intPtrIfPositive(t.Color.ChainingTraceDepth),
		CurrentChainingTraceId:   stringPtrIfNotEmpty(firstNonEmptyString(t.Color.CurrentChainingTraceID, t.Color.TraceID)),
		PreviousChainingTraceIds: stringSlicePtrCopy(t.Color.PreviousChainingTraceIDs),
		TraceId:                  t.Color.TraceID,
		Content:                  domainWorkContentToGeneratedPtr(t.Color.Content),
		Tags:                     stringMapPtr(t.Color.Tags),
		CreatedAt:                t.CreatedAt,
		EnteredAt:                t.EnteredAt,
	}
	if t.Color.Name != "" {
		resp.Name = &t.Color.Name
	}
	if len(t.Color.Tags) == 0 {
		resp.Tags = nil
	}
	if includeHistory {
		resp.History = &factoryapi.TokenHistory{
			TotalVisits:         integerMapPtr(t.History.TotalVisits),
			ConsecutiveFailures: integerMapPtr(t.History.ConsecutiveFailures),
			PlaceVisits:         integerMapPtr(t.History.PlaceVisits),
			LastError:           stringPtrIfNotEmpty(t.History.LastError),
		}
	}
	return resp
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

func statusFromEngineStateSnapshot(snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) factoryapi.StatusResponse {
	categories, resources := categorizeStatusTokens(&snapshot.Marking, snapshot.Topology)
	return factoryapi.StatusResponse{
		Categories:    categories,
		FactoryState:  snapshot.FactoryState,
		Resources:     resourceUsagePtr(resources),
		RuntimeStatus: string(snapshot.RuntimeStatus),
		TotalTokens:   countPublicStatusTokens(&snapshot.Marking),
	}
}

func categorizeStatusTokens(marking *petri.MarkingSnapshot, net *state.Net) (factoryapi.StatusCategories, []factoryapi.ResourceUsage) {
	var categories factoryapi.StatusCategories
	resourceCounts := make(map[string]int)
	resourceTotals := resourceTotalsFromTopology(net)

	if marking == nil {
		return categories, resourceUsage(resourceCounts, resourceTotals)
	}

	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		if token.Color.DataType == interfaces.DataTypeResource {
			resourceID, resourceState := state.SplitPlaceID(token.PlaceID)
			if _, ok := resourceTotals[resourceID]; !ok {
				resourceTotals[resourceID]++
			}
			if resourceState == interfaces.ResourceStateAvailable {
				resourceCounts[resourceID]++
			}
			continue
		}

		switch statusStateCategory(net, token.PlaceID) {
		case state.StateCategoryFailed:
			categories.Failed++
		case state.StateCategoryTerminal:
			categories.Terminal++
		case state.StateCategoryInitial:
			categories.Initial++
		default:
			categories.Processing++
		}
	}

	return categories, resourceUsage(resourceCounts, resourceTotals)
}

func countPublicStatusTokens(marking *petri.MarkingSnapshot) int {
	if marking == nil {
		return 0
	}
	count := 0
	for _, token := range marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		count++
	}
	return count
}

func statusStateCategory(net *state.Net, placeID string) state.StateCategory {
	if net == nil {
		return state.StateCategoryProcessing
	}
	return net.StateCategoryForPlace(placeID)
}

func resourceTotalsFromTopology(net *state.Net) map[string]int {
	totals := make(map[string]int)
	if net == nil {
		return totals
	}
	for id, resource := range net.Resources {
		if resource == nil {
			continue
		}
		totals[id] = resource.Capacity
	}
	return totals
}

func resourceUsage(counts map[string]int, totals map[string]int) []factoryapi.ResourceUsage {
	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	resources := make([]factoryapi.ResourceUsage, 0, len(ids))
	for _, id := range ids {
		resources = append(resources, factoryapi.ResourceUsage{
			Available: counts[id],
			Name:      id,
			Total:     totals[id],
		})
	}
	return resources
}

func resourceUsagePtr(values []factoryapi.ResourceUsage) *[]factoryapi.ResourceUsage {
	if len(values) == 0 {
		return nil
	}
	return &values
}
