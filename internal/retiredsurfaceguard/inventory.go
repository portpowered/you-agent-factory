package retiredsurfaceguard

// SettledRetiredCLIPaths are hard-retired CLI surfaces from completed CLI
// migrations. The list is shared by absence tests so a retired path cannot
// quietly reappear in a generated or handwritten command tree.
func SettledRetiredCLIPaths() []string {
	return append([]string(nil), settledRetiredCLIPaths...)
}

// SettledRetiredDocsTopics are hard-retired packaged docs topics from S15/B08 closeout.
func SettledRetiredDocsTopics() []string {
	return append([]string(nil), settledRetiredDocsTopics...)
}

// SettledScopedNamedFactoryPaths are canonical scoped names whose production
// resolve, persist, and materialize must use hierarchical on-disk layout only.
func SettledScopedNamedFactoryPaths() []string {
	return append([]string(nil), settledScopedNamedFactoryPaths...)
}

var settledRetiredCLIPaths = []string{
	"you config validate",
	"you config flatten",
	"you config expand",
	"you factory save",
	"you factory validate",
	"you factory query",
	"you work visualize",
	"you session dispatches",
	"you serve acp",
	"you mcp serve",
}

var settledRetiredDocsTopics = []string{
	"packaged-fusion",
	"packaged-goal",
	"packaged-tts",
	"mcp-hosts",
}

var settledScopedNamedFactoryPaths = []string{
	"@you/goal",
	"@you/tts",
}
