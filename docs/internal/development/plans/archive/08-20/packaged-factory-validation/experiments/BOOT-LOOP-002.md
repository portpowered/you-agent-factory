# BOOT-LOOP-002

## Identity

- Status: `PASSED`
- Factory: `@you/loop`
- Repository base: `9cdd70fef3aa202cf5702d8aac002a05436ac044`
- Worktree: `.artifacts/bootstrap/worktrees/BOOT-LOOP-002`
- Model: provider `CODEX`, model `gpt-5.6-terra`
- Generated artifact SHA-256:
  `2286EC1EA401BAA37C4A6458E06390A75B2EEBE2CF3577ACAF7EA2C6215D104F`
- Recording: `.artifacts/bootstrap/BOOT-LOOP-002-R01.replay.json`
- Recording SHA-256:
  `A04178F4DD711F8C18F0D084C89A20E0EC8C137AB9682401C41E47B11FEF4AAF`

## Exact command

```powershell
& '.artifacts/bootstrap/bin/you-9cdd70fef.exe' run --continuously --quiet --factory '.artifacts/bootstrap/worktrees/BOOT-LOOP-002/packages/packaged-factories/generated/factories/loop/factory.yaml' --record '.artifacts/bootstrap/BOOT-LOOP-002-R01.replay.json' --provider CODEX --model gpt-5.6-terra --every 20s --trigger-at-start true --max-consecutive-failures 2 --to 'Return exactly LOOP_OCCURRENCE_OK after reading this request. Do not modify files.'
```

The process ran with an isolated `HOME` and `USERPROFILE` rooted at
`.artifacts/bootstrap/homes/BOOT-LOOP-002`. It was explicitly stopped after
four accepted occurrences, then observed for another complete interval.

## Results

The recording contains four scheduled Work requests and four successful agent
dispatches. Every request preserved the complete customer text and carried the
expected `interval=20s`, sequence, and `SCHEDULED` metadata. Every model result
was exactly `LOOP_OCCURRENCE_OK`.

Nominal trigger times were exactly 20 seconds apart. Actual scheduling drift
was below one millisecond for each occurrence, and each execution completed in
4.3 to 6.6 seconds, so no overlap occurred. The recording contained 29 events
when the process was stopped and still contained 29 after a further 22 seconds;
the process was no longer running.

Deterministic coverage additionally proves configured start behavior, duration
validation, overlap suppression, cancellation, and that reaching the
consecutive-failure ceiling disables later triggers.

One rejected preflight attempted continuous execution with response-stream
output. The CLI correctly rejected that unsupported combination with
`INVOCATION_OUTPUT_UNSUPPORTED` before starting a Factory execution.

## Score and decision

- Intended outcome: 5/5
- Factory-specific behavior: 4/4
- Correctness and evidence: 4/4
- Safety and scope: 3/3
- Final result quality: 2/2
- Efficiency: 2/2
- Total: 20/20
- Canary status: `PASSED`
- Representative status: `PASSED`
- Goal status: `MEETS_EXPECTATIONS`
