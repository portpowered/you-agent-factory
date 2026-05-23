# PRD: Model Resource Operation Contracts

## Introduction

Add a resource-backed model operation contract to the agent factory so workstations can declare generic operations such as `TTS`, workers can declare which operation inputs and outputs they support, and the runtime can execute the operation through either local embedded models or cloud providers. This solves the current gap where model execution is mostly prompt-oriented and provider-specific, while future local inference features need multimodal inputs, multimodal outputs, schedulable model resources, and clear compatibility between workstation intent and worker capability.

In version 1, the feature should support text-to-speech through an `OMNIVOICE_Q4_K_M` local model worker while keeping the contract generic enough for future `ASR`, `TRANSCRIBE`, `EMBED`, `CLASSIFY`, and cloud-backed workers. All public enum-like values introduced by this feature must use uppercase string values.

## Goals

- Let workstations declare a generic model operation contract such as `TTS` without knowing provider-specific fields.
- Let workers declare supported operations, input slots, output slots, provider locality, and resource requirements.
- Model local and cloud model capacity as resources that can be scheduled and bounded across factories.
- Reuse the existing `WorkContent` shape for multimodal input and output instead of introducing a parallel payload envelope.
- Support configurable input bindings so a slot can come from submitted work content, workstation config, worker config, or a default value.
- Preserve event, replay, API, and diagnostics evidence for resource acquisition, model loading, invocation, output production, and failure.
- Expose model discovery and invocation through an explicit `/models` API surface.
- Validate real local inference on macOS, Windows, and Linux through long-running CI coverage.

## User Stories

### US-001: Author a workstation with a generic model operation
**Description:** As a factory author, I want a workstation to declare `type: "MODEL_INVOKE"` and `operation: "TTS"` so that the workstation describes what needs to happen without hardcoding the model provider.

**Acceptance Criteria:**
- [ ] A workstation can declare `type: "MODEL_INVOKE"` in public factory config.
- [ ] A workstation can declare an uppercase operation value such as `operation: "TTS"`.
- [ ] `MODEL_INVOKE` workstations must reference a worker that supports the requested operation.
- [ ] Existing workstation types such as `MODEL_WORKSTATION` and `LOGICAL_MOVE` remain backward compatible.
- [ ] Validation errors explain whether a failure is caused by workstation type, operation, content contract, or worker compatibility.
- [ ] Typecheck, lint, generated-artifact checks, and relevant backend tests pass.

### US-002: Declare worker operation capabilities
**Description:** As a factory author, I want a worker to declare which operation inputs and outputs it supports so that the runtime can validate compatibility before dispatch.

**Acceptance Criteria:**
- [ ] A `MODEL_WORKER` can declare one or more supported operations.
- [ ] Each operation declaration includes an uppercase `name`, such as `TTS`.
- [ ] Each operation can declare supported input slots with uppercase content types such as `TEXT`, `AUDIO`, `IMAGE`, `JSON`, or `BINARY`.
- [ ] Each operation can declare produced output slots with uppercase content types.
- [ ] Worker validation rejects duplicate operation names on the same worker.
- [ ] Worker validation rejects duplicate slot names within one operation direction.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-003: Reuse WorkContent for multimodal input and output
**Description:** As an API user, I want model invocations to accept and return the same ordered content shape used by work items so that multimodal data has one canonical representation.

**Acceptance Criteria:**
- [ ] The invocation input contract uses `WorkContent` or a direct extension of `WorkContent`.
- [ ] The invocation output contract uses `WorkContent` or a direct extension of `WorkContent`.
- [ ] Existing `text` and `image` content parts remain valid for existing work submission APIs.
- [ ] New multimodal parts support uppercase public type values while preserving compatibility with existing lowercase content values where already published.
- [ ] Content parts can include optional `label`, `role`, `contentType`, `artifactId`, and `metadata` fields.
- [ ] New model-operation content parts use uppercase `type` values such as `TEXT`, `IMAGE`, `AUDIO`, `JSON`, and `BINARY`.
- [ ] Audio output can be returned through a response stream rather than inline JSON payload.
- [ ] Typecheck, lint, generated-artifact checks, and relevant API contract tests pass.

