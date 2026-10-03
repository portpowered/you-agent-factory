# Multilingual dubbing and readable concurrent progress

## 1. Problem and desired outcome

### Problem statement

A customer cannot reliably dub Chinese video into English, and progress output
overwrites/interleaves itself instead of remaining readable.

### Current behavior and gap

The attached Windows log fails transcription when Whisper's final segment ends
1.283 seconds beyond decoded audio. The Factory defaults to English-only ASR,
supports only Chinese TTS, batches translation by segment count alone, and has
20-minute stage deadlines. Spinner and persistent event/log writes collide.

### Desired outcome and success measures

The customer's Chinese video completes with `--language en-US`, reference audio
for every spoken segment, English translations/subtitles, and unchanged source
video packets. Chinese, Japanese, English, and Korean regional/script language
tags select supported target speech languages. Long content is processed in
bounded batches without a fixed total input-size or duration rejection.

## 2. Scope and constraints

### In scope

Multilingual ASR selection, safe timestamp normalization, language validation,
translation batching, stage execution budgets, reference-conditioned synthesis,
and one coordinated progress writer for concurrent worker observations.

### Non-goals

Music separation, new application lifecycle owners, new state projections, and guarantees of
translation correctness or perceived speaker identity from structural checks.

### Assumptions and constraints

Use existing Models and Factory boundaries. Preserve source audio as the TTS
reference. Model context and memory are physical constraints; partition work
rather than reject the whole input. Keep machine output stable and UTF-8.

### Open questions and replanning triggers

Validate the requested Qwen3-ASR recognition and forced-alignment assets with
the reproducible Windows CUDA backend before declaring the default operational.
Replan if native ASR buffers an entire recording or if
language normalization needs a new public Models contract. The current file
is 240.067 seconds; the pasted log describes a 245.017-second decoded input.

## 3. Recommended approach

Three delegated tasks own multilingual dubbing, Models ASR, and concurrent CLI
progress; the delivery owner integrates packaging and runs the real video.
Use the existing models/renderer ownership and preserve canonical Factory
events. Normalize only defensible native timestamp drift, retaining failures
for invalid ranges and unsupported language/model combinations.

### Decision record

| Decision | Reason |
| --- | --- |
| Multilingual ASR for dubbing | An English-only model cannot transcribe the submitted Chinese speech. |
| Qwen3-ASR as the general built-in ASR default | User requested the recognition model; real forced alignment supplies timed dubbing cues. |
| One shared Factory GPU resource | ASR, translation, and TTS must serialize across concurrent Work within a Factory Session. |
| Default model invocation capacity of one | Existing Models lease admission prevents overlapping unconfigured invocations within a runtime scope; explicit positive capacity remains configurable. |
| Explicit supported target mapping | BCP 47 customer tags and native TTS language names are different boundary representations. |
| Partition long inference work | A small per-call context must not become a total video input limit. |
| One progress writer | Multiple workers can update progress without concurrent terminal writes. |

## 4. Customer behavior

Readable input/writable output follow OS permissions. Reject malformed or
unsupported target tags before expensive model effects. Loading shows one
compact status line in a terminal; redirected output contains stable milestones
without animated frames. Success reports artifact paths; failure reports an
actionable cause and keeps diagnostics. Terminal cancellation clears progress
and releases rendering ownership. Keyboard and text accessibility are preserved;
no browser or visual UI changes are needed.

## 5. Contracts and data

Models invocation and HTTP shapes remain unchanged. The authored Factory
signature preserves existing parameters; multilingual ASR/default and execution
budget changes are documented in the implementation record below after the
model selection is confirmed. Language normalization retains the canonical
BCP 47 tag in artifacts and translates it to the native TTS name only at invocation.
The packaged Factory catalog is regenerated from authored source; no generated
files are edited manually. Existing explicitly selected ASR models remain valid.

CLI invocation (unchanged grammar):

### Current and proposed configuration

| Surface | Current | Proposed |
| --- | --- | --- |
| `asrModel` default | `asr` resolving English-only Whisper base.en | Same `asr` identity resolving pinned Qwen3-ASR 0.6B Q8_0 and forced aligner |
| `language` | Default `zh-CN`, unchecked tags passed through | Same default; supported BCP 47 language/script/region/variant tags normalized; native Qwen names mapped explicitly |
| SCRIPT workers | Four authored `timeout: 20m` values | Omit stage timeout; caller/session cancellation or explicit budget controls execution |
| ASR inference input | Complete source video | Explicit video intervals of at most five minutes, global indexed IDs/timestamps, original video retained for voice references |
| Translation inference input | Up to 64 segments regardless of text length | Complete UTF-8 prompt partition target 12,000 bytes plus 64 segments; no whole-input rejection; oversized single segments passed through |
| MP4 language metadata | Ad hoc base-language mapping | Shared supported language mapping to ISO 639-2; MKV retains canonical customer tag |
| TTS parameters | `{"ref_text":"original source transcript","language":"English"}` with `voice=@original-reference.wav` | `{"language":"English"}` with the same `voice=@original-reference.wav`; audio-only speaker embedding, source transcript remains artifact metadata |
| Factory GPU scheduling | No declared shared GPU resource | `resources: [{name: gpu, capacity: 1}]`; transcription, translation, and synthesis each request `gpu` capacity 1; render does not request GPU |
| Default Models capacity | Missing or nonpositive authored capacity permits unlimited leases | One lease per model/runtime scope; explicit positive capacity overrides it |

Current authored Factory configuration (affected fields):

```yaml
resources: []
workers:
  - name: transcribe-video
    type: SCRIPT_WORKER
    timeout: 20m
  - name: translate-video-segments
    type: SCRIPT_WORKER
    timeout: 20m
  - name: synthesize-video-segments
    type: SCRIPT_WORKER
    timeout: 20m
  - name: render-dubbed-video
    type: SCRIPT_WORKER
    timeout: 20m
```

Proposed authored Factory configuration (affected fields):

```yaml
resources:
  - name: gpu
    capacity: 1
workers:
  - name: transcribe-video
    type: SCRIPT_WORKER
  - name: translate-video-segments
    type: SCRIPT_WORKER
  - name: synthesize-video-segments
    type: SCRIPT_WORKER
  - name: render-dubbed-video
    type: SCRIPT_WORKER
workstations:
  - name: transcribe-and-save
    resources: [{name: gpu, capacity: 1}]
  - name: translate-and-validate
    resources: [{name: gpu, capacity: 1}]
  - name: synthesize-from-source-audio
    resources: [{name: gpu, capacity: 1}]
```

Current native English TTS invocation:

```text
you models invoke qwen3-tts-base --operation TTS --input "text=Can anyone help?" --input "voice=@original-reference.wav" --parameter "language=English" --parameter "ref_text=有沒有好心"
```

Proposed native English TTS invocation:

```text
you models invoke qwen3-tts-base --operation TTS --input "text=Can anyone help?" --input "voice=@original-reference.wav" --parameter "language=English"
```

Current built-in ASR binding and default admission:

```text
asr -> localai-whisper -> ggml-base.en.bin
missing/nonpositive MODEL capacity -> unlimited concurrent leases
```

Proposed built-in ASR binding and default admission:

```text
asr -> localai-qwen3-asr-cpp -> qwen3-asr-0.6b-q8_0.gguf + qwen3-forced-aligner-0.6b-q8_0.gguf
missing/nonpositive MODEL capacity -> 1 concurrent lease per model/runtime scope
positive MODEL capacity -> authored capacity
```

Current backend publication toolchain constraint (affected field):

```json
{"vcpkgCommit":"<required immutable 40-character commit>"}
```

Proposed backend publication toolchain constraint (affected field):

```json
{}
```

`vcpkgCommit` may be absent for the Go/purego bridge, which does not consume
vcpkg. Any supplied value must still be a valid immutable commit; existing C++
publication metadata retains its actual vcpkg pin. All other toolchain metadata
continues to describe the actual build. ASR is published separately from the
existing TTS archive, with the custom bridge/recipe source commit and hashes
recorded in its archive provenance.

English/Chinese/Japanese/Korean examples are `en-US`, `zh-CN`, `zh-Hant-TW`,
`ja-JP`, and `ko-KR`, mapping to English/Chinese/Japanese/Korean native speech.
Region/script tags guide translation, not guaranteed accents. Five-minute clips
bound Factory inference inputs; the Qwen backend reads overlapping 30-second
audio windows. Factory clips may cut speech at a boundary, so source-word
quality across a boundary remains a real-model validation concern. UTF-8 prompt
bytes are a partition heuristic, not a tokenizer/context-window guarantee.

```text
you run --named @you/dub-video --video chinese.mp4 --language en-US --output-video english.mp4
```

Existing transcripts/translation artifacts remain inspectable. Model cache assets
use immutable source identities. Rollback is a source revert and regeneration;
already published output is not removed.

## 6. Runtime flow and ownership

Factory Work carries the manifest path between transcription, translation,
synthesis, and rendering. Models owns ASR/native response handling and asset
readiness. Packaged scripts own translation contracts and media alignment.
CLI/Factory Visualization own presentation only; worker events remain canonical.
No second runtime, opener, or persistent state graph is introduced.

## 7. Tasks and sequencing

1. Characterize native timestamp failure and correct multilingual ASR selection.
2. Validate target tags and partition translation/synthesis for long recordings.
3. Coordinate terminal writes and prove concurrent worker rendering/cleanup.
4. Regenerate packaging, rebuild once, and run the submitted Chinese video.
5. Independent review and focused shared-surface gates; resolve concrete failures.

