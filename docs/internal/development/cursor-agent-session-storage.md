# Cursor agent CLI session storage (v1)

Infinite-you loads Cursor provider-session detail from **cursor-agent CLI storage** parsed in-process under `pkg/internal/cursorstorage`. The external [`cursor-session`](https://github.com/iksnae/cursor-session) CLI is not invoked at runtime.

## v1 scope

| Backend | Required for v1 | Notes |
| --- | --- | --- |
| cursor-agent CLI storage (`store.db` under chats roots) | **Yes** | Linux paths: `~/.config/cursor/chats`, then `~/.cursor/chats` |
| Cursor desktop `globalStorage` (`state.vscdb`) | No | Documented as optional/future |

## Layout

```
{agentStorageRoot}/{workspace-hash}/{session-id}/store.db
```

Session identifiers are validated (`^[A-Za-z0-9_-]+$`). The API and parser never accept client-supplied filesystem paths.

## Attribution

MIT-ported parsing from `iksnae/cursor-session` at commit `340f0f72a760ba8b454eac814f986d1a6a4c2f57`. See `pkg/internal/cursorstorage/NOTICE.md`.

## Manual verification

Record results of parsing a local developer machine’s chats root in PR or closeout notes when validating story US-004/US-012 (for example: session count resolved, one readable transcript, encrypted blobs counted as unavailable).
