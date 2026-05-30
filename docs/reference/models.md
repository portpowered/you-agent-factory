---
author: Agent Factory Team
last-modified: 2026-05-22
doc-id: agent-factory/models
---

# Models And Model Operations

Use this page when you need the current customer-facing contract for
model-backed operations: `MODEL_INVOKE` workstations, `MODEL_WORKER`
capabilities, typed model resources, multimodal `WorkContent`, and the
runtime `/models` discovery, pull, and invocation surface.

Keep the workflow topology and field-by-field `factory.json` contract in
[Config](config.md). Keep worker-only runtime fields
in [Workers](workers.md), workstation-only routing and prompt fields in
[Workstations](workstations.md), and resource pool semantics in
[Resources](resources.md).

## Current Contract

- Use `type: "MODEL_INVOKE"` on a workstation when the step should request one
  uppercase provider-agnostic operation such as `TTS`.
- Keep operation names, model localities, resource types, slot content types,
  and other public enum-like values uppercase in authored config.
- A `MODEL_INVOKE` workstation must reference a `MODEL_WORKER` that declares
  the same operation and a compatible input and output contract.
- Model invocation input and output use canonical ordered `WorkContent`.
  Existing lowercase `text` and `image` parts remain valid at the API
  boundary, while new multimodal parts should use uppercase public types such
  as `TEXT`, `IMAGE`, `AUDIO`, `JSON`, and `BINARY`.
- Use `operationBindings` to map runtime input or authored config content into
  named worker slots. Bindings resolve from `INPUT`, `CONFIG`, `DEFAULT`, or
  explicit omission.
- Use typed resources such as `MODEL`, `PROVIDER_QUOTA`, and
  `INVOCATION_SLOT` when you need scheduling to enforce local cache capacity,
  shared cloud quota, or per-invocation concurrency.

## WorkContent For Model Operations

Model-operation payloads reuse the same ordered `WorkContent` shape used by
submission and replay surfaces.

```json
[
  {
    "type": "TEXT",
    "label": "utterance",
    "role": "user",
    "text": "Please narrate this update."
  },
  {
    "type": "JSON",
    "role": "voice",
    "json": { "name": "alloy", "speed": 1.0 }
  }
]
```

Each content part can also carry optional `contentType`, `artifactId`, and
`metadata` fields when the operation needs richer runtime context or when the
response needs to refer to produced artifacts instead of embedding large binary
payloads inline.

## Compatibility Rules

Keep the compatibility chain explicit:

1. The workstation `type` must be `MODEL_INVOKE`.
2. The workstation `operation` must be uppercase and must match one declared
   worker operation.
3. The referenced worker must be `MODEL_WORKER`.
4. That worker operation must declare at least one input slot and one output
   slot.
5. Each authored binding slot must exist in the worker's declared input slots.
6. Bound content must satisfy the worker slot's allowed content types.

Validation distinguishes wrong workstation type, operation mismatch, duplicate
worker declarations, duplicate slot names, unknown binding slots, invalid enum
values, and incompatible content contracts so authors do not need to read the
implementation to diagnose config errors.

## Slot Bindings

Use `operationBindings` on the workstation to keep provider-specific slot names
out of submitted work. A binding targets a named worker slot and resolves it in
this order:

- matching runtime input content through `selector`
- authored `config` content
- authored `defaultContent`
- omitted when the slot is optional and no source matched

Selectors can match by `slot`, `label`, `type`, or `role`. Resolution is
deterministic against the ordered runtime input content.

```json
{
  "type": "MODEL_INVOKE",
  "operation": "TTS",
  "worker": "tts-worker",
  "operationBindings": [
    {
      "slot": "text",
      "selector": {
        "label": "utterance",
        "type": "TEXT"
      }
    },
    {
      "slot": "voice",
      "config": [
        {
          "type": "JSON",
          "role": "voice",
          "json": { "name": "alloy" }
        }
      ]
    }
  ]
}
```

## Local And Cloud TTS With One Workstation Contract

The workstation contract stays the same across local and cloud execution. Only
the bound worker and resource metadata change.

### Shared workstation

```json
{
  "name": "speak",
  "type": "MODEL_INVOKE",
  "operation": "TTS",
  "worker": "tts-worker",
  "operationBindings": [
    {
      "slot": "text",
      "selector": {
        "type": "TEXT",
        "label": "utterance"
      }
    },
    {
      "slot": "voice",
      "defaultContent": [
        {
          "type": "JSON",
          "role": "voice",
          "json": { "name": "alloy" }
        }
      ]
    }
  ],
  "inputs": [{ "workType": "speech", "state": "init" }],
  "outputs": [{ "workType": "speech", "state": "complete" }],
  "onFailure": [{ "workType": "speech", "state": "failed" }]
}
```

