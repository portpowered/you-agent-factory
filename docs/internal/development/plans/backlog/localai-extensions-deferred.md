# LocalAI extensions — deferred scope

Status: backlog (iterate before implementation)  
Date: 2026-08-20  
Audience: Models, Workers, Factory Definitions, Work, transports, UI, and
functional-test maintainers

This document holds the remainder of the original "Run local AI with YOU"
program after `docs/internal/development/plans/localai-extensions.md` was
narrowed to the priority slice in `problems.md` (ASR, TTS on VibeVoice-7B,
gemma-e4b omni text + media understanding, embeddings, CLI-first plus the
factory TTS switch). Nothing here is scheduled; each section must be
re-shaped into its own standards-conformant plan (per
`docs/internal/standards/code/planning-standards.md`) before
implementation, using the contracts the priority slice lands as its
baseline.

## What the priority slice already establishes

These are settled decisions the deferred work builds on, not open
questions:

- Joined `InvokeModel` (resolve → download → load → invoke → output →
  release) is the canonical invocation path for direct transports and for
  Workers' inference execution; `InvokeModelWithLease` remains the
  low-level prepared primitive.
- Model names resolve operator configuration first, then built-in defaults
  (`llm`, `asr`, `tts`, `embed`), then path/`hf://` source forms; the
  operator-config `models:` block and its OpenAPI configuration schema
  exist and are documented.
- `OMNI` is the single text-and-media-understanding operation (text
  prompt, repeatable images, audio, video); `EMBED`, `TTS`, and `ASR`
  complete the shipped slot-named contracts. Repeatable input slots are a
  contract feature.
- Two-adapter LocalAI decision: managed pinned-gRPC backends by default;
  no full LocalAI HTTP server for the managed path; no gallery source
  type; no LocalAI types across the Models boundary.
- Source forms (local paths, `file://`, `hf://…@revision`), HF cache
  order, atomic digest-verified downloads, separate model/backend caches.
- Generic CLI grammar (`--operation/--input/--param/--output/--json/
  --offline`), stdout/multi-output rules, `@` media-type detection.
- Pinned backend protocol + CI-built platform artifacts with checksum
  manifest; pin updates ride dependency PRs gated on conformance.
- Factory TTS already runs on the joined path with the `tts` (vibevoice)
  builtin; the packaged TTS factory is migrated; the OmniVoice specialized
  runtime and its schemas are deleted (`D1`). There is no OmniVoice path
  left to retire here.

## Deferred work set

### 1. Remaining bounded operation families

Extend the operation catalog and codecs; each family is its own vertical
lane (CLI + HTTP + docs + functional story + conformance cell), following
the priority slice's lane shape.

| Group | Operations |
| --- | --- |
| Text/retrieval extras | `RERANK`, `MODERATE`, `TOKENIZE`, `DETOKENIZE` |
| Generated audio | `SOUND_GENERATE` |
| Audio analysis | `DIARIZE`, `AUDIO_CLASSIFY`, `VAD`, `SPEAKER_EMBED`, `SPEAKER_VERIFY`, `SPEAKER_ANALYZE` |
| Audio transform | `AUDIO_TRANSFORM` (denoise, separation, voice conversion; named stem outputs) |
| Image generation | `IMAGE_GENERATE` (text-to-image, image-to-image, inpaint, control), `IMAGE_UPSCALE` |
| Detection and SAM | `OBJECT_DETECT`, `SEGMENT` (point/box/text prompts) |
| Spatial vision | `DEPTH_ESTIMATE` (depth, geometry, optional point cloud) |
| Face | `FACE_DETECT`, `FACE_EMBED`, `FACE_VERIFY`, `FACE_ANALYZE` |
| Video and 3D | `VIDEO_GENERATE`, `MESH_GENERATE`, `MESH_REMESH` (GLB as `BINARY`, `model/gltf-binary`) |

Full input/output slot tables for these operations live in the archived
original program document,
`docs/internal/development/plans/archive/08-20/localai-extensions-full-program.md`;
lift them from there when planning each family. Where that document names
`TEXT_GENERATE`/`VISION`, the shipped equivalent is `OMNI`.

If the priority slice's llama.cpp modality validation narrowed `OMNI` to
text + image inputs, audio/video understanding re-enters here as its own
lane.

### 2. Factory graph generalization

The priority slice migrates only the existing TTS path onto the joined
kernel. Making local inference a first-class Factory citizen for every
family remains deferred:

- Generic `INFERENCE_RUN` Workstations with `operationBindings` (slot
  selectors and config values), named Work outputs, and `onFailure`
  routing with provider-neutral failure classification for non-TTS
  operations.
- `INFERENCE_WORKER` declares only `type` and `model` for any operation;
  operations resolve from operator configuration or built-in defaults;
  `operations` becomes optional override input; deprecate and stop
  requiring `modelLocality`.
- Resources become capacity-only: remove `Model`, `Backend`, and
  `LoadPolicy` from `MODEL` Resources (`models.RuntimeResource` keeps
  `ID`, `Name`, `Type`, `Capacity`). Migration decodes old fields long
  enough to emit a targeted error/warning; it never builds a second
  effective model definition from them.
- Semantic operation-binding validation runs when a Factory Session
  resolves the Worker's model; structural graph validation stays
  network-free.
- `VIDEO` becomes a first-class `WorkContent` type; named multi-artifact
  lineage is preserved through Work, events, and replay.
- Dispatch stays claimed while assets prepare; preparation progress is
  model/runtime observations, never fake Work states.
- UI renders schema-derived slots, readiness progress, and named
  artifacts.
- Factory Runtime, Sessions, and Recordings gain no new service methods;
  canonical events stay provider-neutral.

### 3. Attached `LOCALAI_HTTP` adapter

Optional compatibility adapter for an operator-supplied, already-running
LocalAI server, behind the same Models adapter contract and codecs. Its
own test lane starts a full LocalAI server; a passing HTTP test is never
required for the managed gRPC path.

### 4. Conformance expansion

Extend the scheduled real-backend lane to every newly shipped family and
platform, keeping the publish-only-metadata rule (backend version, model
revision, operation, hardware class, duration, result digest).

## Still explicitly out of any LocalAI program

Unchanged from the original program's deferral table — these need their
own programs with durability/privacy/session policy and are not inference
work: face/speaker identity registries (register/identify/forget),
voice-profile create/delete, realtime speech-to-speech and streaming
transforms, background response jobs, fine-tuning and quantization, and
LocalAI agents/MCP/stores.

## Planning notes for whoever picks this up

- Sequence: section 1 families are independent of each other but all sit
  on the priority slice's kernel; section 2 is required before any new
  family runs in a Factory graph; sections 3 and 4 are independent.
- Each future plan must state its own functional-test cells, customer
  interfaces, docs surface, package changes, service interactions, failure
  modes, and the standard implementation-stage delivery criterion — do not
  inherit them implicitly from this backlog note.
- Contract changes (OpenAPI invocation and configuration schemas,
  generated clients, `WorkContent`) each need a single owning lane,
  mirroring the priority slice's shared-surface rule.
