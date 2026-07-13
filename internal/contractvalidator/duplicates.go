package contractvalidator

import (
	"fmt"
	"sort"
	"strconv"
)

const stableIDField = "id"

type loadedDocument struct {
	path  string
	value any
}

type stableIDOccurrence struct {
	document string
	path     string
}

// duplicateStableIDDiagnostics checks the family-neutral stable ID field from
// the common documentation shape. Item IDs can legitimately repeat when
// documentation and lifecycle components describe the same contract item.
func duplicateStableIDDiagnostics(documents []loadedDocument) []Diagnostic {
	occurrences := make(map[string][]stableIDOccurrence)
	for _, document := range documents {
		collectStableIDs(document.value, nil, document.path, occurrences)
	}

	ids := make([]string, 0, len(occurrences))
	for id, locations := range occurrences {
		if len(locations) > 1 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var diagnostics []Diagnostic
	for _, id := range ids {
		message := fmt.Sprintf("stable ID %s appears more than once", strconv.Quote(id))
		for _, location := range occurrences[id] {
			diagnostics = append(diagnostics, newDiagnostic("identity.duplicate", location.path, message, location.document))
		}
	}
	return diagnostics
}

func collectStableIDs(value any, segments []string, document string, occurrences map[string][]stableIDOccurrence) {
	switch typed := value.(type) {
	case map[string]any:
		id, hasID := typed[stableIDField].(string)
		_, hasCanonicalEnglish := typed["canonicalEnglish"].(string)
		if hasID && hasCanonicalEnglish {
			occurrences[id] = append(occurrences[id], stableIDOccurrence{
				document: normalizeRepositoryPath(document),
				path:     instancePath(appendPath(segments, stableIDField)),
			})
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectStableIDs(typed[key], appendPath(segments, key), document, occurrences)
		}
	case []any:
		for index, child := range typed {
			collectStableIDs(child, appendPath(segments, strconv.Itoa(index)), document, occurrences)
		}
	}
}
