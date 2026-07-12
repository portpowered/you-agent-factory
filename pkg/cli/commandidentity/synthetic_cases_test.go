package commandidentity_test

type syntheticCommandCase struct {
	path              string
	idCandidate       string
	name              string
	aliases           []string
	groupID           string
	short             string
	long              string
	example           string
	visibility        string
	lifecycle         string
	deprecatedMessage string
	runnable          bool
	docIDCandidate    string
	handlerPresent    bool
}

func syntheticCommandCases() []syntheticCommandCase {
	return []syntheticCommandCase{
		{
			path:           "synth",
			idCandidate:    "synth",
			name:           "synth",
			aliases:        []string{},
			short:          "root short",
			long:           "root long",
			example:        "",
			visibility:     "visible",
			lifecycle:      "active",
			runnable:       true,
			handlerPresent: true,
		},
		{
			path:           "synth nested",
			idCandidate:    "synth.nested",
			name:           "nested",
			aliases:        []string{},
			short:          "nested short",
			visibility:     "visible",
			lifecycle:      "active",
			runnable:       false,
			handlerPresent: false,
		},
		{
			path:           "synth nested hidden-leaf",
			idCandidate:    "synth.nested.hidden-leaf",
			name:           "hidden-leaf",
			aliases:        []string{},
			short:          "hidden short",
			visibility:     "hidden",
			lifecycle:      "active",
			runnable:       true,
			handlerPresent: true,
		},
		{
			path:           "synth aliased",
			idCandidate:    "synth.aliased",
			name:           "aliased",
			aliases:        []string{"alias-one", "alias-two"},
			groupID:        "operations",
			short:          "aliased short",
			visibility:     "visible",
			lifecycle:      "active",
			runnable:       true,
			handlerPresent: true,
		},
		{
			path:              "synth deprecated",
			idCandidate:       "synth.deprecated",
			name:              "deprecated",
			aliases:           []string{},
			short:             "deprecated short",
			visibility:        "visible",
			lifecycle:         "deprecated",
			deprecatedMessage: "use synth nested instead",
			runnable:          false,
			handlerPresent:    false,
		},
		{
			path:           "synth docs",
			idCandidate:    "synth.docs",
			name:           "docs",
			aliases:        []string{},
			short:          "docs short",
			visibility:     "visible",
			lifecycle:      "active",
			runnable:       false,
			handlerPresent: false,
		},
		{
			path:           "synth docs agents",
			idCandidate:    "synth.docs.agents",
			name:           "agents",
			aliases:        []string{},
			short:          "agents short",
			visibility:     "visible",
			lifecycle:      "active",
			docIDCandidate: "agents",
			runnable:       true,
			handlerPresent: true,
		},
	}
}