## 8. Validation plan

| Behavior | Layer/boundary | Dependencies and isolation | Command/evidence |
| --- | --- | --- | --- |
| Native ASR timestamps and failure distinction | Models codec unit boundary | Controlled decoded response, independent fixtures | Focused Go codec tests |
| Regional/script language tags and exact translation IDs | Python component boundary | Controlled model replies, temporary per-case files | Python `-B` unittest discovery |
| Bounded multi-batch processing without total input rejection | Python component boundary | Generated transcript, controlled model edge | Long transcript tests |
| Concurrent status/events and final cleanup | Renderer component boundary | Coordinated concurrent observations, isolated writer | Focused normal and race tests |
| Public multilingual model/invocation behavior if changed | Functional `root.BuildProcess` + `Process.Execute` | Command-runner edges, explicit session/isolated profile | Focused Models functional tests |
| Delivered Chinese-to-English pipeline | Integration/media proof | One externally built binary, real GPU/models, isolated output directory | Installed CLI invocation plus ffprobe/ref hashes |

No unit test assembles the application. Functional tests do not build/spawn the
CLI, and external effects are replaced only through `edges.Edges`. Native/GPU
calls are serialized during manual validation to avoid contention. Large-scale
stress is outside this task; representative long transcript/real video evidence
does not claim every hardware/duration combination is tested.

## 9. Acceptance criteria

- [ ] Submitted Chinese video produces reference-conditioned English speech.
- [x] Chinese/Japanese/English/Korean supported tags have focused contract proof.
- [x] Long transcripts span bounded calls without losing segment IDs/timings.
- [x] ASR accepts defensible Whisper final timestamp drift and rejects malformed ranges; Qwen keeps strict real alignment.
- [x] Concurrent terminal updates do not interleave, wrap repeatedly, or leak frames.
- [x] Redirected output has stable nonanimated milestones.
- [x] Shared GPU capacity one serializes model stages and is reusable after failure.
- [x] Default Models invocation capacity one prevents overlapping unconfigured leases; explicit capacity two remains usable.
- [x] Managed Qwen3-ASR default pulls and invokes with both pinned assets.
- [ ] Packaging and focused shared-surface gates pass.

## 10. Delivery and remaining risks

Packaged Python validation: `python -B -m unittest discover -s
packages/packaged-factories/factories/dub-video/scripts -p 'test_*.py'` passes
46 tests. New witnesses cover canonical/native target names and early invalid
tags, Chinese source reference conditioning for English/Japanese/Korean,
bounded ASR clips with silence and global ID/time offsets, Unicode prompt-size
partitioning with an oversized individual segment, MP4/MKV language tags,
clear wrong-script replies, and explicit interval extraction of short sources
whose decoded audio can exceed the container duration.
Real source extraction proved time-based trimming alone insufficient: the
240.067-second video decoded to 245.017 seconds. Resampling before trimming to
`duration_ms * 24` samples at 24 kHz and resetting timestamps produced
`240.067000` seconds in
`C:/t/dub-multilingual-validation/source-reference-sample-trim.wav`; no GPU/model
call was needed for this media proof. That proved the length bound only;
subsequent correlation showed that end-only trimming did not repair indexing.
AAC packets near 5.011–5.018 seconds contain compressed timestamp durations,
placing later decoded PCM about 5.001 seconds behind the container timeline.
Original-video seek clips at 180/235 seconds correlate above 0.998 with plain
full PCM at 185.001333/240.001333 seconds. Timestamp-aware resampling with
`async=1:first_pts=0:min_hard_comp=0.001` produces 240.015667 seconds and aligns
both source seek clips at their exact requested positions. Default asynchronous
resampling retains about 34.667 milliseconds of residual drift, so the one
millisecond hard-compensation threshold is explicit. Factory extraction applies
this resampling before padding/trimming to the requested sample count; direct
Models video ASR applies the same timestamp correction at 16 kHz.
The final Factory extraction produced exactly 5,761,608 frames at 24 kHz
(240.067 seconds). Independent two-second seeks each produced 48,000 frames;
their strongest matches in the full normalized source are exactly 180.000 and
235.000 seconds, correlations 0.998633 and 0.998420. CPU-only evidence is saved
at `C:/t/dub-multilingual-validation/timing-review/aligned-correlation-proof.json`.
The earlier direct native ASR result was valid for a 245-second decoded WAV and
did not establish video-timeline correctness.
Real Chinese-to-English execution and regenerated packaging remain owner gates.

Real English TTS probe: ICL `ref_text` output lasted 4.48 seconds and multilingual
ASR returned `有没有好心 Can anyone help?`. Omitting `ref_text` while retaining
the identical original audio reference produced 1.44 seconds and ASR returned
exactly `Can anyone help?`. The existing native Qwen speaker-embedding mode is
used without a new adapter or a text-only fallback. Probe artifacts are under
`C:/t/dub-multilingual-validation/`. Source transcripts, indexed cue timings,
reference hashes, and original-video reference slices remain unchanged. Tests
assert audio conditioning and absence of `ref_text` for all four priority targets.

The GPU resource uses ordinary Factory resource acquisition/release, including
failure/cancellation paths. Its capacity is shared among model stages within a
Factory Session. Direct Models admission defaults to one invocation per model
within a runtime scope using the existing lease owner. This does not provide an
OS-wide GPU reservation across separate CLI processes or Factory Sessions.
No new persistent scheduling state is added.

The public `TestPackagedDubVideo` functional suite passed after GPU resource
regeneration (6.45 seconds). Two Work items share a live Factory Session and
controlled script-command gates. Public resource observations show total one,
available zero, and one active model stage while blocked; normal completion and
an initial ASR failure both leave the slot available and allow the peer Work to
complete. The test uses `root.BuildProcess`/`Process.Execute`, no built CLI or
fixed sleeps. Default-capacity host tests passed under Windows race detection:
no resource, unspecified capacity, explicit capacity two, and release/reuse.

The reproducible CUDA bridge's real English probe returned
`Can you assist me right now?` twice with one resident recognition/aligner pair,
including real word spans on a 4.872-second WAV. A 30-second interval from the
submitted Chinese video returned nine phrase spans after correcting whitespace
in detected language before alignment; it also exposed an upstream zero-duration
word requiring truthful grouping with an adjacent positive-duration phrase
before canonical ASR acceptance. A five-second silence probe returned
empty text and zero segments twice. These are native backend proofs, not yet
proof of managed pull or complete Factory delivery. Probe evidence is under
`C:/t/dub-qwen-asr-probe-*.json` and the corresponding native server log.

Final native repairs are committed as
`db28671aeba6d17156c54086f2cade2626a95dfa`. A real 60-second interval from the
Chinese source passed across three streamed windows, producing 16 positive-duration
phrases with global timestamps through 55.84 seconds. Zero-duration aligned
words stay in adjacent positive-duration phrases without inventing their times.
Cancellation then an English request succeeded on the same loaded model pair.
Cancellation during forced alignment waits for the current 30-second audio
window; ASR token generation and window transitions check cancellation.
Unsupported prompt, nonzero temperature, and unknown timestamp granularities
fail explicitly. Segment/word granularities are accepted; phrase output retains
word evidence on the native wire. Canonical saved ASR JSON contains phrase
ID/start/end/text fields and does not promise a per-word JSON output.

The native audio reader holds at most 480,000 float samples plus a 16 KiB
conversion buffer. Its tests cover overlapping windows, PCM16/float32, RIFF/RF64,
malformed/truncated input, and a sparse RF64 file above 4 GiB with a successful
seek/read of its final known samples. Native audio-window memory is bounded;
transcript/output storage and upstream CLI/media materialization still grow
with input or output size. No whole-input size or duration rejection is added.

Structured semantic audit issues with unique known source IDs can supply complete
target-language replacements. Patch only those indexed texts, revalidate the
entire candidate against the unchanged source IDs/times/glossary, and audit again.
Generic/mixed/duplicate-ID issues and invalid replacements regenerate the batch.
No path exceeds three total reviews. Saved candidate files preserve the evidence
for each review. Tests prove unchanged neighbors, invalid glossary replacement
rejection, duplicate-ID fallback, and the review ceiling without full regeneration.

A real review proposed changing Chinese `也不少` from many to not many. Translation
and audit prompts now require source-language idiomatic polarity, preserving the
literal proposition under irony, and checking proposed corrections against the
source before rejecting an equivalent paraphrase. This guidance does not provide
a deterministic semantic guarantee; the three-review ceiling and saved source,
candidate, and audit evidence remain necessary for diagnosing model mistakes.

An actual 320 ms source cue (`这是，`) generated 1.120 seconds of English speech
(`This is,`) and failed the unchanged two-times speech-speed guard. The next
source cue starts at 67,200 ms, leaving enough unoccupied trailing silence.
Synthesis now borrows only the required trailing gap, bounded by the next cue
or video end. ASR/source `start`/`end`, IDs, original reference slices, and hashes
remain evidence; `speech_end` records the derived playback end. Generated SRT,
ASS, and PCM share that playback window. Operator-supplied ASS remains authored.
Insufficient gaps still fail the two-times guard; no output overlaps the next cue.

Current affected translation artifact:

```json
{"id":16,"start":65120,"end":65440,"text":"This is,","source_text":"这是，"}
```

