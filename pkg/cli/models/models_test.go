package models

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestRenderList_WritesDiscoveredModelsTable(t *testing.T) {
	var out bytes.Buffer
	err := RenderList(factoryapi.ListModelsResponse{
		Results: []factoryapi.ModelSummary{{
			Name:             "OMNIVOICE_Q4_K_M",
			ProviderLocality: factoryapi.WorkerModelLocalityLocal,
			Status:           factoryapi.READY,
			LoadState:        factoryapi.UNLOADED,
			Operations:       []factoryapi.ModelOperation{{Name: "TTS"}},
			Modalities:       []factoryapi.ModelOperationContentType{factoryapi.ModelOperationContentTypeAudio, factoryapi.ModelOperationContentTypeText},
			Resources:        []factoryapi.ModelResourceSummary{{Name: "voice-cache", Type: factoryapi.ResourceTypeModel, Capacity: 1}},
		}},
	}, &out)
	if err != nil {
		t.Fatalf("RenderList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "OMNIVOICE_Q4_K_M", "LOCAL", "READY", "UNLOADED", "TTS", "AUDIO,TEXT"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("rendered table missing %q:\n%s", want, got)
		}
	}
}

func TestQueryModel_NotFoundUsesFriendlyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"model not found","family":"NOT_FOUND","code":"NOT_FOUND"}`)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port
	_, err := QueryModel(port, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}
