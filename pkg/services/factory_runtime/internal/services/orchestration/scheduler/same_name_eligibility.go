package scheduler

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func transitionUsesSameNameGuard(tr *petri.Transition) bool {
	if tr == nil {
		return false
	}
	for i := range tr.InputArcs {
		if guardUsesSameNameGuard(tr.InputArcs[i].Guard) {
			return true
		}
	}
	return false
}

func shouldFailClosedSameNameJoin(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) bool {
	return transitionUsesSameNameGuard(tr) && sameNameJoinHasMissingCurrentChild(tr, snapshot)
}

func guardUsesSameNameGuard(guard petri.Guard) bool {
	switch typed := guard.(type) {
	case *petri.SameNameGuard:
		return typed != nil
	case *petri.AllGuard:
		if typed == nil {
			return false
		}
		for _, nested := range typed.Guards {
			if guardUsesSameNameGuard(nested) {
				return true
			}
		}
	}
	return false
}

func sameNameJoinHasMissingCurrentChild(
	tr *petri.Transition,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) bool {
	if tr == nil || snapshot == nil {
		return false
	}
	for i := range tr.InputArcs {
		matchBinding, ok := sameNameMatchBinding(tr.InputArcs[i].Guard)
		if !ok {
			continue
		}
		var peerArc *petri.Arc
		for peerIndex := range tr.InputArcs {
			candidate := &tr.InputArcs[peerIndex]
			if arcKey(candidate) == matchBinding {
				peerArc = candidate
				break
			}
		}
		if peerArc == nil {
			continue
		}
		if sameNameParentHasMissingCurrentChild(
			snapshot,
			stableTokens(snapshot.Marking.TokensInPlace(tr.InputArcs[i].PlaceID)),
			stableTokens(snapshot.Marking.TokensInPlace(peerArc.PlaceID)),
		) || sameNameParentHasMissingCurrentChild(
			snapshot,
			stableTokens(snapshot.Marking.TokensInPlace(peerArc.PlaceID)),
			stableTokens(snapshot.Marking.TokensInPlace(tr.InputArcs[i].PlaceID)),
		) {
			return true
		}
	}
	return false
}

func sameNameParentHasMissingCurrentChild(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	parentCandidates, childCandidates []factorytoken.Token,
) bool {
	for _, parentCandidate := range parentCandidates {
		parentWorkID := parentCandidate.Color.WorkID
		if parentWorkID == "" || parentCandidate.Color.Name == "" || !sameNameParentChildCandidate(
			parentCandidate,
			childCandidates,
		) {
			continue
		}
		registration, registered := snapshot.Marking.ParentChildRegistrations[parentWorkID]
		if !registered {
			// A missing registration does not prove that this is a parent-child
			// population. Preserve the legacy same-name fallback for fresh
			// flows whose relation projection has not been populated yet.
			continue
		}
		if !registration.Complete || len(registration.Children) == 0 ||
			!sameNameRegistrationIsConsistent(registration.Children, parentWorkID) {
			// Once the candidate pair proves the authored parent-child join, an
			// incomplete or contradictory canonical projection cannot authorize
			// an arbitrary historical token through the legacy fallback.
			return true
		}

		foundRegisteredSameNameChild := false
		for index := len(registration.Children) - 1; index >= 0; index-- {
			child := registration.Children[index]
			if child.Color.Name != parentCandidate.Color.Name {
				continue
			}
			foundRegisteredSameNameChild = true
			for _, candidate := range childCandidates {
				if sameNameTokenIdentity(candidate, child) {
					return false
				}
			}
			return true
		}
		if !foundRegisteredSameNameChild {
			// The visible candidate proves the relation, but the canonical
			// projection omits the same-name child. Do not fall back to
			// token ordering when the two authorities disagree.
			return true
		}
	}
	return false
}

func sameNameParentChildCandidate(
	parentCandidate factorytoken.Token,
	childCandidates []factorytoken.Token,
) bool {
	for _, childCandidate := range childCandidates {
		if childCandidate.Color.ParentID == parentCandidate.Color.WorkID &&
			childCandidate.Color.Name == parentCandidate.Color.Name {
			return true
		}
	}
	return false
}

func sameNameRegistrationIsConsistent(children []factorytoken.Token, parentWorkID string) bool {
	seen := make(map[string]struct{}, len(children))
	for _, child := range children {
		if child.Color.ParentID != parentWorkID {
			return false
		}
		identity := sameNameRegistrationIdentity(child)
		if identity == "" {
			return false
		}
		if _, duplicate := seen[identity]; duplicate {
			return false
		}
		seen[identity] = struct{}{}
	}
	return true
}

func sameNameRegistrationIdentity(token factorytoken.Token) string {
	if token.Color.WorkID != "" {
		return "work:" + token.Color.WorkID
	}
	if token.ID != "" {
		return "token:" + token.ID
	}
	return ""
}

func sameNameTokenIdentity(left, right factorytoken.Token) bool {
	if left.Color.WorkID != "" && right.Color.WorkID != "" {
		return left.Color.WorkID == right.Color.WorkID
	}
	return left.ID != "" && left.ID == right.ID
}

// sameNamePeerCandidates handles the authored Factory shape where SAME_NAME
// is declared on the parent input and names the child input as its peer. The
// child arc is otherwise unguarded, so its stable token ordering would choose
// a historical child before the parent guard is evaluated. When a peer guard
// references this arc, select the child from the ordered parent registration
// projection before the parent candidate is tried.
func (s *singleTokenBindingSearch) sameNamePeerCandidates(arc *petri.Arc, candidates []factorytoken.Token) ([]factorytoken.Token, bool) {
	if s == nil || s.transition == nil || s.snapshot == nil || arc == nil {
		return nil, false
	}
	childBinding := arcKey(arc)
	for index := range s.transition.InputArcs {
		peerArc := &s.transition.InputArcs[index]
		if peerArc == arc {
			continue
		}
		matchBinding, ok := sameNameMatchBinding(peerArc.Guard)
		if !ok || matchBinding != childBinding {
			continue
		}

		parentCandidates := stableTokens(s.snapshot.Marking.TokensInPlace(peerArc.PlaceID))
		if len(parentCandidates) == 0 {
			return nil, true
		}
		matched := make([]factorytoken.Token, 0, len(candidates))
		seen := make(map[string]bool, len(candidates))
		for _, parentCandidate := range parentCandidates {
			parent := parentCandidate
			selected, selectedOK := (&petri.SameNameGuard{MatchBinding: arcKey(peerArc)}).EvaluateRuntime(
				s.runtime,
				candidates,
				map[string]*factorytoken.Token{arcKey(peerArc): &parent},
				&s.snapshot.Marking,
			)
			if !selectedOK {
				continue
			}
			for _, candidate := range selected {
				if seen[candidate.ID] {
					continue
				}
				seen[candidate.ID] = true
				matched = append(matched, candidate)
			}
		}
		return stableTokens(matched), true
	}
	return nil, false
}

func sameNameMatchBinding(guard petri.Guard) (string, bool) {
	switch typed := guard.(type) {
	case *petri.SameNameGuard:
		if typed == nil || typed.MatchBinding == "" {
			return "", false
		}
		return typed.MatchBinding, true
	case *petri.AllGuard:
		if typed == nil {
			return "", false
		}
		for _, nested := range typed.Guards {
			if matchBinding, ok := sameNameMatchBinding(nested); ok {
				return matchBinding, true
			}
		}
	}
	return "", false
}
