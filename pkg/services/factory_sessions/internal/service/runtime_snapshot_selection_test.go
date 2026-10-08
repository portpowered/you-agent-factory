package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestRuntimeSnapshotSelectionLiveDetachesSelectedFacts(t *testing.T) {
	t.Parallel()
	selectedDir := t.TempDir()
	args := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{"model": {Values: []string{"selected"}}}}
	original := activationSnapshot()
	original.Invocation.Arguments = args
	original.PromptSources = []factorydefinitions.RuntimePromptSource{{Name: "selected"}}
	original.InvocationSensitiveJSONPointers = []string{"/selected"}
	var requests []factorydefinitions.ResolveRuntimeSnapshotRequest
	calls := 0
	selector := NewRuntimeSnapshotSelection(func(_ context.Context, request factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
		calls++
		requests = append(requests, request)
		original.Invocation.FactorySessionID = request.Invocation.FactorySessionID
		return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: original}, nil
	}, nil, nil, func(string) (string, error) { return selectedDir, nil }, nil)
	if calls != 0 {
		t.Fatal("constructor executed selection")
	}
	definition := factorydefinitions.RuntimeSelection{Directory: "current", ExecutionBaseDir: "/selected-base", InvocationArguments: args}
	first, err := selector.Resolve(t.Context(), definition, recordings.RuntimeSelection{WorkflowID: "workflow-a"}, nil, nil, "session-a")
	if err != nil {
		t.Fatal(err)
	}
	args.Arguments["model"] = work.InvocationArgument{Values: []string{"changed"}}
	original.PromptSources[0].Name = "changed"
	original.InvocationSensitiveJSONPointers[0] = "/changed"
	original.DefinitionVersion.Logical = 9
	second, err := selector.Resolve(t.Context(), definition, recordings.RuntimeSelection{}, nil, nil, "session-b")
	if err != nil {
		t.Fatal(err)
	}
	request := requests[0]
	assertLiveSnapshotSelectionRequest(t, first, request, selectedDir)
	if request.Invocation.Arguments.Arguments["model"].Values[0] != "selected" || first.snapshot.Invocation.Arguments.Arguments["model"].Values[0] != "selected" {
		t.Fatal("arguments aliased caller")
	}
	if first.snapshot.PromptSources[0].Name != "selected" || first.snapshot.InvocationSensitiveJSONPointers[0] != "/selected" || first.snapshot.DefinitionVersion.Logical != 1 || first.snapshot.Invocation.FactorySessionID != "session-a" || second.snapshot.Invocation.FactorySessionID != "session-b" {
		t.Fatalf("selected snapshot mutated: %#v", first.snapshot)
	}
}

func assertLiveSnapshotSelectionRequest(t *testing.T, result activationSnapshotResolution, request factorydefinitions.ResolveRuntimeSnapshotRequest, selectedDir string) {
	t.Helper()
	if result.factoryDir != selectedDir || result.sourcePath != filepath.Join(selectedDir, "factory.json") || result.runtimeBaseDir != "/selected-base" {
		t.Fatalf("selected paths = %#v", result)
	}
	if request.FactoryDir != "" || request.SourcePath != result.sourcePath || request.ExecutionBaseDir != result.runtimeBaseDir || request.Invocation.FactorySessionID != "session-a" || request.Invocation.WorkflowID != "workflow-a" {
		t.Fatalf("selection request = %#v", request)
	}
}

func TestRuntimeSnapshotSelectionRecordedInputsAreNotReread(t *testing.T) {
	t.Parallel()
	for _, intent := range []string{"replay", "resume"} {
		t.Run(intent, func(t *testing.T) {
			t.Parallel()
			canonical := factorydefinitions.FactorySnapshot(`{"name":"recorded"}`)
			input := recordings.LoadReplayInputResult{Legacy: &factorydefinitions.ReplayArtifact{Factory: &canonical}}
			loads := &snapshotSelectionReplayLoader{err: errors.New("must not reread")}
			var request factorydefinitions.ResolveRuntimeSnapshotRequest
			original := activationSnapshot()
			original.FactoryDir = ""
			original.RuntimeBaseDir = ""
			selector := NewRuntimeSnapshotSelection(func(_ context.Context, got factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
				request = got
				return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: original}, nil
			}, func(got *factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
				if got != &canonical {
					t.Fatal("decoder lost captured input")
				}
				return replayRuntimeConfigStub{factoryDir: "/recorded", runtimeBaseDir: "/recorded-base"}, nil
			}, loads, nil, nil)
			var replay *recordings.LoadReplayInputResult
			var resume *recordings.LoadResumeInputResult
			recording := recordings.RuntimeSelection{WorkflowID: "recorded-workflow"}
			if intent == "replay" {
				replay = &input
				recording.ReplayPath = "recording.json"
			} else {
				resume = &recordings.LoadResumeInputResult{Input: input}
			}
			result, err := selector.Resolve(t.Context(), factorydefinitions.RuntimeSelection{SourcePath: "/different", ExecutionBaseDir: "/override"}, recording, replay, resume, "recorded-session")
			if err != nil {
				t.Fatal(err)
			}
			assertRecordedSnapshotSelectionRequest(t, request, canonical, loads.calls)
			canonical = factorydefinitions.FactorySnapshot("changed")
			original.DefinitionVersion.Logical = 10
			if string(request.Canonical) != `{"name":"recorded"}` || result.snapshot.DefinitionVersion.Logical != 1 || result.factoryDir != "/recorded" || result.runtimeBaseDir != "/override" || result.snapshot.RuntimeBaseDir != "/override" {
				t.Fatalf("recorded facts mutated: %#v", result)
			}
		})
	}
}

