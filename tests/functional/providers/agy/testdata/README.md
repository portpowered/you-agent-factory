# Real agy -p invocation traces (golden-test source material)

Recorded 2026-08-08 against agy CLI **1.1.11**
(`C:\Users\andre\AppData\Local\agy\bin\agy.exe`). These are REAL
headless invocations, not hand-written fixtures. Use them to drive the
ANTIGRAVITY provider's parser/executor golden tests (work-order B1).
Record more with the same recipes when coverage needs it.

## Files

| file | what it proves | generating command |
|---|---|---|
| `agy-trace-simple-text.stream.jsonl` | minimal stream-json shape: `init` event (model, cwd, tool inventory) -> response -> final `result` with usage | `agy -p "Reply with exactly the text TRACE_OK and nothing else." --output-format stream-json --model gemini-3.6-flash-low --dangerously-skip-permissions` |
| `agy-trace-file-read.stream.jsonl` | tool-use events (workspace file read) + the `--add-dir` requirement | `agy -p "Read the file fixture-note.txt in the workspace and report the values of alpha and beta." --output-format stream-json --add-dir <abs-dir> --model gemini-3.6-flash-low --dangerously-skip-permissions` (workspace dir contains `fixture-note.txt` with `alpha: 3`, `beta: 7`) |
| `agy-trace-video-watch.stream.jsonl` | MULTIMODAL: watches `clip-fixture.mp4` (5s, 700KB, real MiniMax render) and reports visuals AND audio-track content (ambient drone, clock ticking, no speech) | `agy -p "Watch the video file clip-fixture.mp4 in the workspace. Describe ... and state whether the audio track contains speech, music, noise, or silence." --output-format stream-json --add-dir <abs-dir> --model gemini-3.6-flash-high --dangerously-skip-permissions --print-timeout 8m` |
| `agy-trace-structured.json` | `--output-format json` final envelope with `structured_output` + echoed `json_schema` | `agy -p "Classify this statement ..." --output-format json --json-schema '{"type":"object","properties":{"sentiment":{"type":"string","enum":["positive","negative"]},"confidence":{"type":"number"}},"required":["sentiment","confidence"]}' --model gemini-3.6-flash-low --dangerously-skip-permissions` |
| `clip-fixture.mp4` | the video input used by the video-watch trace | copied from tv-girl `production/factory-grounded/shots/SH080/SH080_CLIP_a6.mp4` |
| `groundtruth-fixture.mp4` | synthetic 4.0s ground-truth video: 0-2s solid red + white text `PHASE 1` + 440Hz sine; 2-4s solid blue + yellow text `PHASE 2` + digital silence. 320x240, 25fps, AAC mono 44.1kHz. Deterministic — golden tests can ASSERT perception. | built with ffmpeg (lavfi color+drawtext+sine+anullsrc concat) |
| `agy-trace-groundtruth-verbose.stream.jsonl` | VERBOSE multimodal review of the ground-truth video. The response states: exact 4.000s duration, 320x240, 100 frames @25fps, verbatim `PHASE 1`/`PHASE 2`, red->blue cut at 2.000s, 440.00 Hz A4 sine terminating to silence at 2.000s. Every one of those is a mechanical assert for golden tests. | `agy -p "Watch groundtruth-fixture.mp4 ... VERBOSE, exhaustive review: timestamped visual beats + verbatim text + audio timeline + duration/resolution" --output-format stream-json --add-dir <abs-dir> --model gemini-3.6-flash-high --dangerously-skip-permissions --print-timeout 8m` |
| `agy-trace-clipqa-schema.stream.jsonl` | the CLIP-QA GATE shape: video watch + `--json-schema` verdict `{action_completed, spec_deviations, temporal_artifacts, audio_content, unexpected_speech, verdict: pass\|reroll, confidence}` — returned `structured_output` verdict `pass` on the real clip. This is the fixture for the B2 clip-QA workstation. | `agy -p "You are a clip QA gate. Watch clip-fixture.mp4 ... judge against this shot spec ... emit the structured verdict" --output-format stream-json --json-schema '<QA schema>' --add-dir <abs-dir> --model gemini-3.6-flash-high --dangerously-skip-permissions --print-timeout 8m` |
| `agy-trace-missing-file.stream.jsonl` | FAILURE SHAPE: asking for a nonexistent file still yields exit 0 AND `status: SUCCESS`; the refusal exists only in response prose. | `agy -p "Watch does-not-exist-xyz.mp4 ..." --output-format stream-json --add-dir <abs-dir> --model gemini-3.6-flash-low --dangerously-skip-permissions` |

## Integration facts discovered live (encode these in B1)

1. **cwd is NOT the workspace in print mode.** Without `--add-dir`, agy
   answered from `~/.gemini/antigravity-cli/scratch` and refused to read
   a file sitting in cwd (see the first, kept-for-reference run inside
   `agy-trace-file-read` history: "Please create the file or open the
   directory as your active workspace"). The executor MUST pass
   `--add-dir <working-directory>` on every dispatch.
2. **Exit code is 0 even when the agent declines** (no-workspace case)
   — success detection must parse the final `result` event, not the
   exit code.
3. Final stream-json line is `{"event":"result", "result":{"status":
   "SUCCESS", "response": "...", "duration_seconds": ..., "num_turns":
   ..., "usage": {input/output/thinking/cache_read/total tokens}}}` —
   usage accounting is available per invocation.
4. `--output-format json` returns one envelope object with
   `conversation_id`, `status`, `response`, `structured_output` (when
   `--json-schema` given), `json_schema` echo, and `usage`.
5. Models available (via `agy models`): gemini-3.6-flash-{high,medium,
   low}, gemini-3.5-flash-{high,medium,low}, gemini-3.1-pro-{high,low},
   claude-sonnet-4-6, claude-opus-4-6-thinking, gpt-oss-120b-medium.
6. Video+audio understanding confirmed: the video trace's description
   matches the clip's actual content including audio character —
   this is the capability gpt-5.6 lacks (work-order B1 "Why").
7. `--print-timeout` default is 5m; raise it for long media reviews.
8. **`status: SUCCESS` does not mean the TASK succeeded.** The
   missing-file trace returns exit 0 + `status: SUCCESS` with the
   refusal only in prose. Task-level success must be established via
   `--json-schema` structured verdicts (or explicit response
   contracts), never via exit code or status alone.
9. **Budget note:** agy invocations are rate-limited for this operator
   (~10-20 runs remaining as of 2026-08-08). Golden tests must run
   OFFLINE from these recorded traces; live agy integration tests
   should be a single gated smoke test, not a per-CI-run cost.
