package retiredsurfaceguard

// SettledRetiredCLIPaths are hard-retired CLI surfaces from S11/S12/B08 closeout.
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
