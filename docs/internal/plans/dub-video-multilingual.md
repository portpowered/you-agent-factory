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
