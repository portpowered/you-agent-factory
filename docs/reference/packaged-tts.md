# Packaged TTS (`@you/tts`)

Use this guide when you want the first-party packaged text-to-speech factory:
invoke it by name, read the default metadata result, find the materialized
factory on disk, and customize it like any other named factory.

`you docs sessions` and `you docs config` own the shared invocation input and
return-policy contract. This guide focuses on the `@you/tts` packaged factory
workflow.

The default `@you/tts` factory uses inference worker/run behavior:
`INFERENCE_WORKER` plus `INFERENCE_RUN` for one bounded TTS operation through
the local OmniVoice managed runtime. It does not use agent-loop fields.

## Quick start

Generate speech from positional text:

```bash
you run --named @you/tts "hi there"
```

Pipe stdin when no positional text is present:

```bash
echo "hi there" | you run --named @you/tts
```

Supplying both positional text and piped stdin for the same invocation is
rejected with `INVOCATION_INPUT_SOURCE_CONFLICT` before runtime work starts.

## Default invocation result

Successful `@you/tts` invocations return JSON metadata on stdout, not raw audio
bytes. The primary result is authored by the packaged factory runtime through
the shared invocation-return contract.

Typical metadata shape:

```json
{
  "artifactPath": "/path/to/generated.wav",
  "mediaType": "audio/wav",
  "backend": "OMNIVOICE_Q4_K_M/LLAMACPP",
  "traceId": "trace-123",
  "sessionId": "~default"
}
```

| Field | Meaning |
|-------|---------|
| `artifactPath` | Filesystem path or URL reference to the generated audio artifact |
| `mediaType` | Output media type for the artifact |
| `backend` | Selected managed TTS model/runtime identifier derived from the loaded factory worker configuration |
| `traceId` | Invocation trace correlation when exposed by the shared contract |
| `sessionId` | Factory session identifier when exposed by the shared contract |

Raw artifact streaming on stdout is intentionally out of scope for this slice.
Future artifact streaming will use the shared invocation contract rather than a
TTS-only escape hatch.

## Where the factory materializes

`you run --named @you/tts` resolves named factories in this order:

1. Project-local `./factory`
2. Global shared root `~/.you-agent-factory/you-agent-factories`
3. Built-in catalog materialization on first use

On first invocation, `@you/tts` materializes into the global root using the
normal named-factory persist pipeline. Scoped names use hierarchical safe
segments on disk:

```text
~/.you-agent-factory/you-agent-factories/@you/tts/
```

Inspect the materialized factory:

```bash
you factory list --dir ~/.you-agent-factory/you-agent-factories
```

The directory contains `factory.json`, split `workers/` and `workstations/`
files, and any supporting assets needed for the default TTS runtime.

The default `@you/tts` factory uses an `INFERENCE_WORKER` with a `TTS`
operation and an `INFERENCE_RUN` workstation. The materialized on-disk factory
may still show legacy `MODEL_WORKER` / `MODEL_INVOKE` values from earlier
catalog versions; both names execute as inference behavior during the migration
window.

## Customer edits affect the next run

Packaged factories stay editable after materialization. The CLI reuses the
on-disk directory on later invocations instead of overwriting customer changes
with pristine embedded content.

Edit distinguishing fields such as:

- `workers/tts-executor/AGENTS.md` inference worker operation context
- `workstations/execute-tts/AGENTS.md` workstation routing or binding notes
- `factory.json` worker model or resource settings

The default factory is inference behavior (`INFERENCE_WORKER` plus
`INFERENCE_RUN`), not an agent loop. Do not add agent-loop fields such as
`onContinue` or repeater routing when customizing TTS behavior.

The next `you run --named @you/tts` invocation loads the edited on-disk factory
immediately. No reinstall, cache clear, or special reload step is required.

Example: changing the `tts-executor` worker `model` in `factory.json` updates
the `backend` field in the default metadata result on the next successful
invocation.

If the materialized factory becomes invalid or incomplete, invocation fails
with a clear packaged-factory load error instead of silently falling back to
embedded behavior.

## Readiness and failure outcomes

Packaged TTS exposes loading, model-not-ready, and generation-failure states
through the shared invocation surface. Successful metadata is absent until
terminal success.

| Outcome | Stable code (when applicable) |
|---------|-------------------------------|
| Model or backend not ready | `INVOCATION_TTS_MODEL_NOT_READY` |
| Generation failed after startup | `INVOCATION_TTS_GENERATION_FAILED` |

Structured logs record factory resolution, selected backend, and readiness or
failure classification without logging submitted input text or generated artifact
bodies.

## Related topics

- `you docs authoring-factories` — named-factory resolution and factory layout
- `you docs config` — invocation input sources and `invocationReturn` policy
- `you docs sessions` — session-scoped invocation API
- `you docs models` — local TTS model setup and `INFERENCE_RUN` authoring