### US-004: Bind workstation inputs from content or configuration
**Description:** As a factory author, I want operation slots to come from submitted content, static configuration, or a default so that simple factories can hardcode voice settings while advanced factories can supply them dynamically.

**Acceptance Criteria:**
- [ ] A workstation operation contract can bind a slot from runtime input content.
- [ ] A workstation operation contract can bind a slot from static config content.
- [ ] A workstation operation contract can use config content as a default when runtime input is absent.
- [ ] Bindings can select input content by slot, label, type, or role.
- [ ] Binding resolution records whether each slot came from `INPUT`, `CONFIG`, `DEFAULT`, or `OMITTED`.
- [ ] Validation rejects required slots that cannot be resolved.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-005: Execute the same operation through local or cloud workers
**Description:** As a factory author, I want the same `TTS` workstation contract to work with either an embedded local `OMNIVOICE_Q4_K_M` worker or a cloud-backed TTS worker.

**Acceptance Criteria:**
- [ ] Worker declarations can identify provider locality using uppercase values such as `LOCAL` or `CLOUD`.
- [ ] A local worker can declare `modelProvider: "LOCAL"` and `model: "OMNIVOICE_Q4_K_M"`.
- [ ] A cloud worker can declare a cloud provider and model while supporting the same generic `TTS` operation.
- [ ] Workstation compatibility validation depends on operation and content contract, not provider locality alone.
- [ ] Provider-specific slot names are hidden behind worker bindings.
- [ ] Typecheck, lint, and relevant service-level tests pass.

### US-006: Model inference capacity as resources
**Description:** As a runtime operator, I want model capacity, provider quotas, and invocation slots to be schedulable resources so that multiple factories cannot overrun local hardware or cloud limits.

**Acceptance Criteria:**
- [ ] Factory config can declare model-related resources with uppercase resource type values such as `MODEL`, `PROVIDER_QUOTA`, or `INVOCATION_SLOT`.
- [ ] Local model resources can describe explicit model name, backend, and load policy.
- [ ] Cloud model resources can describe provider, model identity, and quota capacity.
- [ ] Workstations and workers can both declare resource requirements for model-backed execution.
- [ ] The runtime enforces factory-level resource requirements before invoking the worker.
- [ ] The service enforces process-level local model capacity across concurrently running factories.
- [ ] Typecheck, lint, and concurrency-sensitive backend tests pass.

### US-007: Load and invoke OmniVoice locally
**Description:** As a customer, I want to run `OMNIVOICE_Q4_K_M` locally for `TTS` without managing Python dependencies so that portable factories can synthesize speech through the binary-managed runtime.

**Acceptance Criteria:**
- [ ] The runtime includes an `OMNIVOICE` local worker implementation behind the generic worker capability interface.
- [ ] The implementation can load GGUF/GGML model assets from a managed local model cache.
- [ ] Missing model assets produce a clear `MODEL_NOT_AVAILABLE` failure.
- [ ] The first invocation can load the model on demand when load policy allows it.
- [ ] Repeated invocations reuse the loaded model handle when resource keys match.
- [ ] The local worker emits audio output as either a streamed response for direct API calls or a `WorkContent` audio output part for factory-level execution.
- [ ] Typecheck, lint, and relevant local-runtime adapter tests pass.

### US-008: Expose resource-backed operation invocation through the API and CLI
**Description:** As a developer, I want to list resources and invoke operations directly so that I can test model workers outside a full factory run.

**Acceptance Criteria:**
- [ ] The API exposes `GET /models` for model discovery.
- [ ] The API exposes `GET /models/{model_name}` for one model's status, capabilities, supported operations, load state, and resource metadata.
- [ ] The API exposes `POST /models/{model_name}/invocations` for direct model invocation.
- [ ] `POST /models/{model_name}/invocations` accepts operation name, `WorkContent` input, binding options, and invocation options.
- [ ] The invocation response can return JSON metadata and a streamed audio body for audio-producing operations.
- [ ] The CLI can list model resources and invoke `TTS` against a running service.
- [ ] CLI output can write audio artifacts to a file path.
- [ ] Error messages distinguish missing resources, incompatible worker capabilities, invalid content, and provider execution failures.
- [ ] Typecheck, lint, generated-artifact checks, and CLI tests pass.

