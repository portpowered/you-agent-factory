# Make verification targets portable to Windows worktrees

The Windows `make` executable runs recipes without a Unix shell. As a result,
`make verify-fast` fails before any gate runs because its banner and
`run_verification_step` helper invoke `printf` and POSIX shell control syntax.
The Makefile also detects Bun with `command -v`, which is unavailable to this
runner and sends `make ui-test` down a fallback path that reports no test files
instead of the Bun-backed unit suite.

Make verification target orchestration portable across the supported local
development shells without weakening any individual quality gate. Keep the
current Linux/CI behavior, ensure Bun detection works when Bun is available on
Windows, and add a focused reproducible check for the wrapper's command
selection and failure reporting.
