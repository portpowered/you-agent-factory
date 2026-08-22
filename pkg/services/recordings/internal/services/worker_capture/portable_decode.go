package worker_capture

import (
	"encoding/json"
	"reflect"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/jsoncompat"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerPortableRecordingDecodeDiagnostics contains safe metadata produced
// while decoding one portable Worker recording. It records ignored paths only;
// ignored values are never retained for logging or persistence.
type WorkerPortableRecordingDecodeDiagnostics struct {
	IgnoredJSONPaths []string
}

// Paths returns a detached, deterministic copy of ignored JSON paths.
func (diagnostics WorkerPortableRecordingDecodeDiagnostics) Paths() []string {
	return jsoncompat.SortedUniquePaths(diagnostics.IgnoredJSONPaths)
}

// DecodeWorkerPortableRecording decodes and validates exactly one portable
// Worker recording. Unknown object fields are ignored and trailing JSON stays
// rejected before any replay projection is returned.
func (codec WorkerRecordingCodec) DecodeWorkerPortableRecording(payload []byte) (WorkerPortableRecording, error) {
	recording, _, err := codec.DecodeWorkerPortableRecordingWithDiagnostics(payload)
	return recording, err
}

// DecodeWorkerPortableRecordingWithDiagnostics decodes and validates exactly
// one portable Worker recording, returning sorted paths for ignored additive
// fields in the envelope, records, and embedded Worker drafts.
func (codec WorkerRecordingCodec) DecodeWorkerPortableRecordingWithDiagnostics(
	payload []byte,
) (WorkerPortableRecording, WorkerPortableRecordingDecodeDiagnostics, error) {
	var recording WorkerPortableRecording
	diagnostics, err := jsoncompat.Decode(payload, &recording)
	if err != nil {
		return WorkerPortableRecording{}, WorkerPortableRecordingDecodeDiagnostics{}, portableDiagnostic(
			WorkerPortableCodeMalformedContract, "document", "portable recording JSON is malformed", ErrWorkerPortableRecording,
		)
	}
	if err := codec.ValidateWorkerPortableRecording(recording); err != nil {
		return WorkerPortableRecording{}, WorkerPortableRecordingDecodeDiagnostics{}, err
	}
	paths, err := collectWorkerPortableRecordingJSONPathsFromValue(
		recording, diagnostics.Paths(), "$",
	)
	if err != nil {
		return WorkerPortableRecording{}, WorkerPortableRecordingDecodeDiagnostics{}, portableDiagnostic(
			WorkerPortableCodeMalformedContract, "document", "portable Worker recording diagnostics could not be decoded", ErrWorkerPortableRecording,
		)
	}
	return cloneWorkerPortableRecording(recording), WorkerPortableRecordingDecodeDiagnostics{
		IgnoredJSONPaths: paths,
	}, nil
}

func collectWorkerPortableRecordingJSONPathsFromValue(
	recording WorkerPortableRecording,
	paths []string,
	prefix string,
) ([]string, error) {
	paths = jsoncompat.PrefixPaths(prefix, paths)
	for index, record := range recording.Records {
		draftPaths, err := collectWorkerDraftJSONPaths(
			record.Payload,
			prefix+".records["+strconv.Itoa(index)+"].payload",
		)
		if err != nil {
			return nil, err
		}
		paths = append(paths, draftPaths...)
	}
	return jsoncompat.SortedUniquePaths(paths), nil
}

func collectWorkerDraftJSONPaths(payload json.RawMessage, prefix string) ([]string, error) {
	var draft workers.Draft
	diagnostics, err := jsoncompat.Decode(payload, &draft)
	if err != nil {
		return nil, err
	}
	paths := jsoncompat.PrefixPaths(prefix, diagnostics.Paths())
	payloadType := workerDraftPayloadType(draft.Kind, draft.Phase)
	if payloadType == nil {
		return paths, nil
	}
	nested, err := jsoncompat.CollectUnknownJSONPaths(draft.Payload, payloadType)
	if err != nil {
		return nil, err
	}
	paths = append(paths, jsoncompat.PrefixPaths(prefix+".payload", nested)...)
	return jsoncompat.SortedUniquePaths(paths), nil
}

var workerDraftPayloadTypes = map[workers.Kind]reflect.Type{
	workers.KindSession:    reflect.TypeOf(workers.SessionPayload{}),
	workers.KindRun:        reflect.TypeOf(workers.RunPayload{}),
	workers.KindTurn:       reflect.TypeOf(workers.TurnPayload{}),
	workers.KindReasoning:  reflect.TypeOf(workers.ReasoningPayload{}),
	workers.KindFileChange: reflect.TypeOf(workers.FileChangePayload{}),
	workers.KindPlan:       reflect.TypeOf(workers.PlanPayload{}),
	workers.KindProgress:   reflect.TypeOf(workers.ProgressPayload{}),
	workers.KindUsage:      reflect.TypeOf(workers.UsagePayload{}),
	workers.KindError:      reflect.TypeOf(workers.ErrorPayload{}),
	workers.KindStreamGap:  reflect.TypeOf(workers.StreamGapPayload{}),
}

func workerDraftPayloadType(kind workers.Kind, phase workers.Phase) reflect.Type {
	if kind == workers.KindMessage {
		if phase == workers.PhaseDelta {
			return reflect.TypeOf(workers.MessageDeltaPayload{})
		}
		return reflect.TypeOf(workers.MessagePayload{})
	}
	if kind == workers.KindTool {
		if phase == workers.PhaseDelta {
			return reflect.TypeOf(workers.ToolDeltaPayload{})
		}
		return reflect.TypeOf(workers.ToolPayload{})
	}
	return workerDraftPayloadTypes[kind]
}

func decodeWorkerDraftStrict(payload json.RawMessage) (workers.Draft, error) {
	var draft workers.Draft
	if _, err := jsoncompat.Decode(payload, &draft); err != nil {
		return workers.Draft{}, err
	}
	if err := workers.ValidateDraft(draft); err != nil {
		return workers.Draft{}, err
	}
	return draft, nil
}
