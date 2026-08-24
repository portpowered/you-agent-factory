---
type: INFERENCE_WORKER
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
    outputs:
      - name: audio
        contentTypes:
          - AUDIO
---
Synthesize speech from the resolved utterance.
