package named_invocation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One immutable process serves success, preparation failure, and stage failures.
// Cases are sequential to prove a failed invocation does not poison process reuse.
// Python, Models, and ffmpeg execution belong to the compiled integration witness;
// this public Factory proof controls only the script subprocess effect.
func TestPackagedDubVideoRunsFourStagesAndRejectsFailures(t *testing.T) {
	t.Parallel()
	home, working := t.TempDir(), t.TempDir()
	factoryDirectory := support.InstallPackagedFactory(t, home, "@you/dub-video")
	runner := &dubVideoScriptEffect{}
	process := support.BuildProcess(t, serviceedges.Edges{ScriptCommandRunner: runner, ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	for _, scenario := range []struct {
		name, failStage, preserveNames string
		missingVideo                   bool
	}{
		{name: "success"}, {name: "preserve names", preserveNames: "Silver Clouds\nAlice Smith"},
		{name: "missing video", missingVideo: true},
		{name: "invalid translation", failStage: "translate"},
		{name: "reference TTS failure", failStage: "synthesize"}, {name: "reuse after failures"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			video, output := filepath.Join(working, "source video.mp4"), filepath.Join(directory, "dubbed video.mp4")
			manifest := filepath.Join(directory, "saved stage manifest.json")
			want := map[string]string{"video": output, "asr": filepath.Join(directory, "asr.json"),
				"translations": filepath.Join(directory, "translations.json"), "srt": filepath.Join(directory, "subtitles.srt"),
				"ass": filepath.Join(directory, "subtitles.ass"), "language": "zh-CN"}
			runner.reset(manifest, want, scenario.failStage)
			args := []string{"you", "--json", "run", "--named", "@you/dub-video", "--no-record",
				"--output", "primary", "--language", "zh-CN", "--output-video", output}
			if !scenario.missingVideo {
				args = append(args, "--video", video)
			}
			if scenario.preserveNames != "" {
				args = append(args, "--preserve-names", scenario.preserveNames)
			}
			inputs := support.FakeInputs(t.Context(), args)
			inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.WorkingDirectory = working
			err := process.Execute(inputs.Input)
			requests := runner.snapshot()
			if scenario.missingVideo {
				if err == nil || len(requests) != 0 || !strings.Contains(inputs.Stderr(), "video") {
					t.Fatalf("missing video error=%v dispatches=%d stderr=%q", err, len(requests), inputs.Stderr())
				}
				return
			}
			if scenario.failStage != "" {
				assertDubVideoStageFailure(t, err, inputs.Stdout(), inputs.Stderr(), requests, scenario.failStage, output)
				return
			}
			if err != nil {
				t.Fatalf("dub command: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
			if response.Status != factoryapi.InvocationTerminalStatusCompleted || response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
				t.Fatalf("dub response=%#v, want completed primary result", response)
			}
			body, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]string
			if err := json.Unmarshal([]byte(body.Text), &result); err != nil || !reflect.DeepEqual(result, want) {
				t.Fatalf("dub result=%#v error=%v, want %#v", result, err, want)
			}
			assertDubVideoStageArguments(t, requests, factoryDirectory, working, video, output, manifest, scenario.preserveNames)
		})
	}
}

func assertDubVideoStageArguments(t *testing.T, requests []platformprocess.CommandRequest, factoryDirectory, working, video, output, manifest, preserveNames string) {
	t.Helper()
	if len(requests) != 4 {
		t.Fatalf("script stages=%d, want four", len(requests))
	}
	script := filepath.Join(factoryDirectory, "scripts", "dub_video.py")
	for index, stage := range []string{"transcribe", "translate", "synthesize", "render"} {
		want := []string{script, stage, "--manifest", manifest}
		if index == 0 {
			want = []string{script, stage, "--video", video, "--language", "zh-CN", "--output", output,
				"--asr-model", "asr", "--llm-model", "llm", "--tts-model", "qwen3-tts-base", "--tts-server", "", "--subtitles", "", "--preserve-names", preserveNames}
		}
		request := requests[index]
		if request.Command != "python" || request.WorkDir != working || !reflect.DeepEqual(request.Args, want) {
			t.Fatalf("%s script request command=%q dir=%q args=%#v, want python dir=%q args=%#v", stage,
				request.Command, request.WorkDir, request.Args, working, want)
		}
	}
}

func assertDubVideoStageFailure(t *testing.T, err error, stdout, stderr string, requests []platformprocess.CommandRequest, stage, output string) {
	t.Helper()
	wantCalls := 2
	if stage == "synthesize" {
		wantCalls = 3
	}
	if err == nil || len(requests) != wantCalls {
		t.Fatalf("%s failure error=%v dispatches=%d stdout=%q stderr=%q", stage, err, len(requests), stdout, stderr)
	}
	if strings.Contains(stdout, `"status":"COMPLETED"`) || strings.Contains(stdout, `"video":`) {
		t.Fatalf("failed %s claimed successful video: %q", stage, stdout)
	}
	var diagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &diagnostic); decodeErr != nil ||
		diagnostic.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") ||
		!strings.Contains(diagnostic.Message, "video-dub:failed") || !strings.Contains(diagnostic.Message, "workId=") {
		t.Fatalf("failed %s diagnostic=%q decode=%v, want failed Work identity and runtime failure code", stage, stderr, decodeErr)
	}
	t.Logf("%s failure returned no successful video: %s", stage, strings.TrimSpace(stderr))
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("failed %s published output: %v", stage, statErr)
	}
}

type dubVideoScriptEffect struct {
	mu                  sync.Mutex
	requests            []platformprocess.CommandRequest
	manifest, failStage string
	result              map[string]string
}

func (runner *dubVideoScriptEffect) reset(manifest string, result map[string]string, failStage string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.requests, runner.manifest, runner.result, runner.failStage = nil, manifest, result, failStage
}

func (runner *dubVideoScriptEffect) snapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), runner.requests...)
}

func (runner *dubVideoScriptEffect) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.requests = append(runner.requests, request)
	if request.Command != "python" || len(request.Args) < 2 {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider/script command %q", request.Command)
	}
	stage := request.Args[1]
	if stage == runner.failStage {
		return platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("Dubbing " + stage + " failed: controlled invalid translation or reference TTS failure")}, nil
	}
	if stage != "render" {
		return platformprocess.CommandResult{Stdout: []byte(runner.manifest + "\n")}, nil
	}
	output, err := json.Marshal(runner.result)
	return platformprocess.CommandResult{Stdout: output}, err
}

func (runner *dubVideoScriptEffect) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
		observer(platformprocess.OutputStreamStderr, result.Stderr)
	}
	return result, err
}