func assertRecordedSnapshotSelectionRequest(t *testing.T, request factorydefinitions.ResolveRuntimeSnapshotRequest, canonical factorydefinitions.FactorySnapshot, loads int) {
	t.Helper()
	if loads != 0 || string(request.Canonical) != string(canonical) || request.SourcePath != "" || request.ExecutionBaseDir != "/override" || request.Invocation.FactorySessionID != "recorded-session" || request.Invocation.WorkflowID != "recorded-workflow" {
		t.Fatalf("recorded request = %#v; loads=%d", request, loads)
	}
}

type snapshotSelectionReplayLoader struct {
	calls int
	err   error
}

func (loader *snapshotSelectionReplayLoader) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	loader.calls++
	return recordings.LoadReplayInputResult{}, loader.err
}

func TestRuntimeSnapshotSelectionFailuresPreserveCauseAndZeroResult(t *testing.T) {
	t.Parallel()
	cause := errors.New("controlled failure")
	for _, effect := range []string{"source", "path", "resolver", "decoder", "replay"} {
		t.Run(effect, func(t *testing.T) {
			t.Parallel()
			resolve := factorydefinitions.RuntimeSnapshotOperation(func(context.Context, factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
				return factorydefinitions.ResolveRuntimeSnapshotResult{}, cause
			})
			decode := factorydefinitions.ReplayRuntimeConfigDecoder(func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
				return nil, cause
			})
			selector := NewRuntimeSnapshotSelection(resolve, decode, &snapshotSelectionReplayLoader{err: cause}, nil, nil)
			definition := factorydefinitions.RuntimeSelection{Directory: "/factory"}
			recording := recordings.RuntimeSelection{}
			var input *recordings.LoadReplayInputResult
			switch effect {
			case "source":
				definition.SourcePath = "~/factory.json"
				selector.resolveHome = func() (string, error) { return "", cause }
			case "path":
				selector.resolveCurrentDir = func(string) (string, error) { return "", cause }
			case "decoder":
				canonical := factorydefinitions.FactorySnapshot(`{}`)
				input = &recordings.LoadReplayInputResult{Legacy: &factorydefinitions.ReplayArtifact{Factory: &canonical}}
				recording.ReplayPath = "recording.json"
			case "replay":
				recording.ReplayPath = "recording.json"
			}
			result, err := selector.Resolve(t.Context(), definition, recording, input, nil, "session")
			if !errors.Is(err, cause) || !reflect.DeepEqual(result, activationSnapshotResolution{}) {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
}

func TestRuntimeSnapshotSelectionPortableFallbackAndUnsupportedResume(t *testing.T) {
	t.Parallel()
	var selected factorydefinitions.ResolveRuntimeSnapshotRequest
	canonical := factorydefinitions.FactorySnapshot(`{}`)
	input := recordings.LoadReplayInputResult{Portable: &recordings.PortableRecording{}, Legacy: &factorydefinitions.ReplayArtifact{Factory: &canonical}}
	selector := NewRuntimeSnapshotSelection(func(_ context.Context, request factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
		selected = request
		return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: activationSnapshot()}, nil
	}, func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
		t.Fatal("portable input decoded as legacy")
		return nil, nil
	}, nil, nil, nil)
	source := filepath.Join(t.TempDir(), "factory.json")
	result, err := selector.Resolve(t.Context(), factorydefinitions.RuntimeSelection{SourcePath: source}, recordings.RuntimeSelection{ReplayPath: "portable.json"}, &input, nil, "portable")
	if err != nil || result.sourcePath != source || selected.SourcePath != source || selected.Canonical != nil {
		t.Fatalf("portable fallback: %#v %#v %v", result, selected, err)
	}
	result, err = selector.Resolve(t.Context(), factorydefinitions.RuntimeSelection{}, recordings.RuntimeSelection{}, nil, &recordings.LoadResumeInputResult{Input: input}, "portable")
	if err == nil || !reflect.DeepEqual(result, activationSnapshotResolution{}) {
		t.Fatalf("unsupported resume: %#v %v", result, err)
	}
}