### Local OMNIVOICE_Q4_K_M worker

`factory.json`:

```json
{
  "resources": [
    {
      "name": "omnivoice-cache",
      "type": "MODEL",
      "capacity": 1,
      "model": "OMNIVOICE_Q4_K_M",
      "backend": "LLAMACPP",
      "loadPolicy": "ON_DEMAND"
    }
  ],
  "workers": [
    { "name": "tts-worker" }
  ]
}
```

`workers/tts-worker/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: CODEX
modelLocality: LOCAL
resources:
  - name: omnivoice-cache
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
      - name: voice
        contentTypes:
          - JSON
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech from the resolved text content.
```

### Cloud-backed TTS worker

`factory.json`:

```json
{
  "resources": [
    {
      "name": "cloud-tts-quota",
      "type": "PROVIDER_QUOTA",
      "capacity": 8,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    },
    {
      "name": "cloud-tts-slot",
      "type": "INVOCATION_SLOT",
      "capacity": 2,
      "provider": "CODEX",
      "model": "gpt-4o-mini-tts"
    }
  ],
  "workers": [
    { "name": "tts-worker" }
  ]
}
```

`workers/tts-worker/AGENTS.md`:

```yaml
---
type: MODEL_WORKER
model: gpt-4o-mini-tts
modelProvider: CODEX
modelLocality: CLOUD
resources:
  - name: cloud-tts-quota
    capacity: 1
  - name: cloud-tts-slot
    capacity: 1
operations:
  - name: TTS
    inputs:
      - name: text
        required: true
        contentTypes:
          - TEXT
      - name: voice
        contentTypes:
          - JSON
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech through the cloud-backed provider.
```

Cross-factory local-model throttling is enforced by the running service with
canonical model metadata, not only by one factory-local resource name. That
means two concurrently running factories that both target the same local model
still share one process-level local capacity boundary.

## `/models` API And CLI Surface

Use the `/models` surface to inspect readiness, pull managed local assets, and
invoke one model directly without running a full factory workflow.

| Goal | API | CLI |
|------|-----|-----|
| List discovered models | `GET /models` | `you models list` |
| Inspect one model | `GET /models/{model_name}` | `you models inspect OMNIVOICE_Q4_K_M` |
| Invoke one model | `POST /models/{model_name}/invocations` | `you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "hello" --output speech.wav` |
| Pull managed local assets | `POST /models/{model_name}/pull` | `you models pull OMNIVOICE_Q4_K_M` |

### Discovery

`GET /models` and `you models list` report the concrete public model name,
provider locality, readiness, load state, supported operations, supported
modalities, and summarized resource metadata.

### Pull

Use `POST /models/{model_name}/pull` or `you models pull <model>` to populate
the managed cache for supported local assets such as `OMNIVOICE_Q4_K_M`.
Successful pull responses report the outcome, cache path, revision, and
downloaded files. Invocation keeps returning `MODEL_NOT_AVAILABLE` until the
required local assets are present.

### Invocation

`POST /models/{model_name}/invocations` accepts the same uppercase operation
name, canonical `WorkContent`, optional per-request `bindings`, and invocation
options used by the factory runtime.

Use JSON response mode when you want metadata plus canonical `WorkContent`
references:

```bash
you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "release notes" --json
```

Use streamed-audio mode when you want the audio bytes directly:

```bash
you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "release notes" --output speech.wav
```

The streamed response writes the audio body directly to the requested output
path. The JSON response returns invocation metadata and audio artifact
references instead of a raw stream.

## Maintainer Long-Test Expectations

`make long-tests` is the maintainer entrypoint for real OMNIVOICE local
inference coverage.

- Set `INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1` to opt into the real-runtime
  test.
- Ensure `omnivoice-llamacpp` is on `PATH`, or point
  `INFINITE_YOU_OMNIVOICE_COMMAND` at the executable explicitly.
- Set `INFINITE_YOU_OMNIVOICE_CACHE_DIR` when you want to reuse an existing
  managed cache instead of pulling into a temporary test directory.
- In CI, `.github/workflows/long-local-inference.yml` restores
  `.cache/managed-models`, installs the backend command, and runs the same
  `make long-tests` lane unchanged.

## Related

- [Config](config.md)
- [Submitted work](work.md)
- [Author factories](authoring-factories.md)
- [Workstations](workstations.md)
- [Workers](workers.md)
- [Resources](resources.md)
- [Functional long-test package map](../internal/processes/runtime-lookup-test-fixture-inventory.md)