Proposed affected translation artifact:

```json
{"id":16,"start":65120,"end":65440,"speech_end":66240,"text":"This is,","source_text":"这是，"}
```

Five focused playback component tests cover measured-duration gap borrowing,
unchanged source evidence, absent/insufficient gaps, last-cue video limits,
invalid/overlapping playback metadata, and shared PCM/subtitle presentation.
The original reference-conditioning test also observes the bounded fit request
and saved derived end while proving extraction still uses original cue bounds.

The first post-audit TTS cue generated 163.84 seconds for `Ahhh!`: exactly the
native default 2,048 frames at 80 ms each. The native backend had treated reaching
`max_new_tokens` without EOS as successful completion. Native generation now
rejects exhaustion before publishing a WAV and retains the precise error through
the Go bridge. Models maps only that anchored native diagnostic to the safe
message `TTS generation limit reached without EOS`; other private details remain
in the error cause. Failed invocations expose no canonical audio output.

Real repeated `Ahhh!` requests with the identical 7.52-second original crying
reference and default random sampling produced: successful 8.40-second output,
explicit exhaustion with no WAV, then successful 1.12-second output. This
demonstrates stochastic recovery with unchanged conditioning, without promising
all retries converge. The Factory permits three total attempts only for the
structured `MODEL_BACKEND_FAILURE` plus the exact safe exhaustion message.
All attempts use identical text, language, reference path/audio bytes, and
parameters. Distinct retry audio paths and per-attempt failure JSON preserve
evidence. Other errors and cancellation stop immediately; the two-times fit
guard remains independent and unchanged. Four additional component tests prove
unchanged retries, saved failures, the attempt ceiling, and rejection of generic,
unstructured, non-exhaustion, and cancellation failures. The Python suite now
contains 46 tests.

The next real run passed semantic review and recovered an EOS failure, then cue
2 failed the fit guard: 2,320 ms of speech in a 1,120 ms window needs 2.0714x.
Initial translation prompts now supply each source cue's duration and request
compact natural speech without a hard word count or omitted meaning. Only a
measured `SpeechDoesNotFit` rejection can request a targeted concise revision.
There are at most three speech versions including the original; intermediate audited text candidates are bounded separately. Revisions receive
the measured duration, available playback duration, unchanged original source,
and two neighbors on each side. The complete candidate is structurally validated;
the changed cue and its source neighbors are audited before regeneration.
Unchanged cues retain previous semantic approval. Full candidate and failed
speech/evidence remain saved, while canonical translations change only after
successful fitting. Every version retains the identical original audio reference,
source bounds, and IDs. Generic model/media errors and cancellation remain fatal.
The two-times speed guard and indexed playback constraints remain unchanged.

Current initial translation prompt segment:

```json
{"id":2,"text":"有没有好心人可以帮帮我？"}
```

Proposed initial translation prompt segment:

```json
{"id":2,"text":"有没有好心人可以帮帮我？","duration_ms":1120}
```

Output segments still allow only original ID and target text. Focused controlled
tests cover measured overflow, complete candidate validation, source-context
audit before TTS, unchanged neighbors/references, semantic rejection, three-version
exhaustion, and immediate generic-error propagation.

The first real fit repair returned an outer array using `segment_id` and
`replacement_text` twice. Both replies were correctly rejected. The repair prompt
now provides the concrete requested-language/ID `{language,segments:[{id,text}]}`
object shape, explicitly distinguishes input metadata from output fields, and
requires the previous rejection to be corrected. No invalid-array normalization
was added. A controlled malformed-array then valid-object witness verifies the
schema guidance, retained feedback, strict rejection, and audit before acceptance.

The schema-fixed real reply `Can any kind person help me?` was then rejected by
the audit in favor of the longer but equivalent `Is there anyone kind who can
help me?`. Fit audits now select only changed original IDs using `focus_ids`;
the two neighboring cues on each side remain source context and retain previous
approval. The prompt explains equivalent concise interrogative grammar while
continuing to reject lost meaning, changed roles, and inverted polarity. Focused
issues must use the structured issue shape and identify a changed cue. Initial
translation audits still select every segment. Four focused audit component tests
cover selected IDs/source context, faithful concise grammar guidance, malformed
or neighbor-directed issues, actual role reversal rejection, and invalid focus
rejection before invocation. The Python suite passes 56 tests.

The EOS repair is source commit `a344ad7c327b56e7c9fd26dc9db5914aa4522da1`.
Its new immutable TTS-only Windows CUDA publication is
`localai-backends-v1-847f8b8c954d54d913886ee5e43be40477e3f53e8d846f1f956b69be24ccca7d`.
Anonymous download verified 445,863,586 bytes and SHA-256
`c2a3d82d9c8029e51298f17727a4e44535f080a64844d273ffe4068ae201b99d`.
The ASR publication remains unchanged. A real native request with a one-frame
budget failed without a WAV; the next normal request reused the loaded model
successfully. Independent CPU bridge tests verify failure/no output followed
by successful output, and all provenance source hashes match the source tree.

Functional CI on `9aaf678` passed its tests but measured logging coverage below
the required floor (67.77% versus 69.67%). The unreachable `BuildLogger` default
terminal policy is removed in favor of the existing canonical terminal builder.
A public finite CLI failure test verifies error records, caller and stack trace
retained in the runtime file, quiet normal terminal output, and verbose request
details. Normal and race runs passed; the coverage floor is unchanged and the
full CI gate must still measure the repaired tree.

Record actual commands, measured artifacts, review results, and remaining limits
after implementation. Do not claim semantic translation/voice quality solely
from JSON validation, a same-model audit, or nonzero audio samples.

The final native seam repair is commit
`7e640360a555b8b8a7460deee87fced4e1aef14f`. Original overlapping word spans
remain intact while phrase envelopes merge, rather than rejecting quantized
seam evidence. Immutable Windows CUDA publication
`localai-backends-v1-8d2cd1a1c4337e1c8efb60bf64ffdfb9a559a91e6a6d06ec3eea7df2996e5e8a`
was anonymously downloaded and verified: 441,891,463 bytes, SHA-256
`e20cafd64c87a5a0293f6a7a19ef20be3566aa1a0cc1af54f70c383b7fdb531c`.
The managed `models invoke asr` endpoint now successfully processes the original
Chinese MP4: 58 positive, nonoverlapping cues, ending at 239,920 ms on the
timestamp-normalized 240.016-second audio. Saved proof is
`C:/t/dub-multilingual-validation/qwen-final-chinese-asr.json`.

The pinned upstream LocalAI SDK has a 50 MiB RPC message limit. Audio input
travels by file path and this does not impose a recording-size limit; exceptionally
large transcript responses can still exceed that upstream output limit. Factory
ASR partitioning bounds each response, while direct whole-recording invocation
does not yet partition its transcript response.

The first fresh Factory attempt exposed a separate audio-format gap: its
24 kHz reference WAV reached a backend requiring mono 16 kHz PCM16/float32.
Models now normalizes unsupported ordinary audio formats through the existing
FFmpeg/temp-file effects before the Qwen codec; native-format WAV avoids that
conversion. Focused tests cover 24 kHz WAV, compressed audio, and the bypass.
The rebuilt installed CLI completes the actual Factory transcription step:
59 cues on its padded 240,067 ms reference clip, final cue ending at 240,000 ms.
This is a separate resampling path from direct video ASR, so differing phrase
counts do not imply reused or edited transcript evidence. Native inference
errors retain their nested diagnostic cause instead of discarding it.

The fresh Factory run on `fa2ff80` passed transcription, English translation,
and its first semantic audit, then saved the translated manifest. Its Python
worker exited with code zero, but the command monitor classified it as
`PROCESS_GONE` and cancelled the attempt before TTS started. Runtime evidence
is the `command_runner.completed` record with `exit_code: 0`,
`cancellation_reason: PROCESS_GONE`, and duration 322,008 ms. The operator did
not cancel this run.

The process monitor previously allowed only 50 ms between observing leader exit
and declaring the process lost, although `exec.Cmd.Wait` must also join output
copying and observer callbacks. It now respects the existing bounded orphaned
output-pipe drain allowance, with the existing observation margin, and checks
Wait completion before publishing process loss. Caller cancellation and actual
exit errors retain their original handling. A deterministic component test
covers output completion after the former 50 ms threshold; real-process tests
cover zero exit with inherited pipes, nonzero/killed processes, and interruption.
Focused race tests and the short process race suite passed.

Two unrelated CI repairs preserve existing assertions and limits: EOS tests
were consolidated into the existing protocol test file to satisfy the package
file-count limit; the metrics deadline test now observes its behavior deadline
before starting its existing cleanup watchdog, removing the deadline/watchdog
race under coverage. Neither repair changes production timeout policy or
coverage floors. The installed CLI was rebuilt with the lifecycle repair before
a fresh full English acceptance run.

The rebuilt lifecycle run completed translation and its first audit, then
entered TTS normally. The first cry cue exercised the real native EOS failure:
the factory saved its structured failure, retried unchanged text/reference,
and produced a successful two-second WAV on attempt two. The next cue completed.
Cue 2 then correctly failed the preserved fit guard: its source/playback interval
was 16,320–17,440 ms with no trailing silence, and the nine-word English request
required 2.320 seconds, or 2.0714 times the available interval. This is a separate
translation-duration issue, not a lifecycle cancellation or swallowed error.
The acceptance loop therefore adds duration-aware initial wording and bounded,
audited targeted shortening for measured speech overflows, retaining source
alignment and reference conditioning rather than weakening the timing guard.