### US-009: Record model resource and invocation events
**Description:** As a user inspecting runtime history, I want model loading, resource waiting, invocation, and output evidence to appear in events so I can debug local and cloud model behavior.

**Acceptance Criteria:**
- [ ] Event history records resource wait and acquisition for model-backed invocations.
- [ ] Event history records local model load request and response when model loading occurs.
- [ ] Event history records model invocation request and response with operation, worker, resource, and slot binding evidence.
- [ ] Events preserve as much request and response detail as practical for debugging, including raw text content and response metadata.
- [ ] Events store large binary audio through artifact references, stream metadata, or bounded previews rather than unbounded inline event payloads.
- [ ] Replay preserves enough model invocation evidence to reconstruct user-visible runtime history.
- [ ] Typecheck, lint, replay tests, and event contract tests pass.

### US-010: Validate local inference in tests
**Description:** As a maintainer, I want automated tests to prove that local inference actually loads and invokes a local model so that the feature does not silently degrade into mocked-only behavior.

**Acceptance Criteria:**
- [ ] Unit tests cover model registry lookup, model load planning, binding resolution, and invocation request validation without requiring real model assets.
- [ ] Integration tests cover a fake local model runtime through the same manager and API paths used by real local inference.
- [ ] At least one long test invokes a real local `OMNIVOICE_Q4_K_M` model asset and validates that non-empty audio is produced.
- [ ] The real local inference test verifies output content type, operation metadata, and basic audio integrity.
- [ ] Long tests run locally through `make long-tests`.
- [ ] `make long-tests` is the documented entrypoint used by CI and local development.
- [ ] Long tests fail clearly when model assets cannot be pulled, loaded, or invoked.

### US-011: Run local invocation tests in GitHub Actions
**Description:** As a maintainer, I want long-running local inference tests in CI on macOS, Windows, and Linux so that binary packaging and platform-specific loading behavior stay healthy.

**Acceptance Criteria:**
- [ ] GitHub Actions includes a long-test workflow or job matrix for `macos`, `windows`, and `linux`.
- [ ] Each GitHub Actions long-test job runs `make long-tests`.
- [ ] The CI workflow can pull or restore the required `OMNIVOICE_Q4_K_M` model assets before running real local inference tests.
- [ ] The CI workflow caches model assets when safe to reduce repeated download time.
- [ ] The workflow runs direct `/models/{model_name}/invocations` coverage on each platform.
- [ ] The workflow runs at least one factory-level `MODEL_INVOKE` path using the same local model worker on each platform.
- [ ] The workflow is allowed to be slower than ordinary PR tests and is documented as a long lane.
- [ ] Failures include platform, backend, model asset, and invocation diagnostics.

### US-012: Pull local model assets
**Description:** As a user, I want a supported way to pull local model assets so that missing `OMNIVOICE_Q4_K_M` files can be resolved without manual cache setup.

**Acceptance Criteria:**
- [ ] The API or CLI supports a `PULL` action for local model assets.
- [ ] `PULL` can fetch the required `OMNIVOICE_Q4_K_M` assets into the managed local model cache.
- [ ] Model pull reports progress, final cache location, model revision, and downloaded files.
- [ ] Failed pulls produce actionable errors for network, checksum, disk, and unsupported-platform failures.
- [ ] Invocation still returns `MODEL_NOT_AVAILABLE` when assets are missing and pull was not requested or fails.

### US-013: Document operation contracts and authoring patterns
**Description:** As a factory author, I want clear documentation for model operation workstations and workers so that I can author local and cloud TTS flows without reading implementation code.

**Acceptance Criteria:**
- [ ] Workstation docs explain `MODEL_INVOKE`, uppercase `operation` values, input contracts, output contracts, and bindings.
- [ ] Worker docs explain operation capability declarations, local/cloud providers, and provider-specific bindings.
- [ ] Resource docs explain model resources, provider quota resources, and cross-factory local resource boundaries.
- [ ] Docs include one local `OMNIVOICE_Q4_K_M` `TTS` example.
- [ ] Docs include one cloud-backed `TTS` example using the same workstation contract.
- [ ] Docs call out the compatibility rules between workstation type, worker type, operation, and content contracts.

