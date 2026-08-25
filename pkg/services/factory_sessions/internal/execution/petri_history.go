package factorysessionexecution

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// PetriTokenSummary is the bounded durable representation of one terminal
// public Work token. It intentionally contains identity, placement, and
// outcome facts only; worker output, Work content, payloads, and structured
// results remain owned by their canonical Work, Worker Session, and Recording
// projections.
type PetriTokenSummary struct {
	TokenID      string                      `json:"tokenId"`
	WorkID       string                      `json:"workId"`
	WorkTypeID   string                      `json:"workTypeId"`
	Name         string                      `json:"name,omitempty"`
	RequestID    string                      `json:"requestId,omitempty"`
	TraceID      string                      `json:"traceId,omitempty"`
	ParentID     string                      `json:"parentId,omitempty"`
	PlaceID      string                      `json:"placeId"`
	State        string                      `json:"state,omitempty"`
	Outcome      workerexecution.WorkOutcome `json:"outcome"`
	DispatchID   string                      `json:"dispatchId,omitempty"`
	TransitionID string                      `json:"transitionId,omitempty"`
	MutationType string                      `json:"mutationType"`
	Reason       string                      `json:"reason,omitempty"`
	Retired      bool                        `json:"retired,omitempty"`
}

type petriTokenHistoryFact struct {
	summary              PetriTokenSummary
	terminalSummary      PetriTokenSummary
	terminal             bool
	transitionReachable  bool
	retired              bool
	hasTerminalSummary   bool
	seen                 bool
	lastMutationPosition int
}

type petriWorkHistoryFacts struct {
	tokens map[string]*petriTokenHistoryFact
}

type petriHistoryIndex struct {
	factsByTokenID    map[string]*petriTokenHistoryFact
	works             map[string]*petriWorkHistoryFacts
	recordWorkIDs     []string
	retainedSummaries map[string]PetriTokenSummary
}

// compactPetriTokenHistory removes lossless mutation records only when the
// runtime supplied a terminal fact and proved that the token is no longer
// usable by a live transition. It is pure so callers can persist a candidate
// and publish it only after the unchanged Store.Save operation succeeds.
func compactPetriTokenHistory(
	mutations []interfaces.TokenMutationRecord,
	summaries []PetriTokenSummary,
) ([]interfaces.TokenMutationRecord, []PetriTokenSummary) {
	index := indexPetriTokenHistory(mutations, summaries)
	eligible := eligiblePetriTokenSummaries(index.works)
	retainedMutations := retainPetriMutations(mutations, index.recordWorkIDs, eligible)
	for workID, summary := range eligible {
		index.retainedSummaries[workID] = summary
	}
	return retainedMutations, sortedPetriTokenSummaries(index.retainedSummaries)
}

func indexPetriTokenHistory(
	mutations []interfaces.TokenMutationRecord,
	summaries []PetriTokenSummary,
) petriHistoryIndex {
	index := petriHistoryIndex{
		factsByTokenID:    make(map[string]*petriTokenHistoryFact),
		works:             make(map[string]*petriWorkHistoryFacts),
		recordWorkIDs:     make([]string, len(mutations)),
		retainedSummaries: petriSummaryMap(summaries),
	}
	for position, mutation := range mutations {
		index.addMutation(position, mutation)
	}
	return index
}

func (index *petriHistoryIndex) addMutation(position int, mutation interfaces.TokenMutationRecord) {
	tokenID := petriMutationTokenID(mutation)
	if tokenID == "" {
		return
	}
	fact := index.factsByTokenID[tokenID]
	if fact == nil {
		fact = &petriTokenHistoryFact{}
		index.factsByTokenID[tokenID] = fact
	}
	fact.seen = true
	fact.lastMutationPosition = position
	updatePetriTokenSummary(&fact.summary, tokenID, mutation)
	updatePetriTokenStatus(fact, mutation)
	workID := strings.TrimSpace(fact.summary.WorkID)
	if workID == "" {
		return
	}
	index.recordWorkIDs[position] = workID
	workFacts := index.works[workID]
	if workFacts == nil {
		workFacts = &petriWorkHistoryFacts{tokens: make(map[string]*petriTokenHistoryFact)}
		index.works[workID] = workFacts
	}
	workFacts.tokens[tokenID] = fact
	delete(index.retainedSummaries, workID)
}

func petriMutationTokenID(mutation interfaces.TokenMutationRecord) string {
	if mutation.Token != nil && strings.TrimSpace(mutation.Token.ID) != "" {
		return strings.TrimSpace(mutation.Token.ID)
	}
	return strings.TrimSpace(mutation.TokenID)
}

func updatePetriTokenSummary(summary *PetriTokenSummary, tokenID string, mutation interfaces.TokenMutationRecord) {
	if summary.TokenID == "" {
		summary.TokenID = tokenID
	}
	if mutation.Token != nil {
		color := mutation.Token.Color
		if summary.WorkID == "" {
			summary.WorkID = strings.TrimSpace(color.WorkID)
		}
		if summary.WorkTypeID == "" {
			summary.WorkTypeID = strings.TrimSpace(color.WorkTypeID)
		}
		if summary.Name == "" {
			summary.Name = color.Name
		}
		if summary.RequestID == "" {
			summary.RequestID = color.RequestID
		}
		if summary.TraceID == "" {
			summary.TraceID = color.TraceID
		}
		if summary.ParentID == "" {
			summary.ParentID = color.ParentID
		}
	}
	if summary.PlaceID == "" && mutation.Token != nil && mutation.ToPlace == "" {
		summary.PlaceID = mutation.Token.State
	}
	if mutation.ToPlace != "" {
		summary.PlaceID = mutation.ToPlace
	} else if mutation.Type == interfaces.MutationConsume && mutation.FromPlace != "" {
		summary.PlaceID = mutation.FromPlace
	}
	summary.State = stateFromPetriPlaceID(summary.PlaceID)
	summary.Outcome = mutation.Outcome
	summary.DispatchID = mutation.DispatchID
	summary.TransitionID = mutation.TransitionID
	summary.MutationType = string(mutation.Type)
	summary.Reason = mutation.Reason
}