Functional CI on `ef503c42` passed and measured logging coverage at 70.7% above
the unchanged 69.7% reported floor; all 462 gated packages passed. Backend lint
passed all 24 canonical checkers without changing the package-file baseline.
Unit CI found one stale archive assertion; commit `ce87516b42` changes only its
expected TTS archive byte count and hash to the independently verified release,
retaining offline selection, CUDA, CPU rejection, and Darwin assertions.


### Qwen3-ASR 1.7B comparison on ambiguous Chinese speech

A bounded native comparison tested the existing 0.6B model and a compatible
1.7B Q8_0 candidate through the same pinned Windows CUDA loader and existing
0.6B ForcedAligner. The pinned native converter explicitly supports 1.7B;
the candidate GGUF metadata and tensor namespace match the loader. The official
Qwen example pairs 1.7B ASR with that same 0.6B aligner. This validates native
loading and inference, rather than assuming that a larger model fixes the clip.
See the [pinned converter](https://github.com/predict-woo/qwen3-asr.cpp/blob/6dcc586e5073fd6e85ee5728e75f0903d6c70c6c/scripts/convert_hf_to_gguf.py)
and [official model and alignment example](https://huggingface.co/Qwen/Qwen3-ASR-1.7B).

The downloaded candidate is
`hf://dseditor/qwen3-asr-1.7b-GGUF/qwen3-asr-1.7b-q8_0.gguf@ee6cf896a2ab854a0dcbedfd7002207120c842bf`.
Its complete download was verified as 3,200,764,736 bytes with SHA-256
`1e448ead1220e819c8ef6e8e30ba4355e5d7899ef19f03670a4a1a6e9a37d4bf`.
The [immutable asset repository](https://huggingface.co/dseditor/qwen3-asr-1.7b-GGUF/tree/ee6cf896a2ab854a0dcbedfd7002207120c842bf)
and local `C:/t/dub-qwen-asr-1p7b/provenance.json` record its provenance.
The alternative `cstr` GGUF uses different metadata and tensor names and was
not loaded as a substitute for this backend's compatible format.

Both models received identical canonical mono 16 kHz PCM clips from the corrected
source timeline for source cue IDs 25, 27, 52, and 57, with two seconds of
context on each side, an explicit Chinese language hint, and four CPU threads.
The comparison did not show a substantial correction of the reported ambiguous
speech. Both emitted `为什么加` in cue 27 and `顶我的脸` in cue 57; both emitted
`不耍我` in cue 52. Cue 25 retained `很贵` with 0.6B but became `不会` with 1.7B.
These are observed decoder outputs, not a word-error-rate claim: no independently
corrected human transcript was supplied. Short-context 0.6B output also differed
from its full-video output, so this does not establish full-video accuracy.
The default remains 0.6B because the measured result does not justify migration.

On the RTX 4090, warm per-clip inference plus alignment times in cue-ID order were
0.6B: 0.281, 0.125, 0.157, and 0.234 seconds; 1.7B: 0.265, 0.156, 0.157, and
0.234 seconds. Complete model-plus-aligner load times were 1.047 and 1.812 seconds.
Whole-device GPU memory used was 5,355 MiB with 0.6B and 7,869 MiB with 1.7B,
an increase of 2,514 MiB; free memory was 18,784 and 16,270 MiB respectively.
These are whole-device snapshots rather than exclusive process allocations.
After both probe processes exited, usage returned to the 2,039 MiB baseline
with 22,100 MiB free. The larger bundle would require 4.195 GB including the
unchanged aligner, compared with the current 2.348 GB. The short measurements
are not a long-input throughput guarantee.

The initial direct Python `ctypes` research harness completed all four 0.6B
inferences and saved the results, then failed during interpreter shutdown when
native static-model destruction called `cudaFree` after CUDA driver shutdown.
Explicitly releasing the native DLL while Python was still alive corrected
that harness teardown; final 0.6B and 1.7B probes both exited zero. This was a
direct research harness failure, not an observed MCP endpoint failure. Logs,
clips, timed results, real word alignment, and the initial failure remain under
`C:/t/dub-qwen-asr-1p7b/`. No native servers remain and no catalog, factory,
backend archive, or production model default changed during this comparison.

The real focused cue-2 repair now returned the exact required JSON object with
`Can any kind person help me?`. Its focused contextual audit returned valid=true
and no issues. Native reference-conditioned TTS produced a 1.840-second WAV,
which fitted the unchanged 1.120-second source/playback interval at
1.642857 times speed, below the preserved two-times guard. Reference SHA-256
is `8ded3fda62bb22ecb779ed095a768a49085510d49cd84db8423d6857943b80e3`.
The saved result is `C:/t/dub-multilingual-validation/fit-focused-audit-probe/fitted-segment.json`.
All inference and fit steps completed; the standalone research script's final
console print then raised a Windows cp1252 UnicodeEncodeError on Chinese source
text. This is a research reporting failure, not a model or factory failure;
the production entrypoint already reconfigures both streams to UTF-8. CPU
inspection independently read the saved result, audit, and WAV duration.


## Constrained text responses and bounded fit correction review

The latest full English run stopped legitimately at cue 2. Its first concise
proposal omitted kindness; the next omitted the person receiving help. Both
were rejected before replacement speech. There is no final delivery acceptance
or merge claim from that run.

Text-only Models calls now receive explicit GBNF for translation, fit proposals,
and audits. Translation grammar fixes the original ordered IDs and requested
language; audit grammar permits either approval with an empty issue list or
rejection with structured corrections for permitted source IDs. Unicode text and
legal JSON escapes remain supported. File pointers cannot be followed by these
plain model calls. Grammar constrains representation and does not establish
semantic accuracy; structural checks, glossary checks, source-context audits,
and the 2x speech guard remain required. Real native grammar/reasoning
compatibility is pending; no global reasoning setting or output cap changed.

For each of the two possible replacement speech versions, a fit proposal permits
at most three audited candidate forms. An indexed audit suggestion is validated
against the complete source and glossary, then audited again with the same
focused cue and neighboring source context. Only an approved form reaches TTS.
Intermediate complete candidates and rejections are retained; an approval file
identifies the accepted audit round. Model errors and cancellation propagate.
A controlled component witness reproduces missing-kindness rejection, faithful
indexed correction, re-audit approval, and only then replacement TTS; another
witness bounds repeated semantic rejection to three reviews per proposal.


The first native grammar probe enforced the requested Korean object/ID prefix
despite a prompt requesting plain prose, but exhausted its explicit research
512-token budget after producing hundreds of inter-token blank lines. The
incomplete output is preserved under `C:/t/dub-multilingual-validation/native-grammar-proof`.
The grammar now permits at most one optional whitespace character between JSON
tokens, preventing that unbounded whitespace path. Translation strings remain
unbounded legal JSON text; this adds no token, reasoning, or text-length cap.
The 62-test component suite passes; a repeat native probe remains required.


## Supplied larger Qwen: actual managed comparison

The supplied local Qwen GGUF was successfully pulled and invoked through the
installed public Models CLI using an isolated named operator configuration;
this supersedes the earlier external note's untested managed-route statement.
The exact model/projector hashes were preserved. The first offline invocation
selected the CPU archive and failed before load after a CUDA pull; normal
managed invocation succeeded. This observed offline-selection issue remains
recorded without a speculative resolver fix.

A managed text call produced 16 complete translations of four clean Chinese
propositions into English, Japanese, Korean, and Spanish. The positive meanings
of 不少/不错 and the complete band name/music-playback action were retained.
These are proposition checks, not native-speaker fluency certification. The
response was a fenced outer array despite a requested object: this was a schema
failure, not output-budget truncation. The complete UTF-8 artifacts are preserved
under `C:/t/dub-local-qwen-managed/`; a later cp1252 reporting error did not
invalidate the successful Models invocation.

Observed device free memory reached only 430 MiB during the multilingual call.
The managed effective context was not exposed, the projector was not exercised,
and neither 131k context nor projector/MTP settings were proven. English cue 2
remained ten words, with no demonstrated concise-fit or TTS-duration advantage.
The larger model is usable when resources permit, but the built-in default
remains unchanged. Source-ASR ambiguities and self-audit limitations remain.
The customer preference for easier/freeform agent outputs is retained; plain
model steps requiring indexed data use constrained grammar. Full Japanese,
Korean, and Spanish long-video tests remain pending after merge, alongside the
still-required successful English full delivery. Comparison evidence is in
`C:/t/dub-local-qwen-comparison/comparison.md` (final public-route sections).


The repeat native probe with bounded whitespace exited successfully and returned
a complete object with exact language `ko-KR` and ordered IDs 4,11 in 62 tokens,
although its prompt requested plain Korean text without JSON. The audit call
returned exactly `{"valid":true,"issues":[]}` in 16 tokens despite a prompt
requesting the plain word INVALID. Both completed below their explicit research
512-token budgets. Saved grammars exactly match the current Factory generators;
independent CPU inspection confirmed response schemas, IDs, language, usage,
and proof-file agreement. Evidence: `C:/t/dub-multilingual-validation/native-grammar-bounded-ws-proof/`
and `C:/t/dub-multilingual-validation/independent-review/current/native-grammar-proof.json`.
This confirms the public parameter/native transport enforces representation
without a global reasoning change. The deliberately contrary prompts provide
no semantic-accuracy or complete-video-delivery evidence. The actual cue-2
repair and reference-conditioned TTS probe remains a separate acceptance step.


## Freeform comparison before constrained semantic decisions

The real grammar-backed cue-2 fit probe produced `Can anyone help me?` and its
11-token audit approved it despite the explicit kindness modifier in the source.
This is an observed false approval. It does not establish grammar as the cause;
prior runs, context, and generation differ. The isolated uncapped freeform
comparison subsequently explained the missing modifier and proposed faithful
alternatives, using 1093 tokens. It also criticized tone, so an independent
final decision must distinguish stylistic preferences from proposition changes.
Evidence: `C:/t/dub-multilingual-validation/fit-grammar-correction-probe/` and
`C:/t/dub-multilingual-validation/audit-freeform-reasoning-probe/`.

Every semantic audit now first obtains and saves freeform source-first comparison
prose: source actors, actions, recipient, explicit modifiers, quantities, polarity,
and names are compared with candidate wording. The second call receives the
original source/candidate and that comparison as untrusted review evidence;
it independently checks claims and returns the existing constrained indexed
decision. No default token, text-length, or reasoning cap is added. This adds
one model call per audit review, retaining the same focused IDs, three-review
ceilings, validated correction re-audits, original reference audio, and 2x speech
guard. Empty comparison fails before a decision; model failures and cancellation
propagate from either stage. Earlier decision/fit tests isolate the comparison
boundary, while three dedicated component tests exercise actual two-dispatch
ordering/evidence forwarding, empty evidence, and both-stage model failures.
The complete 65-test suite passes. Native bad/faithful modifier, role-reversal,
and polarity cases remain required before another full delivery claim.

### Native two-stage semantic audit proof (2026-10-02)

The actual built-in LLM passed four source/candidate cases through the public
Models CLI with the new sequential freeform comparison and grammar-constrained
decision. It rejected an omitted explicit kindness qualifier, reversed helper
roles, and the incorrect negative rendering of Chinese 不少. It accepted the
faithful concise question “Can any kind person help me?”. Comparison usage was
926–1223 generated tokens; final decisions used 11–36. No output/reasoning cap
was supplied. Prompts, both responses, usage, corrections, and exit-zero proof
are retained under `C:/t/dub-multilingual-validation/audit-two-stage-native-proof/`.
These controlled cases support the repair; they do not establish complete-video
translation accuracy or native-speaker fluency.

An eight-minute fixture was prepared from two repetitions of the original demo
with copied video packets and clock-normalized AAC audio. Its measured container
length is 480.223242 seconds and size 88,573,882 bytes. The original was preserved.
`C:/t/dub-multilingual-validation/chinese-long-8min.mp4` crosses the five-minute
ASR partition; post-merge Japanese/Korean/Spanish end-to-end runs remain pending.

### Exact-head CI repair notes (2026-10-02)

CI for `02b9c9dc593066355166f0d1f5b6d9b4158846d4` completed with failures;
coverage floors were not evaluated because tests failed. Vet found a test-only
protobuf mutex copy in formatting, corrected to format its pointer. An ACP SDK
peer-disconnect outcome skipped otherwise safe redacted stderr; normalization
now retains that evidence while preserving dependency classification and
cancellation precedence. Focused normal tests ×10 and race tests ×2 passed.

The controlled integration runner, a test harness, observed Linux `ETXTBSY` on
start. Its repair retries only that exact transient error at most five times,
with 10 ms cancellation-aware waits; other errors remain immediate. A real held
write descriptor demonstrates failure then successful execution after close,
including the existing identity/report/ledger/cleanup assertions. Full Linux
and Windows platform-conformance suites and targeted Linux race tests passed.
The original transient descriptor origin in CI is not proven.

The rollout test treated `DISPATCH_RESPONSE` as a projection barrier, although
runtime recording precedes end-of-tick snapshot publication. It now awaits the
exact expected terminal Work identity/state and session categories before all
unchanged assertions. Normal ×20 and race ×5 passed. This changes test readiness,
not canonical event ordering or runtime state ownership. The in-flight HTTP test now awaits the exact scoped Work PROCESSING projection before its single Worker Sessions read. Normal and race runs ×3 passed; all original assertions and deadlines remain. Final-head CI remains required.


## Conservative nonverbal source-audio preservation

The fresh two-stage English run accepted all 59 translations but stopped at
cue 0 after three explicit native TTS EOS-exhaustion failures. Source cue 0 is
`啊啊！`, 5920–13440 ms, approved translation `Ahhh!`; the original reference
is 7.52 seconds. No incomplete synthesized audio was accepted or truncated.
Evidence remains under `C:/t/dub-multilingual-validation/chinese-english-audited-dub-8od5751h/`.

A narrow pre-TTS policy now preserves original audio only when BOTH complete
source and approved target strings match the same documented repeated-cry or
laughter spelling category. The inventory supports Chinese, Japanese, Korean,
English and Spanish phonetic forms; single interjections, mixed lexical phrases,
annotations, different categories and operator glossary names do not qualify.
This is not independent acoustic classification and cannot establish that an
ASR interval contains no hidden words. It is never a backend-error fallback.

Matched cues keep original start/end, approved caption, reference hash, normal
speed, and exact mono24k decoded source PCM with no following-gap borrowing,
additional padding or trimming after normal source-reference extraction. They record `audio_origin: source-nonverbal`, phonetic kind,
no TTS attempts and one fitted source version. Other cues record
`audio_origin: reference-conditioned` and retain all existing audit, TTS, source
reference and 2x fit guards. Independent delivery verification separately checks
the conservative spelling policy, exact source window and PCM equality against
re-extracted original references; the ordinary synthesis lineage remains required.

The actual cue-0 CPU-only production-helper proof preserved exactly 360960 PCM
bytes in the 7.52-second source window with no model invocation. Its saved
reference/provenance result is `C:/t/dub-multilingual-validation/source-nonverbal-cue0-proof/`.
The complete 70-test suite passes, including multilingual positives/lexical
negatives, exact WAV→PCM/timeline/subtitle positioning, no TTS dispatch, no
backend-error fallback and reference-length mismatch rejection. This proves the
isolated nonverbal path, not complete Factory delivery; another full run remains
necessary. General native EOS stability for lexical speech remains under review.

### Native sampler robustness repair (source validation)

Source inspection found a separate numerical-safety defect: invalid sampling
probabilities could fall through to a reserved token. No log evidence establishes
that this caused the opening cry's three EOS failures. The new native patch
rejects NaN, positive infinity, missing finite candidates, and invalid softmax
sums while retaining deliberate negative-infinity masks and valid finite
sampling order. Standalone CPU C++ regression tests cover these failures, greedy,
top-k/nucleus, 128-seed finite multinomial behavior, and the EOS exemption.
They pass with warnings treated as errors; patch application/reversal and the
PowerShell recipe parse also pass.

Private native diagnostics now record resolved seed, sampler settings,
conditioning mode, and last EOS logit/rank/finite counts. Identical Factory
requests use fresh native random seeds by default. Sampling defaults, generation
budgets, strict EOS rejection, and the safe public exhaustion message remain
unchanged. Native build, actual inference proof, checksum-verified publication,
and Models manifest selection are pending; the currently installed archive does
not yet contain this source repair.


### CI source-discovery and hermetic test repairs (2026-10-02)

CI run `36984316435` for head `49d340c247f9df15c2eae77ca2c8e4eb598c1db2`
finished with five failed checks: Backend Lint, Backend Unit Coverage, Backend
Functional Coverage, Development Package / Packaged Factories Package, and the
downstream Verification Policy. Unit and functional coverage floors were not
evaluated: their `go list -deps -test -mod=readonly ./...` preflight rejected the
new C++ sampler regression in the existing Go `scripts` package. The same
source-discovery error prevented vet and deadcode measurement. Other lanes,
including Backend Integration and TTS Clean-Install Windows, passed.

Commit `dea7691547a1763721b3fd7ffffe9582d1b27c62` renames the sampler template
from `.cpp` to `.cpp.in` and updates the native recipe's copy path; its contents
and copied native regression remain unchanged. Independent
`go test ./scripts -run '^$'` now passes. This is a source-placement repair,
not a new native inference or archive-publication proof.

The package check separately found two nonverbal Python component tests invoking
FFmpeg on CI without that executable. Their command edge now decodes the fixture
WAV with Python `wave`, preserving the exact PCM/timeline/subtitle assertions and
the reference-length mismatch failure. Production behavior is unchanged; the
actual FFmpeg cue-0 proof remains separate. All 70 tests pass independently in
0.249 seconds with the child process `PATH` empty, so neither FFmpeg nor ffprobe
is available. The author also regenerated the catalog and passed its drift check.
Final repaired-head CI and complete English-video acceptance remain required.

### Native Windows sampler publication proof (2026-10-02)

The native source repair is now built and published from committed recipe
`dea7691547a1763721b3fd7ffffe9582d1b27c62`. The final cached Windows CUDA
recipe passed its MSVC CPU sampler regression, native compilation, and Go
failed-generation/no-artifact/recovery regression (0.027 seconds). The earlier
repeat-build failure is preserved separately: applying the sampler changed
zero-context EOS offsets, so the corrected recipe removes only its own sampler
patch before checking the EOS base and reapplies it afterward. The `.cpp.in`
template avoids Go package source discovery without changing C++ test content.

Actual final-package inference on RTX 4090 passed original-reference English
synthesis (88,364-byte WAV, 1.84 seconds), explicit `max_new_tokens=1` EOS failure
with no WAV, and same-host English recovery with an identical WAV SHA-256. The
bounded failure's private diagnostics record EOS logit 5.83163, raw rank 114,
2,049 finite candidates and zero invalid candidates. Its original safe public
exhaustion error and default 2,048-frame budget remain unchanged. A separate
prior package of the same native source completed one default-random cry request
in 3.52 seconds with EOS at frame 44. This does not establish the cause or
resolution of earlier intermittent cry failures. The owned native probe hosts
were stopped after testing.

The immutable [Windows CUDA sampler release](https://github.com/portpowered/you-agent-factory/releases/tag/localai-backends-v1-53cf3a40d01e831eff5ba390a8f761badef03824e2b6439dfc1fd5ac0072cc87)
contains the final archive and manifest. Its archive is 445,867,793 bytes with
SHA-256 `58fe20ab56dcf6f4e4e485d71138bfa2c24605c1823817a763617199c9172422`.
Independent assembly checks matched nine exact committed recipe inputs, 22
payload/license hashes, and all 23 decompressed ZIP entries including complete
native provenance. Downloading the published archive reproduced its exact size
and SHA-256; the downloaded manifest matches the reviewed bytes with SHA-256
`72a8b8c18aafc58f8b5cafbb340a82361feb6f5a5aa4b1f1d07232a614bc4421`.
Publication used `--latest=false`; GitHub's default latest remains the separate
`localai-backends-v1-3dc488f3dc9848f8e245954d28a3cd223aca1be42003e739d66ee3dd3b423598`.

The protocol revision `ad62c6df07ae1169eb14411a565a689cd996b19c` is the actual
Git blob ID of the pinned SDK's `backend/backend.proto`; its last-changing
commit is separately recorded as `b00422e45fcfff5abb67066c9313e2628aea0d44`.
Provenance also preserves SDK/native/ggml commits, compiled-source hashes,
actual Go/MSVC/CUDA toolchain, original model/reference hashes and native probe
artifacts. CUDA execution remains verified only for sm89 on RTX 4090.

The embedded Qwen TTS manifest and exact offline selection regression now point
to this publication, including archive identity/location, size and checksum.
Raw final evidence is under `C:/t/dub-qwen-tts-sampler-final-proof/`, assembly
under `C:/t/dub-qwen-tts-sampler-final-release/`, and remote verification under
`C:/t/dub-qwen-tts-sampler-remote-verification/`. Public managed Models proof
using the rebuilt CLI, full English Factory delivery, and final repaired-head
CI remain separate pending gates; native publication alone does not satisfy them.


## Public TTS selection and quality acceptance update (2026-10-02)

### Explicit budgets and live publication identity

Commit `c7954cd786339d20b6a7c11307c12c4671117a5d` forwards an explicit
numeric, positive integral `max_new_tokens` through protobuf `Params` as a
canonical decimal string. Only the native signed 32-bit C integer representation
is bounded; omission leaves the parameter absent. Exact `json.Number`
mantissa/exponent checks reject rounded fractions and accept integral decimal
or exponent forms without expanding unbounded powers of ten. Invalid values
fail before backend connection. No public seed parameter or default budget was
introduced; focused codec/protocol tests and docs/maintainability gates pass.

The first public positive/one-frame failure/recovery sequence used the older
`a344ad7c` backend, as live installed provenance and DLL hashes proved. Those
calls validate its public error handling, not the sampler publication. GitHub's
release listing placed the older TTS publication (04:42:07 UTC) before the newer
one (08:41:57 UTC); the resolver selected the first compatible entry. No separate
model-cache invalidation defect was established. Commit
`a676dad3b2b4ceb9683db47243ce133a42ee64df` sorts valid `published_at`
values before manifest discovery, with stable ties and unknown missing/invalid
dates. The failing old-first regression now selects the newer checksum;
compatibility, CUDA preference, offline, retry and cancellation checks pass.

Three normal managed Models invocations then confirmed the new publication:
positive English exit 0 (1.36-second WAV), explicit one-frame limit exit 1
(no WAV), and a subsequent English invocation exit 0 (2.48-second WAV).
The failure is `MODEL_BACKEND_FAILURE`, message
`TTS generation limit reached without EOS`, family `INTERNAL_SERVER_ERROR`.
All three separate native hosts had executable SHA-256
`6aabc6940bd104bb889c5ae0be5d5f5e14db81b12eb9d1704b15d175c0011c48`
and DLL SHA-256
`f71fcb7294c2f7219f0a2e87aa6ee9d87eac1555bdc957335db6712dcdb4362e`,
matching the final package. Their installed provenance names `dea7691547a1`.
The cached selected archive reproduces 445,867,793 bytes and SHA-256
`58fe20ab56dcf6f4e4e485d71138bfa2c24605c1823817a763617199c9172422`.
No override, configuration change, or weight/cache deletion was used. These
separate-process calls do not prove same-host recovery; that remains the earlier
direct native proof. All owned hosts exited. Raw identity/results are in
`C:/t/dub-qwen-tts-sampler-public-confirmed-proof/verification.json`.

CI run `36988478683` for exact head
`a676dad3b2b4ceb9683db47243ce133a42ee64df` completed with all checks passing,
including required Backend Lint and Verification Policy. Functional coverage
measured 554 packages, gated 462, and reported zero gate failures; logging was
70.7% against the unchanged 69.7% floor. Unit evaluation completed across 483
packages. This is evidence for that head; later commits require their own CI.

### English quality probe canceled before synthesis

The fresh installed a676 CLI (SHA-256
`730b93bc0dc5d638ebf8c890a2ecf4f939124c59b3edec63ec9ed8821ba38207`)
completed 59-cue ASR and an initial translation candidate, then was canceled
while the second global comparison was running. Cue 32 changed from
`You're really unfamiliar with people.` to the auditor's proposed
`Again, it's particularly unfamiliar (with people/in this setting).`.
The raw ASR is `又特别又不熟。`; neither it nor neighboring cues supplies the
parenthetical's referents. The slash-separated alternatives are translator
commentary rather than natural spoken dialogue. Valid JSON and a constrained
decision do not establish semantic quality. Earlier variations are historical
outputs, not approved source corrections.

Cancellation occurred at 09:30:53 UTC through the owned CLI session. Preserved
provenance records exit 1, no primary result or CLI terminal envelope, no final
video, and no remaining owned processes. The run did not reach TTS. Its recording
and artifacts remain under
`C:/t/dub-multilingual-validation/factory-english-published-order-recording.json`
and `chinese-english-published-order-dub-8h3mjx1o/`. This is a canceled quality
probe, not a delivered video or an unclassified native synthesis failure.

Independent CPU frame review preserved 20 timestamped images and source/frame
hashes in `C:/t/dub-multilingual-validation/source-caption-review/frame-evidence.json`.
Visible captions conflict with noisy raw ASR:

| Cue / frame | Raw ASR | Independently visible Chinese caption |
| --- | --- | --- |
| 25 / 107.160 s | 车路贵， | 又那么贵 |
| 27 / 110.600 s | 我为什么家？ | 还回什么家啊 |
| 32 / 126.960 s | 又特别又不熟。 | 你这提议不错 |
| 52 / 207.600 s | 不刷我啊，兄弟， | 不要耍我兄弟 真的假的 |
| 57 / 234.600 s | 爸爸，好，什么东西刚盯我的脸，那么痛啊！ | 刚叮我的脸那么痛啊 |

Cue 32's caption is positive; no sampled frame establishes the proposed phonetic
replacement `又特别又不俗`. Cue 57's speaker rubs his cheek. Captions may span
neighboring cues or summarize speech, so they are independent review evidence,
not automatic canonical ASR replacements. No production text was substituted.

### Default Gemma probes and bounded larger-Qwen comparison

Seven sequential public managed OMNI calls used the default Gemma-4-E4B-it
Q4_K_M plus verified mmproj-F16, without ASR/caption hints, grammar, output-token
caps, configuration changes, or model switches. All CLI calls exited zero;
semantic accuracy was insufficient for automatic transcript repair. Of five
caption-only probes, cues 25/27 matched exactly; cue 32 omitted/paraphrased,
cue 52 corrupted the Chinese and included unwanted English, and cue 57 changed
wording. Audio-only cue 32 returned `哎 / 你做給我做 / 怎麼`; combined
image/audio returned caption `你提議不錯` and audio
`Pizza 給你吃不說 / 怎麼啊`. These do not establish accurate speech understanding.
The audio-only and combined calls took 69.734 and 83.312 seconds including
managed startup/load. Saved commands, responses, usage and provenance are under
`C:/t/dub-gemma-caption-native-proof/`; all observed owned hosts exited.

Live modules confirmed CUDA/mtmd on the managed host despite the upstream
`llama-cpp-cpu-all` payload name; projector CPU placement is separately intended.
CPU routing inspection shows direct OMNI audio forwarding, with no wired ASR
fallback. Module presence is not an instruction-level audio-encoder trace.
A separate external cp1252 reporting error occurred after a successful saved
UTF-8 Models result; it is not an inference failure.

The larger-Qwen comparison then completed three harder frames with the same
unprimed full-frame inputs/prompts: cue 52 matched exactly (279.953 seconds),
cue 57 preserved the Chinese but copied `cctv12306.com` (188.828 seconds), and
cue 32 matched exactly (175.453 seconds). Raw responses matched 2/3; Chinese
caption characters matched 3/3, improving these three cases over Gemma. This is
not a five-frame comparison or speech-accuracy certification. Lifecycle timings
include startup/load. Whole-device sampling reached 23,825 MiB used and only
314 MiB free on RTX 4090; this is not exclusive allocation, general capacity,
131k-context, all-GPU placement, or MTP proof. All three owned hosts exited.
Commands, responses, usage, hashes, CUDA/mtmd mappings and sampling are under
`C:/t/dub-qwen-caption-native-proof/comparison.json` and `provenance.json`.
No authored model/default/configuration or weight changes were made. No automatic canonical
source repair or final model selection follows from these bounded measurements.
Full English video delivery remains incomplete; PR 2668 stays draft and
unmerged pending quality repair, complete artifact QA, and final-head CI.

### Optional ASR language conditioning and exact digital silence

The pinned native ASR implementation ignored its language argument. Commit
`5ac4233dcefe8def0bc6564d34d3d29564666242` adds optional language conditioning,
validated official tokenizer prefixes, BCP 47 aliases, and parsing that retains
the first generated speech tokens. Omitted language still selects automatic
detection. CPU GCC/MSVC helper tests, Go bridge tests with race detection,
patch application/reversal, and Windows PowerShell 5.1 idempotence/failure
propagation checks passed. This does not make a requested language an
independently detected language or establish noisy-speech accuracy.

The first rebuilt package exposed a semantic failure on five seconds of exact
digital silence: forced English, Chinese, and Cantonese generated `The.`,
`嗯。`, and `係。`, respectively. That package was not published. Commit
`46520e803d8e7d22672b1b9b99c9b2f2de177a79` returns an empty result when a
nonempty decoded sample buffer consists entirely of exact zero values, after
validating an explicit language hint. It adds no amplitude threshold; quiet
nonzero speech, denormals, noise, nonfinite values, empty buffers, and existing
cancellation checks retain their established paths.

The replacement native package built in 23.763 seconds and repeated in 11.197
seconds with identical hashes for all 26 payload files. Independent review
verified 13 authored inputs, three patched native sources, pinned dependency
revisions, and payload hashes; the prior package remained unchanged. Its core
DLL SHA-256 is
`2e3e780d22bdc6f04d233d951d55c61acb0286070d6cef9d904f1ff68ac4146c`.
The live native ABI probe verified loaded module paths/hashes, empty forced
English/Chinese/Cantonese and automatic silence results, and unchanged known
English `This is.` including its first words. Cancellation pending before a
request was followed by successful recovery; mid-generation cancellation was
not tested. The owned process exited and released its native library/GPU.
Build evidence is in `C:/t/dub-asr-digital-silence-package/`; live evidence is in
`C:/t/dub-asr-digital-silence-native-proof/`. At this checkpoint publication and
canonical managed Models verification of the replacement have not run.

The repaired package was subsequently published as immutable release
`localai-backends-v1-58d1a323eb9679e5f19fa0cc48e0c9286e3448f0c34cf84519ba02dc7d62ad26`,
targeting authored source `46520e803d8e7d22672b1b9b99c9b2f2de177a79` without
marking it latest. A fresh public download matched the assembled archive:
441,956,666 bytes, SHA-256
`8d5236e6fb43b3d1d4da8a3630efd29fca583ba80315efc0a6a8d30a7a0ba769`;
downloaded manifest SHA-256
`13ecd0d03dfa5263ce37793ceda9c4da453abba6588bf5cdc74108dd2295c623`.
Independent archive review verified all 50 entries: 26 payload files, 22 proof
files, and two provenance/evidence documents. The previous failed silence and
cue-32 probes remain historical evidence; the new acceptance covers exact-zero
silence and known English speech, without claiming noisy-source quality repair.
The source ASR publication baseline now selects this archive while retaining
the pinned native, SDK and protocol identities. Normal public Models CLI
verification of the published replacement remains pending.

That managed public check subsequently passed four sequential default `asr`
CLI invocations using the clean `3ac900ddab81dd714886d9ec13955ed24fba848e`
binary and a fresh child-only home. Ordinary discovery downloaded the paired
model assets and selected the `58d1` publication without backend/configuration
overrides. The cached 441,956,666-byte archive independently hashes to
`8d5236e6fb43b3d1d4da8a3630efd29fca583ba80315efc0a6a8d30a7a0ba769`.
All four actual hosts' loaded executable, shim, core and CUDA module paths and
hashes match the published package. Public `en-US` recognition returns
`This is.` with segment 480–1040 ms; exact-zero WAVs with `en-US`, `zh-CN`,
and `yue-HK` each publish an empty transcript and `[]` segments. Every call
exits zero, and all observed CLI/native process identities are absent afterward.

Fresh startup took 98.203 seconds, including model/backend asset preparation.
The three later silence calls took 32.063, 31.062 and 27.594 seconds, including
their separate host lifecycles; these are not pure inference timings or
resident-host reuse proofs. Evidence is in
`C:/t/dub-asr-language-public-proof/`. Unsupported hints and mid-generation
cancellation were not exercised by these public calls. No noisy full-video
or accepted English dubbing output follows from this bounded proof.

### Larger Whisper comparison and malformed full-video timestamps

An isolated managed Whisper large-v3 comparison used the immutable
`ggerganov/whisper.cpp` asset revision
`5359861c739e955e79d9a303bcbc70fb988958b1`, with explicit Chinese recognition
and no translation or caption hints. Six bounded public ASR calls completed,
taking 44–52 seconds including lifecycle work. Some disputed words improved,
but cue 32 remained incorrect; these observations do not justify replacing
the Qwen default. Model, archive, process/module identities, inputs, outputs,
and teardown evidence are under `C:/t/dub-whisper-large-v3-proof/`.

The full original video then failed publicly with `MODEL_BACKEND_FAILURE` and
an invalid ASR segment after 81.219 seconds, without output transcript or
segment files. A separate raw RPC replay reconstructed the same public
request policy and preserved all 107 native segments. Three lexical segments
had equal start/end timestamps: IDs 41, 72, and 101. The failed public call's
staged WAV and raw response were not retained, so the replay is a cause witness
rather than proof of byte-identical responses. Its saved protobuf is a
serialized received message, not captured transport-frame bytes.

The raw full-video response also contained long repeated speech and concerning
source-timing disagreements. No words were dropped, timestamps invented, or
validation guards relaxed to accept it. A focused codec regression now asserts
that valid initial speech followed by a zero-duration lexical segment fails
atomically with no partial outputs. All trial-owned hosts exited. Larger models
remain candidates when they improve measured quality and fit available memory;
size alone does not select a default. English delivery and subsequent longer
Japanese/Korean/Spanish runs remain outstanding.

The latest native cue 32 comparison ran both Qwen3-ASR sizes through the
reviewed `46520e80` DLL using identical ±2s and ±5s WAVs; all eight calls exited
zero. Automatic and explicitly Chinese recognition results were identical for each model and
window. Neither size restored the visible positive `你这提议不错` proposal meaning
from the independent caption review; 1.7B returned `你吃的也不少，怎么？` at ±2s
and `小姐，你干嘛去了？怎么样？跟车贼有仇？怎么？嗯，看来。` at ±5s. These are
observed decoder outputs, not accuracy certification. Whole-device sampling at
250 ms reached 7,991 MiB used and 16,148 MiB free with 1.7B, and 5,471 MiB used
with 18,668 MiB free with 0.6B; these are whole-device snapshots, not exclusive
process allocations. Both owned PIDs exited and released their libraries.
Evidence: `C:/t/dub-qwen-asr-cue32-size-comparison/comparison.json`. No default
switch and no final delivery follows.

### Source-context diagnostic and independent-image packing

The larger supplied Qwen model received the same text-only source evidence,
comparison prompt, and decision grammar as the earlier diagnostic. Inputs
included timestamped caption/OCR observations and neighboring ASR cues, without
audio, image inputs, expected English hints, or new output/context caps.
Independent review checked the saved commands, responses, and evidence hashes
in `C:/t/dub-source-context-qwen-diagnostic/`.

Freeform comparison completed in 330.485 seconds. It recognized the positive
cue-32 proposal meaning and suggested `Your suggestion is good.` It left the
opening of cue 57 unresolved. Other classifications still require review; this
prose does not establish corrected acoustic transcripts or semantic acceptance.
The subsequent grammar-constrained decision failed after 227.406 seconds with
`MODEL_BACKEND_FAILURE`: the backend returned reasoning bytes but no text
output. No decision artifact was available for validation. The diagnostic
reported 15,087 prompt tokens and 1,155 generated tokens for that failed stage;
it showed no reported memory error. Whole-device sampling reached 23,621 MiB
used and 518 MiB free. All observed owned hosts exited. Useful freeform analysis
does not establish reliable structured decisions or justify a default switch.

A separate public image trial supplied three independent 1280×60 caption crops
in cue order 25, 32, 57. It completed in 264.125 seconds, but assigned the
second crop's caption to cue 25 and the third crop's caption to cue 32. It then
reported `NO_CAPTION` for cue 57 and invented an unrelated watermark. No named
cue matched its corresponding crop. Gemma's earlier same-crop trial retained
order but misread caption characters; these selected trials do not rank models
generally. Crop commands, transforms, responses, identities, and teardown are
in `C:/t/dub-qwen-crop-batch-proof/`.

CPU inspection found a concrete independent-image packing defect in the pinned
native path. The actual Qwen projector selects temporal merging of adjacent
equal-size images; native reconstruction appends the independent crops with
contiguous media markers. The saved tokenizer source therefore groups the first
two crops together, leaving two logical image groups for three independent
inputs. This is consistent with the observed attribution failure. No live merge
trace was captured, so it does not explain which merged image the model favored,
the watermark invention, or every recognition error.

A proposed counterfactual separates independent media with nonwhitespace text
boundaries while preserving deliberate within-video grouping. Plain-prompt
separators cannot interleave the native appended image parts. Whitespace alone
may be normalized by the actual chat template. A rebuilt, identified backend
must prove actual grouping and correspondence before accepting that repair.
The CPU trace is in `cpu-image-packing-review.md` beside the crop evidence.
These diagnostics do not produce an accepted English video or authorize source
ASR replacement; full dubbing quality and delivery remain incomplete.

The independent OpenCode engine review also timed out after its 300,000 ms
allowance without a primary result. Its saved error identifies session
`4d14464f-f822-449c-8a0a-38ba485bdae6`; cleanup closed the live Factory Session,
and partial effects remained possible. Root's workspace inspection found only
expected files. The timeout cause is unknown; this dispatch adds no review
evidence or harness stability proof.

### Later Windows verification and remaining delivery gates

The independent-image counterfactual is now built and published as a manual
Windows CUDA release from `6d1aede58898fcaf3466fc3becbc91639d395b97`:
[reviewed native package](https://github.com/portpowered/you-agent-factory/releases/tag/localai-backends-v1-8e455ae80c18ad5221a58be52d3d3a1a7bb651061fa6191353b64069d946d3ab).
Actual CPU grouping tests and a native GPU call accepted the boundary repair.
A subsequent normal public `you models invoke customqwen` call discovered that
release without a backend source override, verified the downloaded archive and
loaded executable/CUDA DLL identities, and returned the three caption literals
in the correct cue order. The normal call completed in 405.406 seconds including
asset preparation and exited cleanly. Evidence:
`C:/t/dub-qwen-independent-images-managed-proof/`.

The same output invented `EPISODE 01` as an overlay on cue 25. Consequently,
correct image grouping and these three subtitle readings do not establish
general recognition quality or acceptance of the complete English dub.
Raw source ASR remains preserved; unresolved cue interpretation and independent
final translation/dubbing review are still required.

The thinking repair now uses the pinned native chat template's schema-aware
grammar/parser path rather than a separate reasoning state machine. Linked CPU
tests exercise ordinary BPE thinking delimiters, misleading private JSON,
ordered translation tuples, audit variants, and final-content separation.
Go/native transport integration, capability detection that rejects older
backends before unconstrained inference, media observation followed by one
schema-constrained final answer, and a new identified native build remain in
progress. No production default disables thinking. CPU validation alone is not
live public structured-inference acceptance.

Required CI passed on `0a83cd7816` (run `37056730593`), but the later pushed
`e4dd9b69c4` failed run `37059462705` on ACP version classification and restart
cancellation recognition. The cancellation test repair is committed as
`1378e88d48`; its compiled Windows scenario passed with clean committed
`8d0225132e` and matching embedded VCS identity. ACP response/EOF and notification
ordering repairs still need their compiled public-boundary proof and CI.
The PR remains unmerged. Current-head CI, accepted full English output, and
postmerge long Japanese/Korean/Spanish runs remain delivery requirements.


### 2026-10-02 current release priority and verified gates

The user explicitly prioritizes a mostly working dubbing pipeline and merging,
accepting imperfect translation and pronunciation. Full valid video output with
reference-conditioned audio and final-head CI remain merge prerequisites;
exhaustive Japanese/Korean/Spanish long-video QA and a public IndexTTS path stay
post-merge follow-ups. PR 2668 remains draft and unmerged.

The Factory migrated from raw final grammar to `json_schema` at translation,
audit, and fit repair. Thinking stays enabled privately, and the final public
structured response omits reasoning/thinking keys. The packaged Python suite
passes 74 tests, and the canonical catalog check passes.

The native Windows CUDA schema release
`localai-backends-v1-90eb06d7fb9a54cb8ae9ca83172ca599cbd1252d500446dedbf8400a9405add5`
is published. A normal full Factory run downloaded the 443,212,798-byte archive
with SHA-256
`272233b21ce0d353b10bfdd31dffb5d37bfc21a55a2efee5dd57490be34a2c19` and a loaded
executable SHA-256
`8ece891bfd64a1526a58a22cd4e82bf8bab572aaaea4f8372a03fb33c2e7c475`. A
prerelease public Models fixture proved 855 private reasoning bytes separated
from the final schema JSON; it returned English text, so it is representation
separation evidence, not Korean translation quality.

The normal CLI installed immutable module `b050`. Its compiled three-row ACP EOF
proof passed in 6.746 seconds and a fresh MCP edit completed in 33.954 seconds
under session `96929d18-8a1e-46c3-90ed-8ce92a00b1f5`. Source head
`8e1daf1b861f8341c83512a22c1d1d0d5ac611bc` fixes two stale controlled-clock tests
left behind when the readiness default became five minutes; focused race and the
full owning package pass, with no production, assertion, or wall-clock limit
change. CI run `37078759573` was still running with no failures at observation;
the preceding b050 CI failed exactly those tests and cascaded into Verification
Policy.

The full original 240-second Chinese video to en-US runs against the supplied
`customqwen` model and default Qwen ASR/reference-audio TTS, with no overrides,
thinking disable, or token caps. It produced 59 source cues and a valid initial
translation; the audit applied three indexed corrections and its second audit is
active. No final accepted video exists yet. Model cold unload/reload and
multi-minute audit inference are observed operational latency limitations, and
causes beyond the measured timings remain unproven.

Evidence roots: `C:/t/dub-schema-english-end-to-end`,
`C:/t/dub-ci-b050-review`,
`C:/t/dub-multilingual-validation/installed-versioned-b050-proof`, and
`C:/t/dub-schema-small-public-candidate-proof`.

### Remaining broader MCP/GPU goal evidence (ad128)

This inventory records the broader goal without adding prerequisites to the first dubbing release. Evidence is tied to committed `ad1280a001fe30799feee77bb88a5a2ed348f329`; active ACP concurrency edits are unaccepted work, not part of that proof.

- **Opener and injection:** `pkg/root/process.go` uses the canonical Wire graph. `pkg/wire/boundary_test.go` and `pkg/initializer/runtimeapplication/bundle_boundary_test.go` prohibit retired runtime builders and secondary composition. No application-opener symbols were found in committed `pkg` or `cmd`; this is targeted enforcement, not an exhaustive claim that all historical abstractions are gone.
- **Lint and deadcode:** CI run `37081425137` completed successfully. Backend Lint job `111082586453` ran the complete canonical inventory, including the matching deadcode ratchet. `docs/internal/baselines/deadcode-baseline.txt` still contains 3148 findings; passing enforcement does not mean zero deadcode debt. Full log: `C:/t/dub-mcp-correlated-concurrency-proof/backend-lint-ad128.log`.
- **Windows ASR and TTS:** `C:/t/dub-asr-language-public-proof/acceptance-summary.json` records public default Qwen CUDA speech/silence checks for en-US, zh-CN, and yue-HK with owned teardown. `C:/t/dub-qwen-tts-sampler-public-confirmed-proof/verification.json` records the actual published native identity, reference-audio synthesis, explicit cap exhaustion without a WAV, and recovery on separate hosts. These fixtures do not establish noisy long-video transcription accuracy or voice similarity.
- **Windows embeddings:** Normal public `models invoke embed --operation EMBED` succeeded in 21.406 seconds with 1024 finite values and norm approximately 1. The live host loaded `ggml-cuda.dll` and exited. Evidence: `C:/t/dub-embed-public-proof/result.json` and `live.json`. This used the older native revision3 package; embeddings have not been separately replayed against the latest schema archive.
- **LLM and language quality:** Multimodal image/audio probes are retained under `C:/t/dub-gemma-caption-native-proof`. `C:/t/dub-schema-small-public-candidate-proof/acceptance-summary.json` proves schema shape and private-reasoning separation, but the Korean fixture returned English. File routing and structured representation are distinct from caption accuracy, translation quality, and long Japanese/Korean/Spanish delivery acceptance.
- **Public IndexTTS:** Private Windows CUDA reference-audio gRPC and existing production-codec interoperability succeeded, retained in `C:/t/dub-indextts-localai-grpc-proof/b224-interop-summary.json`. Its `acceptance-summary.json` records English/Chinese ASR roundtrips differing from intended text and no voice-similarity acceptance. Normal public Index discovery/invocation remains deferred; the six-file draft is preserved in `deferred-index-public-integration.patch` in that proof root.
- **MCP concurrency:** The isolated immutable ad128 artifact completed two calls through one stdio endpoint, with exact final edits and typed private-safe errors retaining actual runtime IDs. `C:/t/dub-mcp-correlated-concurrency-proof/proof-ad128-telemetry.json` records two live Factory Sessions and one shared OpenCode process chain. The baseline `daemon.execute` holds its per-provider gate for the whole attempt, so same-provider execution is serialized. No simultaneous provider-prompt or model-compute claim is made. The telemetry decoder omitted live dispatch reads, and current concurrency repair is not yet accepted.
