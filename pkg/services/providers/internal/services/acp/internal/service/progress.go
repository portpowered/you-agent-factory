package service

import (
	"sort"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// completedPromptProgress normalizes one session/prompt turn's raw
// SessionUpdate-derived progress facts into the established started/
// completed shape every other Providers execution kind already reports,
// synthesizing a start marker for the first message/reasoning chunk and a
// closing completed marker for any item left implicitly finished by the
// turn's own completion.
func completedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	result := []providers.ExecuteProgress{{Phase: "started", Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"}}}
	started := map[string]bool{}
	items := map[string]map[string]bool{}
	content := map[string]map[string]string{}
	for _, update := range updates {
		kind, itemID := update.Metadata["kind"], update.Metadata["item_id"]
		if (kind == "message" || kind == "reasoning") && !started[kind] {
			result = append(result, providers.ExecuteProgress{Phase: "started", Metadata: map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "acp/synthetic_start",
			}})
			started[kind] = true
		}
		if kind != "" && itemID != "" {
			if items[kind] == nil {
				items[kind] = map[string]bool{}
			}
			if content[kind] == nil {
				content[kind] = map[string]string{}
			}
			items[kind][itemID] = update.Phase == "completed"
			content[kind][itemID] += update.Detail
		}
		result = append(result, update)
	}
	for _, kind := range []string{"reasoning", "tool", "session", "message"} {
		ids := make([]string, 0, len(items[kind]))
		for itemID := range items[kind] {
			ids = append(ids, itemID)
		}
		sort.Strings(ids)
		for _, itemID := range ids {
			completed := items[kind][itemID]
			if completed {
				continue
			}
			result = append(result, providers.ExecuteProgress{Phase: "completed", Detail: content[kind][itemID], Metadata: map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "session/prompt",
			}})
		}
	}
	return append(result, providers.ExecuteProgress{Phase: "completed", Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"}})
}

// failedPromptProgress reuses completedPromptProgress's shape for a turn
// that did not reach its own completed marker, flagging any still-open
// message content as partial instead of claiming it finished normally.
func failedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	progress := completedPromptProgress(updates)
	if len(progress) > 0 {
		progress = progress[:len(progress)-1]
	}
	for index := range progress {
		if progress[index].Phase != "completed" || progress[index].Metadata["kind"] != "message" {
			continue
		}
		progress[index].Metadata["partial"] = "true"
	}
	return progress
}
