package localai

import (
	"context"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"google.golang.org/protobuf/proto"
)

func TestPinnedGRPCHostProtocolNegotiatorUsesSelectedArchiveAccelerator(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name, accelerator string
		gpuLayers         int32
	}{
		{name: "CPU archive on CUDA host", accelerator: "cpu", gpuLayers: 0},
		{name: "CUDA archive on CUDA host", accelerator: "cuda", gpuLayers: 99},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			connection := &recordingGRPCConnection{}
			connection.response, _ = proto.Marshal(&Result{Success: true})
			negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
			_, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
				Configuration: modelseffects.ResolvedHostConfiguration{
					ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
					Backend:         "localai-llamacpp", ModelName: models.BuiltInModelNameEmbed,
					ModelPath:       "/models/embed.gguf",
					Platform:        models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true},
					BackendArtifact: modelseffects.BackendArtifactSelection{Accelerator: testCase.accelerator},
				},
			})
			if err != nil {
				t.Fatalf("negotiate %s: %v", testCase.name, err)
			}
			if got := connection.loadRequest.GetNGPULayers(); got != testCase.gpuLayers {
				t.Fatalf("GPU layers = %d, want %d", got, testCase.gpuLayers)
			}
		})
	}
}
