# Make the UI unit lane terminate reliably on Windows

`make ui-test` can run for more than five minutes on Windows without emitting a
terminal result, leaving multiple workspace-owned Node test processes behind.
This has recurred across independent CLI work items even when UI sources are
unchanged, so `make verify-fast` cannot provide a reliable local completion
signal on that platform.

Reproduce the non-terminating lane on Windows, identify the Vitest/Bun worker or
process-lifecycle cause, and make the command finish with an explicit success or
failure while preserving the existing test inventory. Add a bounded regression
check for worker cleanup so interrupted and completed runs do not retain
workspace-owned child processes.