## Functional Requirements

1. FR-1: The system must support a public workstation type value named `MODEL_INVOKE`.
2. FR-2: `MODEL_INVOKE` workstations must declare or inherit an uppercase operation value such as `TTS`.
3. FR-3: The system must support uppercase operation values, with `TTS` required for v1.
4. FR-4: The system must validate that a `MODEL_INVOKE` workstation references a compatible `MODEL_WORKER`.
5. FR-5: Worker compatibility must require the worker to declare the workstation operation in its supported operations.
6. FR-6: Worker compatibility must validate that workstation-required input slots can be satisfied by worker-supported input slots.
7. FR-7: Worker compatibility must validate that worker-produced outputs satisfy workstation-required output slots.
8. FR-8: Operation input and output contracts must use `WorkContent` or a backward-compatible extension of `WorkContent`.
9. FR-9: `WorkContent` must support optional `label`, `role`, `contentType`, `artifactId`, and `metadata` fields for model operation use cases.
10. FR-10: `WorkContent` must add an audio-capable content part for `TTS` input and output.
11. FR-11: Existing text and image content submission behavior must remain backward compatible.
12. FR-12: Slot bindings must support resolving values from `INPUT`, `CONFIG`, `DEFAULT`, and optional omission.
13. FR-13: Slot binding selectors must support matching by slot name, content label, content type, or role.
14. FR-14: Slot binding resolution must be deterministic when multiple content parts match the same selector.
15. FR-15: Validation must reject required operation slots that cannot be resolved before invocation.
16. FR-16: Workers must be able to declare provider locality using uppercase values such as `LOCAL` and `CLOUD`.
17. FR-17: A local `OMNIVOICE_Q4_K_M` worker must support the `TTS` operation in v1.
18. FR-18: The local `OMNIVOICE_Q4_K_M` worker must load model assets from a managed local cache rather than requiring Python runtime dependencies.
19. FR-19: Local model loading must be lifecycle-managed with load, reuse, release, and idle-unload behavior.
20. FR-20: Model-backed execution must use resource scheduling for both workstation-level and worker/model-level requirements.
21. FR-21: The service must enforce local model process resources across all live factory sessions.
22. FR-22: The API must expose model discovery through `GET /models`.
23. FR-23: The API must expose one model's status and capabilities through `GET /models/{model_name}`.
24. FR-24: The API must expose direct model invocation through `POST /models/{model_name}/invocations`.
25. FR-25: `POST /models/{model_name}/invocations` must accept uppercase operation values, `WorkContent` input, binding options, and invocation options.
26. FR-26: Model invocation responses must return output content, diagnostics, and streamed output support for audio-producing operations.
27. FR-27: The CLI must provide a way to list models, pull local model assets, and invoke `TTS` through the API.
28. FR-28: Events must record model resource wait, model load, invocation request, invocation response, binding resolution, output artifact or stream metadata, and failure diagnostics.
29. FR-29: Event payloads must preserve raw text request and response data when practical.
30. FR-30: Event payloads must avoid unbounded raw binary audio and instead store artifact references, stream metadata, or bounded previews for large audio.
31. FR-31: Documentation must define uppercase enum values, compatibility rules, resource modeling, local/cloud authoring examples, `/models` API usage, and long-test expectations.
32. FR-32: The test suite must include unit and integration coverage for local model registry, loading, binding, invocation, and API contract behavior.
33. FR-33: A `make long-tests` lane must validate real local `OMNIVOICE_Q4_K_M` invocation.
34. FR-34: GitHub Actions must include a macOS, Windows, and Linux long-test matrix for real local invocation.
35. FR-35: Every GitHub Actions long-test matrix job must run `make long-tests`.
36. FR-36: Long-test failures must include enough diagnostics to distinguish asset download failure, load failure, invocation failure, output validation failure, and platform/backend failure.

## Non-Goals

