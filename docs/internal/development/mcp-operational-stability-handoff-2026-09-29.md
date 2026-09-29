# Windows Models and MCP operational stability handoff — 2026-09-29

This records the stopping point after native Windows LLM, TTS, and ASR
invocations succeeded. The detailed run histories are in
[the LocalAI GPU audit](./localai-gpu-invocation-audit-2026-09-28.md) and
[the MCP subagent reliability log](./mcp-subagent-reliability-2026-09-28.md).
The model probes exercised the managed `you models` path; they are not a
three-modality test of the MCP transport itself.

## Confirmed on native Windows

| Modality | Public invocation and observed result | CUDA evidence and limit |
| --- | --- | --- |
| LLM | Managed Gemma 4 E4B pull succeeded. `you models invoke llm --operation OMNI` exited 0 and returned `READY.` with 3 generated tokens. The saved result is `llm-invoke4.json` in the evidence directory below. | The resolver selected the published Windows CUDA llama archive and verified its digest. The backend process appeared in `nvidia-smi` while GPU memory rose by about 5.6 GiB. No CUDA initialization trace was captured, so exact compute placement remains unproven. |
| TTS | `you models invoke tts` selected the published VibeVoice CUDA archive and produced a valid, non-silent mono 24 kHz WAV, 70,444 bytes, from “Hello from published CUDA.” | A direct native gRPC probe also synthesized speech while the VibeVoice process appeared in `nvidia-smi`. This confirms managed archive selection and usable audio output. |
| ASR | `you models invoke asr` selected the repaired Whisper CUDA archive and transcribed the known WAV fixture as `Zero.` with a segment result. | Direct native backend logs reported `using CUDA0 backend` on the RTX 4090. The first published Whisper archive lacked `ggml-cpu.dll`; the repaired archive was published and used for this passing invocation. |

The LLM run's local evidence directory is
`C:\Users\andre\AppData\Local\Temp\you-native-cuda-aca54b5bf4f44e75b147760509c2517c`.
The TTS and ASR measurements and release identifiers are recorded in the GPU
audit linked above. A direct Whisper probe recognized synthesized VibeVoice
speech as `Hello from CUDA!`; the public ASR validator rejected that particular
short generated WAV because Whisper's segment end exceeded its audio duration.
The known-fixture public ASR invocation passed.

## MCP endpoint behavior confirmed

- Fresh Windows MCP stdio processes initialized, listed `you.subagent`, and
  completed bounded OpenCode edit tasks with a primary result. The created
  files were verified in the workspace. Editing required no special flag.
- Two `you.subagent` calls sent concurrently through one fresh MCP server both
  completed with primary results and their requested files. A separate pair
  of OpenCode Muse read-only calls also completed concurrently.
- A malformed request returned a readable first text message, `isError=true`,
  and a typed `structuredContent` error. Focused protocol tests cover this
  presentation. A model-only qualified OpenCode request succeeded after the
  transport began inferring its provider; a conflicting explicit provider
  returned structured `BAD_REQUEST`.
- External-directory read and edit probes succeeded for the particular test
  paths on this host. They do not establish universal access to every path.
- A Windows managed LLM pull and OMNI invocation also finished inside an MCP
  subagent session with a primary result. This took about 11 minutes, including
  the model pull, and required correction of local PowerShell/Python command
  construction before the successful attempt.

## Remaining operational stability work

1. Long OpenCode jobs remain unreliable. Several 10–20 minute calls returned
   typed `factory_session.subagent.timed_out` with recent provider activity and
   no primary result; some left partial edits. One call did not return an MCP
   response even after its deadline and client close. The exact hang point was
   not captured. Verify timeout and cancellation against a deliberately
   noncooperative provider and provide a resumable outcome for long work.
2. Free OpenCode model capacity varies. Recent Muse and MiMo calls returned
   typed `provider_throttled`; another qualified model returned
   `provider_request_rejected`. These are provider outcomes, separate from
   working-root or filesystem permission failures.
3. Earlier empty OpenCode results and `external_directory` permission asks
   motivated permission handling changes. Specific external read/edit probes
   now pass, but a fresh end-to-end test of the original failing paths and
   permission elicitation lifecycle remains needed. `peer connection closed`
   at normal stdio EOF alone is not evidence of failure.
4. A new MCP server process was validated with the repaired Windows binary;
   pre-existing desktop MCP processes can retain an older executable until
   reconnection. Confirm the desktop-hosted connection's binary version after
   restart before using its results as validation of the latest server.
5. The managed model probes establish native Windows backend operation, but
   they do not prove video understanding, audio understanding through LLM
   OMNI, all CUDA variants, or sustained multi-agent sessions. The public ASR
   segment-duration rejection on a short generated WAV remains a separate
   model-output/validation issue.

At this point the requested three Windows modalities work. This handoff ends
the present validation run; the items above are follow-up work, not claims of
completed MCP stability.
