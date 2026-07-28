package wire_test

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
)

func TestNewJavaScriptCheckpointStoreConstructsWorkingAdapter(t *testing.T) {
	t.Parallel()

	var store factoryruntime.JavaScriptCheckpointStore = factoryruntimewire.NewJavaScriptCheckpointStore()
	if store == nil {
		t.Fatal("NewJavaScriptCheckpointStore() = nil, want JavaScript checkpoint store")
	}
}

func TestNewJavaScriptCheckpointSummariesConstructsWorkingProjector(t *testing.T) {
	t.Parallel()

	summaries := factoryruntimewire.NewJavaScriptCheckpointSummaries()
	if summaries == nil {
		t.Fatal("NewJavaScriptCheckpointSummaries() = nil, want JavaScript checkpoint summary projector")
	}
}