- Supporting every local inference model in v1.
- Supporting local ASR, STT, embeddings, image generation, or video generation in v1.
- Embedding model weight files directly into the compiled binary in v1.
- Replacing existing prompt-oriented `MODEL_WORKER` behavior for current Codex/Claude-style workers.
- Designing a complete model marketplace or download manager in the first implementation slice.
- Streaming audio input in v1.
- Real-time low-latency audio chunking beyond returning a completed audio response stream.
- Browser-based model execution.
- Real-time voice conversation, barge-in, or duplex speech behavior.
- A new content system separate from `WorkContent`.

## Design Considerations

- Workstations should describe generic production intent, such as `TTS`, and should avoid provider-specific field names.
- Workers should hide provider-specific slot names behind bindings so a local worker and cloud worker can satisfy the same workstation contract.
- Resource declarations should make model capacity visible in the same mental model as existing factory capacity.
- Factory authors should be able to build a simple TTS station with only dynamic text and hardcoded voice settings.
- Advanced authors should be able to override voice reference, style, language, or output options through submitted `WorkContent`.
- Audio outputs should be retrievable through response streaming for direct `/models/{model_name}/invocations` calls.
- Factory-level audio outputs should still be representable as `WorkContent` output parts with enough metadata to retrieve or inspect the produced audio.
- Uppercase public enum values should be used consistently for new types, operations, resource types, binding source values, provider locality, and content type declarations.

## API Contract

The v1 public model API should use `/models` as the direct model surface. Models remain schedulable resources internally, but clients should not need to know resource topology just to discover or invoke a model. Concrete variants must be represented as explicit model names, not a separate `variant` selector. For example, use `OMNIVOICE_Q4_K_M` as the model name instead of `model=OMNIVOICE` plus `variant=Q4_K_M`.

### List Models

```http
GET /models
```

Response:

```json
{
  "models": [
    {
      "name": "OMNIVOICE_Q4_K_M",
      "provider": "LOCAL",
      "status": "AVAILABLE",
      "loadState": "UNLOADED",
      "operations": ["TTS"],
      "modalities": ["TEXT", "AUDIO"],
      "resources": [
        { "name": "OMNIVOICE_Q4_K_M_MODEL", "type": "MODEL", "capacity": 1 }
      ]
    }
  ]
}
```

### Get Model

```http
GET /models/{model_name}
```

Response:

```json
{
  "name": "OMNIVOICE_Q4_K_M",
  "provider": "LOCAL",
  "status": "AVAILABLE",
  "loadState": "LOADED",
  "operations": [
    {
      "name": "TTS",
      "inputs": [
        { "slot": "TEXT", "type": "TEXT", "required": true },
        { "slot": "VOICE_REFERENCE", "type": "AUDIO", "required": false },
        { "slot": "STYLE", "type": "TEXT", "required": false }
      ],
      "outputs": [
        { "slot": "SPEECH", "type": "AUDIO", "required": true }
      ]
    }
  ],
  "resources": [
    { "name": "OMNIVOICE_Q4_K_M_MODEL", "type": "MODEL", "capacity": 1 }
  ],
  "diagnostics": {
    "backend": "METAL"
  }
}
```

### Invoke Model

```http
POST /models/{model_name}/invocations
```

JSON request:

```json
{
  "operation": "TTS",
  "input": {
    "content": [
      {
        "label": "SCRIPT",
        "type": "TEXT",
        "text": "Shift change starts in five minutes."
      },
      {
        "label": "STYLE",
        "type": "TEXT",
        "text": "calm, clear, warm"
      }
    ]
  },
  "bindings": [
    {
      "slot": "TEXT",
      "source": "INPUT",
      "selector": { "label": "SCRIPT", "type": "TEXT" }
    },
    {
      "slot": "VOICE_REFERENCE",
      "source": "DEFAULT",
      "content": {
        "label": "VOICE_REFERENCE",
        "type": "AUDIO",
        "file": "factory/assets/default-voice.wav",
        "contentType": "audio/wav"
      }
    }
  ],
  "options": {
    "outputContentType": "audio/wav",
    "timeoutMillis": 30000
  }
}
```

JSON metadata response for non-streaming clients:

