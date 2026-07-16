package baseline

import (
	"strings"

	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
)

// SerializeDocsTopicIndex records the production packaged docs topic index in
// a deterministic textual form. Each line is
// "<name>\t<description>" or "<name>\t<description>\t<aliases>" when
// aliases are present. Lines follow the production display order from
// TopicIndexEntries.
func SerializeDocsTopicIndex() string {
	entries := docscli.TopicIndexEntries()
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, formatDocsTopicLine(entry))
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatDocsTopicLine(entry docscli.TopicIndexEntry) string {
	description := strings.ReplaceAll(entry.Description, "\t", " ")
	description = strings.ReplaceAll(description, "\n", " ")
	if len(entry.Aliases) == 0 {
		return entry.Name + "\t" + description
	}
	return entry.Name + "\t" + description + "\t" + strings.Join(entry.Aliases, ",")
}
