package stateaccessquery

// Admission is the query-owned copy of one canonical Work admission fact.
// Order is assigned by the session-scoped canonical history reader.
type Admission struct {
	WorkID string
	Name   string
	Order  int
}

// AnnotateSupersession derives successor references without mutating the
// supplied selection items or introducing a durable lifecycle fact.
func AnnotateSupersession(items []Item, admissions []Admission) []Item {
	annotated := append([]Item(nil), items...)
	if len(annotated) == 0 || len(admissions) == 0 {
		return annotated
	}

	byWorkID := make(map[string]Admission, len(admissions))
	latestByName := make(map[string]Admission, len(admissions))
	for _, admission := range admissions {
		if admission.WorkID == "" || admission.Name == "" {
			continue
		}
		byWorkID[admission.WorkID] = admission
		latest, exists := latestByName[admission.Name]
		if !exists || admission.Order >= latest.Order {
			latestByName[admission.Name] = admission
		}
	}

	for index, item := range annotated {
		if !isSupersedable(item) {
			continue
		}
		admission, found := byWorkID[item.WorkID]
		if !found {
			admission, found = byWorkID[item.ID]
		}
		if !found {
			continue
		}
		successor, found := latestByName[item.Name]
		if found && successor.Order > admission.Order && successor.WorkID != admission.WorkID {
			annotated[index].SupersededBy = successor.WorkID
		}
	}
	return annotated
}

func isSupersedable(item Item) bool {
	return item.State != nil &&
		(item.State.Type == StateTypeTerminal || item.State.Type == StateTypeFailed)
}
