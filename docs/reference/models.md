---
author: Agent Factory Team
last-modified: 2026-07-13
doc-id: agent-factory/models
---

# Models

Use this guide to discover models, check readiness, install managed local
assets, and invoke a model directly. For model-backed Factory authoring, this
page explains the boundary between direct model operations and
`INFERENCE_WORKER` plus `INFERENCE_RUN`; detailed worker, workstation, and
resource fields remain in their owning guides.

Use `you docs providers` for agent worker/provider selection, configured model
roles, effort choices, modality and tool limits, and AGY or ACP behavior. This
page is the canonical guide for managed model discovery, readiness, pull, and
direct inference operations.

## Discover And Inspect Models

Start a Factory service before using discovery, inspection, or pull commands.
They query its `/models` surface; use the global `--server` option when the
service is not at the default address.

```bash
you models list
you models inspect OMNIVOICE_Q4_K_M
```

`list` summarizes each model's provider locality, supported operations and
modalities, managed-runtime readiness and lifecycle, and resource count.
`inspect` shows one model's worker capabilities and readiness diagnostics. Add
the global `--json` flag when scripts need the structured response.

Use these managed-runtime fields when deciding what to do next:

| `readinessState` | Meaning |
| --- | --- |
| `READY` | Invocation can proceed. |
| `MISSING` | Required managed assets are not installed; pull the model. |
| `LOADING` | Assets exist but the runtime is still starting; wait and inspect again. |
| `FAILED` | Startup or health checks failed; inspect diagnostics and service logs. |
| `UNSUPPORTED` | This installation cannot manage the requested runtime. |

`lifecycleState` distinguishes installation from loading, including
`NOT_INSTALLED`, `INSTALLING`, `INSTALLED`, `LOADING`, and `LOADED`.
For managed local models, Models derives the two fields together from the
observed cache, active pull, and host facts:

- no verified required assets is `MISSING` / `NOT_INSTALLED`;
- an active pull with incomplete assets is `LOADING` / `INSTALLING`;
- verified required assets are `READY` / `INSTALLED`, or `READY` / `LOADED`
  when the host is loaded; and
- a failed observation is `FAILED` with a lifecycle consistent with the
  installed evidence.

The service does not publish `READY` with `NOT_INSTALLED`.

## Pull A Managed Local Model

Pull supported local assets into the service's managed cache:

```bash
you models pull OMNIVOICE_Q4_K_M
you --json models pull OMNIVOICE_Q4_K_M
```

The command is synchronous for managed local assets: it remains active until
source transfer, byte/checksum verification, and cache publication reach a
terminal success or failure. It does not report success while the backing
download is still running. The result reports the pull outcome, resulting
readiness, cache path, revision, and downloaded files. Common outcomes include
`ALREADY_READY`, `INSTALLED_SUCCESSFULLY`, `ALREADY_PRESENT`, `STILL_LOADING`,
`TIMED_OUT`, `SOURCE_FETCH_FAILED`, and `UNSUPPORTED_RUNTIME`. A successful
pull normally leaves the model `READY` / `INSTALLED` (or `READY` / `LOADED` if
the host is already loaded); a concurrent `inspect` may observe
`LOADING` / `INSTALLING` while the pull is active.

If pull fails, use the returned outcome and diagnostics rather than editing the
managed cache. Check network and source credentials for `SOURCE_FETCH_FAILED`,
retry a timed-out pull, and verify the configured backend command and health
endpoint when installed assets enter `FAILED`.

## Invoke A Model Directly

Direct invocation runs one named operation through the current `./factory`
configuration without starting a full Factory workflow. Every invocation must
name the model, name an uppercase operation, provide non-empty text, and choose
either an output file or JSON metadata.

Write TTS audio to a file:

```bash
you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "Read the release summary." --output ./speech.wav
```

Return structured invocation metadata and canonical `WorkContent` references:

```bash
you --json models invoke OMNIVOICE_Q4_K_M --operation TTS --text "Read the release summary."
```

`--output` selects streamed-audio mode and writes the response bytes to the
given path. The global `--json` flag selects metadata mode instead. Omitting
`--text`, or omitting both result choices, is rejected before model execution.

Invocation is readiness-gated. For `MISSING`, pull the model. For `LOADING`,
wait and inspect again. For `FAILED`, use the inspect diagnostics and service
logs to correct runtime startup or health failures. `MODEL_NOT_AVAILABLE` and
the managed-runtime failure details identify the model and readiness state;
successful output is not emitted for a failed invocation.

## Direct Operations Versus Factory Execution

`you models invoke` is a bounded direct operation: it selects one configured
model capability and returns that operation's result. Use a Factory when Work
must be routed, scheduled, retried, observed through Factory Session events, or
combined with other steps.

In authored Factory configuration:

- an `INFERENCE_WORKER` declares the model, locality, operation, input slots,
  and output slots;
- an `INFERENCE_RUN` workstation selects that worker and operation and maps
  submitted `WorkContent` into the declared slots;
- a `MODEL` resource declares a managed local runtime dependency and capacity.

For TTS, the worker declares an uppercase `TTS` operation with required `TEXT`
input and `AUDIO` output. Submitted Work supplies the utterance; the Factory
Session owns dispatch and the primary result. This is inference behavior, not
an agent loop. Legacy `MODEL_WORKER` and `MODEL_INVOKE` values remain migration
inputs, but new Factory configuration should use `INFERENCE_WORKER` and
`INFERENCE_RUN`.

Use `you docs providers` for agent provider/model selection and limits. Use
`you docs workers` for worker capabilities, `you docs workstations` for routing
and bindings, `you docs resources` for managed capacity, `you docs config` for
the minimum Factory contract, and `you docs run` for complete Factory
invocation shapes.
