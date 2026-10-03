package wire_test

import (
	"errors"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

const (
	boundedRecordingBodyBytes = 64 << 20
	boundedMetadataReadBudget = 1 << 20
	boundedFillerEvent        = `{"id":"event","payload":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},`
)

// syntheticRecording streams prefix, then filler up to bodyBytes, then suffix,
// counting every byte a caller pulls so tests can prove summary reads are
// bounded without depending on wall-clock time.
type syntheticRecording struct {
	prefix, filler, suffix string
	bodyBytes              int
	stage                  int
	offset                 int
	emitted                int
	counter                *int
}

func (r *syntheticRecording) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) && r.stage < 3 {
		var src string
		switch r.stage {
		case 0:
			src = r.prefix
		case 1:
			src = r.filler
		default:
			src = r.suffix
		}
		copied := copy(p[n:], src[r.offset:])
		n += copied
		r.offset += copied
		if r.stage == 1 {
			r.emitted += copied
		}
		if r.offset >= len(src) {
			r.offset = 0
			if r.stage == 1 && r.emitted < r.bodyBytes {
				continue
			}
			r.stage++
		}
	}
	*r.counter += n
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (r *syntheticRecording) Close() error { return nil }

func TestReplayInputLoaderMetadataModeReadsBoundedBytesOfLargeRecordings(t *testing.T) {
	t.Parallel()

	const sessionID = "00000000-0000-4000-8000-0000000000aa"
	tests := map[string]struct {
		prefix, suffix string
	}{
		"legacy single-line": {
			prefix: `{"schemaVersion":"replay.v1","recordedAt":"2026-08-24T00:00:00Z","events":[{"context":{"sessionId":"` + sessionID + `"},"id":"first"},`,
			suffix: `{"id":"last"}]}`,
		},
		"portable": {
			prefix: `{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"2","replayCompatibilityVersion":"1","session":{"id":"` + sessionID + `"},"events":[`,
			suffix: `{"id":"last"}]}`,
		},
		"v2 header then huge line": {
			prefix: `{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-08-24T00:00:00Z","sessionId":"` + sessionID + `","factoryIdentity":{"id":"factory","name":"factory","factoryDirectory":"factory","sourceDirectory":"factory"},"hashes":{"factory_hash":"sha256:factory","workers_hash":"sha256:workers","workstations_hash":"sha256:workstations","runtime_config_hash":"sha256:runtime"}}` + "\n",
			suffix: "\n",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			counter := new(int)
			loader := recordingswire.NewReplayInputLoader(
				func(string) ([]byte, error) {
					return nil, errors.New("metadata mode must not use the full replay reader")
				},
				func(string) (*recordings.ReplayArtifact, error) {
					return nil, errors.New("metadata mode must not use the full legacy loader")
				},
				logging.NoopLogger{},
				func(string) (io.ReadCloser, error) {
					return &syntheticRecording{
						prefix: test.prefix, filler: boundedFillerEvent, suffix: test.suffix,
						bodyBytes: boundedRecordingBodyBytes, counter: counter,
					}, nil
				},
			)
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "2026/08/24/x.json", MetadataOnly: true})
			if err != nil {
				t.Fatalf("LoadReplayInput(metadata) error = %v", err)
			}
			if result.Metadata == nil || result.Metadata.FactorySessionID != sessionID {
				t.Fatalf("metadata = %#v, want session %q", result.Metadata, sessionID)
			}
			if *counter > boundedMetadataReadBudget {
				t.Fatalf("metadata read consumed %d bytes of a %d byte body, want at most %d", *counter, boundedRecordingBodyBytes, boundedMetadataReadBudget)
			}
		})
	}
}
