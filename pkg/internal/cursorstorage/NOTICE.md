# MIT Attribution — cursor-agent CLI storage parsing

Portions of this package are derived from [iksnae/cursor-session](https://github.com/iksnae/cursor-session) (MIT License).

- **Upstream repository:** https://github.com/iksnae/cursor-session
- **Source commit (default branch HEAD at port time):** `340f0f72a760ba8b454eac814f986d1a6a4c2f57`
- **Ported modules (in-process):** cursor-agent `store.db` discovery helpers, SQLite open/read, blob/meta parsing, protobuf and redacted-reasoning decoding used by agent storage loading.
- **Not ported:** CLI entrypoints, desktop globalStorage exporters, cache reconstruction, and unrelated storage backends.

The infinite-you server resolves sessions only from configured server-side storage roots using validated session identifiers; clients cannot supply filesystem paths.
