---
author: Agent Factory Team
last-modified: 2026-05-22
doc-id: agent-factory/reference/models
---

# Models

`you docs models` stays available as the stable packaged quick reference for
model discovery, pull, direct invocation, and `MODEL_INVOKE` authoring. Use
[`docs/reference/models.md`](../../../docs/reference/models.md) for the
maintained full guide.

## Current Contract

- `MODEL_INVOKE` workstations request one uppercase operation such as `TTS`.
- The referenced worker must be `MODEL_WORKER` and must declare the matching
  operation plus compatible input and output slots.
- Model-operation input and output use canonical `WorkContent`; legacy
  lowercase `text` and `image` still remain accepted at the API boundary.
- `operationBindings` resolve slot content from runtime input, authored
  config, defaults, or omission.
- Typed resources such as `MODEL`, `PROVIDER_QUOTA`, and `INVOCATION_SLOT`
  make local-model and cloud-quota capacity schedulable.

## Shared `/models` Surface

| Goal | CLI | API |
|------|-----|-----|
| List models | `you models list` | `GET /models` |
| Inspect one model | `you models inspect OMNIVOICE_Q4_K_M` | `GET /models/{model_name}` |
| Invoke one model | `you models invoke OMNIVOICE_Q4_K_M --operation TTS --text "hello"` | `POST /models/{model_name}/invocations` |
| Pull local assets | `you models pull OMNIVOICE_Q4_K_M` | `POST /models/{model_name}/pull` |

## Streamed Audio Versus JSON

- Use `--json` when you want invocation metadata and canonical content
  references.
- Use `--output <path>` when you want the audio stream written directly to a
  file.

## Maintainer Lane

Use `make long-tests` for real OMNIVOICE local inference coverage. Set
`INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1` and ensure the
`omnivoice-llamacpp` command is available, or point
`INFINITE_YOU_OMNIVOICE_COMMAND` at it explicitly.

## Related

- `you docs workstation`
- `you docs workers`
- `you docs resources`
- `you docs config`