```json
{
  "invocationId": "MODEL_INVOCATION_123",
  "model": "OMNIVOICE_Q4_K_M",
  "operation": "TTS",
  "outcome": "SUCCEEDED",
  "output": {
    "content": [
      {
        "label": "SPEECH",
        "type": "AUDIO",
        "contentType": "audio/wav",
        "artifactId": "MODEL_ARTIFACT_123",
        "metadata": {
          "sampleRate": "24000",
          "durationMillis": "1820"
        }
      }
    ]
  },
  "diagnostics": {
    "loadState": "REUSED",
    "queueDurationMillis": "12",
    "inferenceDurationMillis": "740"
  }
}
```

For audio-producing invocations, the API must also support a streamed response mode. The request may use an `Accept` header such as `audio/wav` or an explicit option, and the response body should stream the produced audio while exposing invocation metadata through headers or a sidecar metadata envelope.

### Pull Model

```http
POST /models/{model_name}/pull
```

Request:

```json
{
  "force": false
}
```

Response:

```json
{
  "model": "OMNIVOICE_Q4_K_M",
  "status": "AVAILABLE",
  "cachePath": "~/.you-agent-factory/models/OMNIVOICE_Q4_K_M",
  "files": [
    "omnivoice-base-Q4_K_M.gguf",
    "omnivoice-tokenizer-Q4_K_M.gguf"
  ]
}
```

## Technical Considerations

- This feature requires OpenAPI schema updates and generated backend/frontend type updates.
- `WorkContent` currently supports lowercase text and image content parts; model-operation content must move toward uppercase `TEXT`, `IMAGE`, `AUDIO`, `JSON`, and `BINARY` while preserving backward compatibility for existing submissions.
- Backend-owned `interfaces.WorkContentPart` and `pkg/workcontent` translation helpers must be updated before invocation APIs depend on expanded content parts.
- Config parsing must support operation contracts on workstations and capability declarations on workers.
- Runtime validation should separate type compatibility, operation compatibility, content compatibility, and resource compatibility so errors are actionable.
- Local model management should live behind an interface so tests can use fake model runtimes and cloud workers can use the same operation execution path.
- Service-owned process-level model resource enforcement is needed because multiple factory sessions can run concurrently.
- Event history and replay should record resolved slot bindings and as much request/response detail as practical, with bounded handling for large binary audio.
- The first real local adapter should target `OMNIVOICE` through a C++/GGML or compatible embedded boundary while keeping implementation details outside the public operation contract.

## Testing Strategy

- Unit tests should cover contract parsing, uppercase enum validation, `WorkContent` compatibility, slot selectors, binding resolution, and worker/workstation compatibility.
- Package integration tests should cover fake local model loading, model manager lifecycle, resource acquisition, API handlers, CLI request construction, and event emission.
- Service-level tests should cover a factory using `MODEL_INVOKE` with a fake local `TTS` worker and verify output work content.
- Real local inference long tests should pull or locate `OMNIVOICE_Q4_K_M` assets, load the model, invoke `TTS`, and validate non-empty audio output.
- GitHub Actions should run `make long-tests` on macOS, Windows, and Linux.
- `make long-tests` should be clearly separated from ordinary PR tests so everyday feedback stays fast while local inference still receives regular platform coverage.
- CI diagnostics should include operating system, architecture, backend, model name, model cache path, load duration, invocation duration, output content type, and output byte count.

## Success Metrics

- A factory author can swap a `TTS` workstation from local `OMNIVOICE_Q4_K_M` to a cloud TTS worker without changing the workstation operation contract.
- A simple local TTS factory can produce an audio artifact from text input without Python dependency setup.
- `/models` and `/models/{model_name}/invocations` allow a developer to validate local inference without creating a full factory.
- Multiple factories can use local model resources without exceeding configured local model capacity.
- Invalid workstation/worker pairings fail during validation with clear compatibility errors.
- Runtime history clearly shows which operation ran, which worker executed it, which resources were consumed, and where output artifacts were produced.
- Long-test GitHub Actions lanes validate local invocation on macOS, Windows, and Linux.

## Open Questions

- What exact streamed response format should carry both audio bytes and invocation metadata for `POST /models/{model_name}/invocations`?
- Should additional explicit `OMNIVOICE_*` model names be added after `OMNIVOICE_Q4_K_M` proves stable in long tests?
- Should `/models/{model_name}/pull` be synchronous for v1, or should it become an asynchronous operation with progress events?
