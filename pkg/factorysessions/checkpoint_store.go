package factorysessions

import jsstore "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store"

// JavaScriptCheckpointStore keeps orchestrator-owned checkpoint bundles for one
// live JavaScript workflow session.
//
// Deprecated: use pkg/orchestrators/javascript/store.CheckpointStore directly.
type JavaScriptCheckpointStore = jsstore.CheckpointStore

// NewJavaScriptCheckpointStore allocates an empty checkpoint store.
func NewJavaScriptCheckpointStore() *JavaScriptCheckpointStore {
	return jsstore.NewCheckpointStore()
}
