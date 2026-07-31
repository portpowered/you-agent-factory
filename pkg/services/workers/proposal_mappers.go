package workers

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ProposedOutputFromLegacyWorkResult maps a legacy WorkResult onto ProposedOutput
// without carrying Runtime token identity. Canonical FactoryWorkItem IDs from
// RecordedOutputWork are intentionally dropped; Work assigns identity later.
func ProposedOutputFromLegacyWorkResult(result WorkResult) ProposedOutput {
	output := ProposedOutput{
		Feedback:       result.Feedback,
		Classification: result.SelectedClassificationLabel,
		ProposedWork:   ProposedWorkFromFactoryWorkItems(result.RecordedOutputWork),
	}
	if text := strings.TrimSpace(result.Output); text != "" {
		output.Primary = []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: text,
		}}
	}
	return output
}

// ProposedWorkFromFactoryWorkItems converts legacy canonical items into
// non-canonical proposals. Work IDs, place IDs, and chaining identity fields are
// not copied onto the proposal.
func ProposedWorkFromFactoryWorkItems(items []work.FactoryWorkItem) []ProposedWork {
	if len(items) == 0 {
		return nil
	}
	proposed := make([]ProposedWork, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = strings.TrimSpace(item.ID)
		}
		proposed = append(proposed, ProposedWork{
			WorkTypeID: item.WorkTypeID,
			Name:       name,
			State:      item.State,
			Content:    work.CloneWorkContentParts(item.Content),
			Tags:       cloneProposalTagsWithoutIdentity(item.Tags),
		})
	}
	return proposed
}

// WorkProposedItemsFromProposedWork projects Workers proposals onto Work-owned
// proposal values for materialization.
func WorkProposedItemsFromProposedWork(items []ProposedWork) []work.ProposedWorkItem {
	if len(items) == 0 {
		return nil
	}
	proposed := make([]work.ProposedWorkItem, len(items))
	for i, item := range items {
		proposed[i] = work.ProposedWorkItem{
			WorkTypeID: item.WorkTypeID,
			Name:       item.Name,
			State:      item.State,
			Content:    work.CloneWorkContentParts(item.Content),
			Tags:       cloneStringMap(item.Tags),
			Relations:  append([]work.Relation(nil), item.Relations...),
		}
	}
	return proposed
}

func cloneProposalTagsWithoutIdentity(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	clone := make(map[string]string, len(tags))
	for key, value := range tags {
		switch strings.TrimSpace(key) {
		case "id", "workId", "work_id", "placeId", "place_id":
			continue
		default:
			clone[key] = value
		}
	}
	if len(clone) == 0 {
		return nil
	}
	return clone
}
