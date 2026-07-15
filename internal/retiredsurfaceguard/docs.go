package retiredsurfaceguard

// DocsTopicEntry describes one canonical packaged docs topic plus aliases.
type DocsTopicEntry struct {
	Name    string
	Aliases []string
}

// DocsRegistry is the packaged docs topic registry snapshot scanned by guards.
type DocsRegistry struct {
	SupportedTopics   []string
	SupportedCommands []string
	IndexEntries      []DocsTopicEntry
}

// ScanDocsReintroductionViolations reports retired docs topics accepted by the
// registry directly or through compatibility aliases.
func ScanDocsReintroductionViolations(registry DocsRegistry) []Violation {
	retired := retiredDocsTopicSet()
	var violations []Violation

	supported := make(map[string]struct{}, len(registry.SupportedTopics))
	for _, topic := range registry.SupportedTopics {
		supported[topic] = struct{}{}
	}
	commands := make(map[string]struct{}, len(registry.SupportedCommands))
	for _, command := range registry.SupportedCommands {
		commands[command] = struct{}{}
	}

	for topic := range retired {
		if _, stillSupported := supported[topic]; stillSupported {
			violations = append(violations, Violation{
				Family:  "docs",
				Surface: topic,
				Detail:  "retired docs topic is still listed in SupportedTopics()",
			})
		}
		if _, stillAccepted := commands[topic]; stillAccepted {
			violations = append(violations, Violation{
				Family:  "docs",
				Surface: topic,
				Detail:  "retired docs topic is still accepted by SupportedTopicCommands()",
			})
		}
	}

	for _, entry := range registry.IndexEntries {
		if _, isRetired := retired[entry.Name]; isRetired {
			violations = append(violations, Violation{
				Family:  "docs",
				Surface: entry.Name,
				Detail:  "retired docs topic is registered as a canonical docs topic",
			})
		}
		for _, alias := range entry.Aliases {
			if _, isRetired := retired[alias]; isRetired {
				violations = append(violations, Violation{
					Family:  "docs",
					Surface: alias,
					Detail:  "compatibility alias on topic " + entry.Name + " reintroduces retired docs topic",
				})
			}
		}
	}

	return violations
}

func retiredDocsTopicSet() map[string]struct{} {
	retired := make(map[string]struct{}, len(settledRetiredDocsTopics))
	for _, topic := range settledRetiredDocsTopics {
		retired[topic] = struct{}{}
	}
	return retired
}