func updatePetriTokenStatus(fact *petriTokenHistoryFact, mutation interfaces.TokenMutationRecord) {
	switch mutation.Type {
	case interfaces.MutationMove, interfaces.MutationCreate:
		fact.retired = false
		fact.terminal = mutation.Terminal
		fact.transitionReachable = mutation.TransitionReachable
		fact.hasTerminalSummary = mutation.Terminal
		if mutation.Terminal {
			fact.terminalSummary = clonePetriTokenSummary(fact.summary)
		}
	case interfaces.MutationConsume:
		fact.retired = true
		if !mutation.Terminal {
			return
		}
		fact.terminal = true
		fact.transitionReachable = false
		if fact.hasTerminalSummary {
			fact.summary = clonePetriTokenSummary(fact.terminalSummary)
		}
	}
}

func eligiblePetriTokenSummaries(
	works map[string]*petriWorkHistoryFacts,
) map[string]PetriTokenSummary {
	eligible := make(map[string]PetriTokenSummary)
	for workID, workFacts := range works {
		selected, ok := terminalPetriTokenSummary(workFacts)
		if !ok {
			continue
		}
		selected.WorkID = workID
		selected.State = stateFromPetriPlaceID(selected.PlaceID)
		eligible[workID] = selected
	}
	return eligible
}

func terminalPetriTokenSummary(workFacts *petriWorkHistoryFacts) (PetriTokenSummary, bool) {
	if workFacts == nil || len(workFacts.tokens) == 0 {
		return PetriTokenSummary{}, false
	}
	var selected *petriTokenHistoryFact
	for _, fact := range workFacts.tokens {
		if !fact.seen || !fact.terminal || (!fact.retired && fact.transitionReachable) {
			return PetriTokenSummary{}, false
		}
		if selected == nil || fact.lastMutationPosition > selected.lastMutationPosition {
			selected = fact
		}
	}
	if selected == nil {
		return PetriTokenSummary{}, false
	}
	summary := clonePetriTokenSummary(selected.summary)
	summary.Retired = selected.retired
	return summary, true
}

func retainPetriMutations(
	mutations []interfaces.TokenMutationRecord,
	recordWorkIDs []string,
	eligible map[string]PetriTokenSummary,
) []interfaces.TokenMutationRecord {
	retained := make([]interfaces.TokenMutationRecord, 0, len(mutations))
	for index, mutation := range mutations {
		if index < len(recordWorkIDs) {
			if _, compact := eligible[recordWorkIDs[index]]; compact {
				continue
			}
		}
		retained = append(retained, clonePetriMutationRecord(mutation))
	}
	return retained
}

func petriSummaryMap(summaries []PetriTokenSummary) map[string]PetriTokenSummary {
	result := make(map[string]PetriTokenSummary, len(summaries))
	for _, summary := range summaries {
		if workID := strings.TrimSpace(summary.WorkID); workID != "" {
			result[workID] = clonePetriTokenSummary(summary)
		}
	}
	return result
}

func sortedPetriTokenSummaries(values map[string]PetriTokenSummary) []PetriTokenSummary {
	workIDs := make([]string, 0, len(values))
	for workID := range values {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	result := make([]PetriTokenSummary, 0, len(workIDs))
	for _, workID := range workIDs {
		result = append(result, clonePetriTokenSummary(values[workID]))
	}
	return result
}

func compactRuntimePetriHistory(state *runtimeSessionState) {
	if state == nil {
		return
	}
	state.petriMutations, state.petriSummaries = compactPetriTokenHistory(
		state.petriMutations,
		state.petriSummaries,
	)
}

func stateFromPetriPlaceID(placeID string) string {
	placeID = strings.TrimSpace(placeID)
	if index := strings.LastIndexByte(placeID, ':'); index >= 0 {
		return placeID[index+1:]
	}
	return placeID
}

func clonePetriTokenSummary(summary PetriTokenSummary) PetriTokenSummary {
	return summary
}

func clonePetriTokenSummaries(summaries []PetriTokenSummary) []PetriTokenSummary {
	if len(summaries) == 0 {
		return nil
	}
	cloned := make([]PetriTokenSummary, len(summaries))
	copy(cloned, summaries)
	return cloned
}

func clonePetriMutationRecord(mutation interfaces.TokenMutationRecord) interfaces.TokenMutationRecord {
	cloned := mutation
	if mutation.Token != nil {
		token := *clonePetriMutations([]interfaces.TokenMutationRecord{mutation})[0].Token
		token.Color.Content = work.CloneWorkContentParts(mutation.Token.Color.Content)
		token.Color.Payload = append([]byte(nil), mutation.Token.Color.Payload...)
		token.Color.PreviousChainingTraceIDs = append([]string(nil), mutation.Token.Color.PreviousChainingTraceIDs...)
		token.Color.Relations = append([]work.Relation(nil), mutation.Token.Color.Relations...)
		if mutation.Token.Color.Tags != nil {
			token.Color.Tags = make(map[string]string, len(mutation.Token.Color.Tags))
			for key, value := range mutation.Token.Color.Tags {
				token.Color.Tags[key] = value
			}
		}
		cloned.Token = &token
	}
	return cloned
}
