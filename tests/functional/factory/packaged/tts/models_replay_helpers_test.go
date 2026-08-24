package tts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func managedTTSModelEdges(
	t *testing.T,
	backend *packagedTTSModelsBackend,
) (serviceedges.Edges, func()) {
	t.Helper()
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	edges := serviceedges.Edges{
		ModelAssetHostPlatform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return packagedTTSPinnedBackendSelection(), nil
		},
		ModelHostProcessLauncher:      &packagedTTSModelHostLauncher{endpoint: modelServer.URL},
		ModelHostProtocolNegotiator:   packagedTTSHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: packagedTTSHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationBackend:        backend.Invoke,
	}
	return edges, modelServer.Close
}

func assertFactoryTTSWorkEquivalent(
	t *testing.T,
	live, replay factoryapi.Work,
	label string,
) {
	t.Helper()
	if live.WorkTypeName == nil || replay.WorkTypeName == nil ||
		*live.WorkTypeName != *replay.WorkTypeName ||
		live.State == nil || replay.State == nil ||
		live.State.Name != replay.State.Name ||
		live.Content == nil || replay.Content == nil ||
		len(*live.Content) != len(*replay.Content) {
		t.Fatalf("%s shape = live:%#v replay:%#v", label, live, replay)
	}
	for index := range *live.Content {
		liveJSON, err := json.Marshal((*live.Content)[index])
		if err != nil {
			t.Fatalf("marshal live Work content[%d]: %v", index, err)
		}
		replayJSON, err := json.Marshal((*replay.Content)[index])
		if err != nil {
			t.Fatalf("marshal replay Work content[%d]: %v", index, err)
		}
		if string(liveJSON) != string(replayJSON) {
			t.Fatalf("%s content[%d] differs\nlive=%s\nreplay=%s", label, index, liveJSON, replayJSON)
		}
	}
}

func assertFactoryTTSEventProjectionEquivalent(
	t *testing.T,
	live, replay []factoryapi.FactoryEvent,
	label string,
) {
	t.Helper()
	if len(live) != len(replay) {
		t.Fatalf("%s event count = live:%d replay:%d\nlive=%#v\nreplay=%#v", label, len(live), len(replay), live, replay)
	}
	for index := range live {
		if live[index].Type != replay[index].Type {
			t.Fatalf("%s event[%d] type = live:%q replay:%q", label, index, live[index].Type, replay[index].Type)
		}
		liveWorkIDs := []string(nil)
		if live[index].Context.WorkIds != nil {
			liveWorkIDs = *live[index].Context.WorkIds
		}
		replayWorkIDs := []string(nil)
		if replay[index].Context.WorkIds != nil {
			replayWorkIDs = *replay[index].Context.WorkIds
		}
		if strings.Join(liveWorkIDs, ",") != strings.Join(replayWorkIDs, ",") {
			t.Fatalf("%s event[%d] work ids = live:%#v replay:%#v", label, index, live[index].Context.WorkIds, replay[index].Context.WorkIds)
		}
	}
}
