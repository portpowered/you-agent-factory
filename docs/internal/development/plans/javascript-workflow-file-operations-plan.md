# JavaScript Workflow Read-Only File Operations Plan

## Status

Proposed. This document is an implementation plan only. It is intentionally
separate from child Provider permission-bypass work.

## Problem statement

The JavaScript workflow VM exposes no filesystem API. Static validation rejects
the `fs` root and runtime construction binds no filesystem object. As a result,
a workflow cannot:

- list files available in its project;
- inspect a file directly;
- validate text, binary, or multimedia headers and metadata; or
- prepare a file for a future inference input without first asking an agent to
  invoke its own `readFile` tool.

The required first surface is deliberately small and read-only: list one
directory, open one regular file, read bounded bytes, and close the file. Its
file value must also leave a clean future path for passing images, audio, video,
or binary data directly to inference without serializing the full payload
through prompts, JavaScript JSON, or agent tool output.

## Customer outcome

A workflow can discover and inspect project files through a bounded,
project-relative host API while `READ_ONLY` continues to prohibit filesystem
mutation, process execution, module loading, and network access.

## Current-state findings

- `pkg/services/factory_runtime/internal/services/orchestration/javascript/validation/globals.go`
  explicitly rejects `fs`.
- `runtime/globals.go` documents and enforces a VM with no host filesystem
  object.
- Factory Runtime already receives injected `ReadDir`, `ReadFile`, and `Stat`
  seams for source loading and input watching, but those interfaces do not own a
  script-visible capability contract.
- Factory Sessions already knows the project root used for child working
  directories, while `JavaScriptRuntimeRequest` has no approved read root or
  filesystem capability.
- Canonical Work content already supports image, audio, and binary content using
  content URLs and MIME types. Codex image execution already materializes local
  content and passes a path to the Provider.
- `agent.run` cannot currently accept Work content or a workflow-owned file
  value.

## Product decisions

1. Bounded project reads are part of `READ_ONLY`; no dangerous permission bypass
   is required.
2. The public API is a host-provided global `fs` namespace, not Node/Bun/Deno
   module compatibility.
3. The approved root is supplied by Factory Session/runtime composition and is
   never script-authored.
4. Paths are project-relative. Absolute paths, traversal, symlink escape, device
   files, named pipes, and sockets are rejected before data is returned.
5. The initial operations are only `fs.list`, `fs.open`, `OpenFile.read`, and
   `OpenFile.close`.
6. `fs.list` is non-recursive and deterministic.
7. `OpenFile` is an opaque, non-forgeable, runtime-local capability. It cannot be
   serialized, checkpointed, recorded, or returned as JSON.
8. Reads return `Uint8Array` and support offset/length ranges so validation does
   not require loading large media in full.
9. Explicit `close()` is recommended and idempotent. Runtime cleanup closes all
   leaked handles on success, failure, cancellation, and timeout.
10. Read limits are explicit policy facts and zero never means unbounded.

## Recommended authoring UX

```javascript
const entries = await fs.list("./media");
const candidate = entries.find((entry) => entry.kind === "file");

const file = await fs.open(candidate.path);
try {
  const header = await file.read({offset: 0, length: 4096});

  return {
    name: file.name,
    size: file.size,
    contentType: file.contentType,
    headerBytes: Array.from(header),
  };
} finally {
  await file.close();
}
```

Initial contract shape:

```typescript
type FileEntry = {
  name: string;
  path: string;
  kind: "file" | "directory" | "symlink";
};

type OpenFile = {
  readonly name: string;
  readonly path: string;
  readonly size: number;
  readonly contentType: string;
  read(options?: {offset?: number; length?: number}): Promise<Uint8Array>;
  close(): Promise<void>;
};

declare const fs: {
  list(path: string): Promise<FileEntry[]>;
  open(path: string): Promise<OpenFile>;
};
```

`fs.list("")` and `fs.list(".")` address the approved project root. Results are
sorted lexicographically by normalized name and expose normalized relative
paths, never unrestricted host paths.

`file.read()` reads the whole file only when it fits both the per-read and
per-workflow budgets. Offset/length reads are the expected pattern for large
media validation.

## Future direct multimedia handoff

The initial file-operations delivery does not change `agent.run`. The opaque
handle should support this compatible follow-up without changing `fs.open`:

```javascript
const image = await fs.open("./media/frame.png");
try {
  return await agent.run({
    prompt: "Describe this frame and flag visual defects.",
    content: [{type: "image", source: image}],
  });
} finally {
  await image.close();
}
```

Factory Runtime would validate that `source` is a live handle owned by the
current workflow, map it to canonical Work content, and let Workers/Providers
materialize or consume it through existing content boundaries. The JavaScript
heap would not base64-encode the complete media payload and the agent would not
need a separate file-reading tool call.

## UX comparison

| Shape | Advantages | Costs | Decision |
| --- | --- | --- | --- |
| `fs.list`, `fs.open`, `file.read`, `file.close` | Small, explicit, supports bounded binary reads, and provides a stable future media source. | Requires handle lifecycle and cleanup semantics. | Recommended. |
| `fs.readFile(path)` returning full bytes | Familiar and concise. | Encourages whole-file copies and provides no durable handle for direct inference handoff. | Defer as an optional auto-closing convenience. |
| Raw path strings in `agent.run` | Minimal syntax. | Re-resolves mutable paths, creates time-of-check/time-of-use races, and can bypass the workflow capability. | Reject. |
| Base64 or data URLs | Serializable and provider-neutral. | Copies large media through Goja, JSON, events, and logs. | Reject for local files. |
| `require("fs")` or `node:fs/promises` | Familiar Node API. | Implies module loading and a much larger compatibility/security surface. | Reject for this slice. |

## Work stories

### Story 1 — Project-relative directory listing

As a workflow author, I can discover files within the current project without
receiving write, process, module, or network access.

Acceptance criteria:

- `await fs.list(relativePath)` returns a non-recursive, lexicographically sorted
  list of detached `FileEntry` values.
- Empty path and `"."` resolve to the approved project root.
- Absolute paths, traversal, and symlink escape fail with stable diagnostics
  before the escaped target is read.
- Missing paths, non-directories, permission failures, unavailable filesystem,
  and entry-budget exhaustion have stable codes and bounded messages.
- Results expose normalized relative paths and kinds, never unrestricted host
  paths.
- Listing does not follow directory symlinks recursively.
- Factory Session and standalone CLI compositions behave equivalently.

### Story 2 — Bounded open and metadata

As a workflow author, I can open a regular project file and inspect safe metadata
without exposing the underlying host descriptor or absolute path.

Acceptance criteria:

- `await fs.open(relativePath)` accepts only regular files contained by the
  approved root after symlink resolution.
- Absolute paths, root escape, directories, devices, pipes, and sockets fail
  before a handle is returned.
- Metadata includes normalized relative path, basename, byte size, and
  best-effort content type; unknown types use `application/octet-stream`.
- Content type is advisory and never substitutes for byte validation.
- Handles are non-forgeable and owned by one runtime instance.
- The maximum-open-handles budget is enforced before opening another file.

### Story 3 — Bounded binary reads

As a workflow author, I can validate file contents or multimedia headers without
loading unbounded data into the JavaScript VM.

Acceptance criteria:

- `await file.read()` returns `Uint8Array`.
- Optional offset and length are non-negative safe integers; invalid ranges fail
  before I/O.
- Reads stop at EOF and never return more bytes than requested.
- Whole-file reads fail when the file exceeds the per-read budget.
- Per-read and cumulative workflow byte budgets are enforced deterministically;
  zero or omitted limits never mean unlimited.
- Cancellation prevents further reads and returns the existing runtime
  cancellation outcome.
- File bytes never enter logs, runtime records, recordings, checkpoints, or
  diagnostics.

### Story 4 — Deterministic close and cleanup

As a workflow author, I can release a file explicitly, and the runtime prevents
descriptor leaks if my code does not reach `finally`.

Acceptance criteria:

- `await file.close()` closes the host resource and is idempotent.
- Reads after close fail with a stable `file closed` diagnostic.
- Runtime success, script error, unresolved final, timeout, and cancellation all
  close every remaining handle before `Run` returns.
- One handle cannot be used by a sibling workflow, resumed VM, artifact,
  checkpoint, log field, or final result.
- Checkpoint resume starts with an empty handle registry; prior OS resources are
  never reconstructed.

### Story 5 — Observable and replaceable file effects

As an operator and test author, I can inspect safe access facts and replace all
filesystem effects without touching the host filesystem.

Acceptance criteria:

- Factory Runtime owns root/path policy, handle lifecycle, budgets, and
  script-visible behavior.
- Platform owns the policy-free local filesystem implementation.
- `root.BuildProcess` accepts one exact JavaScript workflow filesystem edge for
  list, stat, regular-file open/read/close, and symlink resolution.
- Runtime records include operation, normalized relative path, byte counts,
  outcome, and diagnostic code, but no bytes or absolute paths.
- Preview reports filesystem availability and effective budgets without
  performing I/O.
- Historical replay displays recorded access facts and never re-opens files.
- Documentation states that resumed execution may observe changed project files
  unless content was staged as canonical Work content or an artifact.

### Story 6 — Future-ready media handle

As a future workflow author, I can pass an already-open file to inference without
changing the filesystem API or copying the full payload through JavaScript JSON.

Acceptance criteria for the seam delivered now:

- Each handle has a private runtime-local identity and internal resolver for
  liveness, ownership, resolved path, size, and content type.
- No public path-string escape hatch is added to `agent.run`.
- The handle can later map to canonical image, audio, or binary Work content
  without Provider-specific types in the JavaScript runtime.
- Future dispatch ownership can retain or snapshot the source safely if script
  cleanup closes the author-facing handle.

The future `agent.run({content})` contract and Provider video capability work are
not required in this implementation plan.

### Story 7 — Published JavaScript contract and docs

As a customer, I can discover the complete read-only file API and its limits.

Acceptance criteria:

- The authored symbol catalog, call-behavior descriptor, schemas, examples,
  errors, and generated package projection agree on `fs` and `OpenFile`.
- `OpenFile` is documented as an opaque returned value, not a constructible
  global.
- Static validation accepts only published `fs` and handle members and continues
  to reject writes, `require`, `node:fs`, process, shell, network, and package
  APIs.
- `you docs javascript-workflows` includes a copy-paste `try/finally` example and
  explains project-root confinement and read budgets.

## Implementation plan

### Contracts and policy

- Add `fs` to supported JavaScript globals and define the closed `fs.list`,
  `fs.open`, `OpenFile.read`, and `OpenFile.close` shapes.
- Remove only the host-provided `fs` root from the blanket forbidden map;
  continue rejecting module-based filesystem access and unsupported members.
- Add stable diagnostics for invalid path, root escape, unsupported file kind,
  invalid range, closed handle, unavailable filesystem, I/O failure, and budget
  exhaustion.
- Add effective-policy limits for maximum open handles, entries per listing,
  bytes per read, and cumulative bytes read. Include them in normalization,
  hashing, preview, and recording projections.

### JavaScript runtime host

- Add the host implementation under
  `pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime/`.
- Bind one `fs` namespace and handle registry per Goja VM.
- Keep path normalization, root checks, symlink containment, ownership,
  accounting, and safe diagnostics in Factory Runtime.
- Return Goja `ArrayBuffer`/`Uint8Array` values without converting binary data to
  JSON or base64.
- Create and settle Goja values only on the VM-owning execution path.
- Close the registry on every terminal runtime path.

### Filesystem edge and composition

- Publish a focused `JavaScriptFileSystem` port from Factory Runtime rather than
  reusing source-loader or input-watcher interfaces by accident.
- Expose only directory listing, stat, regular-file open/read/close, and symlink
  resolution effects.
- Add the exact replacement to `pkg/services/edges/definition.go`; compose the
  production `pkg/platform/filesystem` adapter in `pkg/wire`.
- Supply the filesystem and already-known `projectRoot` through Factory Sessions
  runtime construction. Scripts never choose the root.
- Use the same adapter and confinement rules for Factory Session and standalone
  `you run script.js` execution.

### Contracts, observations, and docs

- Update canonical JavaScript call-behavior and runtime-api sources, then
  regenerate derived artifacts.
- Add safe file-access runtime records and preserve them through recordings and
  historical replay without replay-time I/O.
- Update packaged JavaScript workflow reference docs.

### Media handoff seam

- Brand handles with private runtime-owned identities.
- Retain sufficient internal metadata for a future canonical Work content
  mapping: resolved path, normalized relative path, content type, size, and
  lifecycle state.
- Do not add `agent.run.content` or Provider video support in this delivery.

## API and data changes

- No new REST or MCP operation is required.
- If effective-policy projections expose read budgets, author the OpenAPI changes
  under `api/openapi-main.yaml` and `api/components/` and regenerate clients.
- Add access-record fields only to canonical source contracts.
- Never persist OS handles, absolute paths, or raw file bytes.

## Test plan

- Static validation tests for valid `fs` use and rejected modules, writes,
  process, shell, and network APIs.
- Runtime tests with injected filesystems for sorted listing, metadata,
  full/partial reads, typed arrays, EOF, budgets, close, use-after-close,
  cancellation, and automatic cleanup.
- Windows and POSIX path tests for absolute paths, volume names, separators,
  traversal, case behavior, symlink escape, and special files.
- Factory Session integration tests proving root selection and equal
  canonical/standalone behavior.
- Recording/replay tests proving safe metadata retention and zero replay I/O.
- Contract and packaged CLI documentation smoke tests.
- Leak tests for success, script failure, unresolved final, timeout, and
  cancellation.

## Quality gates

- `go test ./pkg/services/factory_runtime/...`
- `go test ./pkg/services/factory_sessions/internal/execution/...`
- focused functional JavaScript orchestration tests through
  `root.BuildProcess` and injected edges
- `make javascript-contract-smoke`
- `make contracts-check`
- `make docs-reference-smoke`
- `make api-smoke` when public projections change
- `make verify-fast`, followed by `make verify-pr`

## Out of scope

- `skipPermissions` propagation or Provider bypass behavior; see
  `docs/internal/development/plans/javascript-workflow-permissions-plan.md`.
- Filesystem writes, create, rename, delete, chmod, watch, recursive traversal,
  globbing, or arbitrary host roots.
- Node/Bun/Deno filesystem compatibility or package/module loading.
- Direct process, shell, network, connector, environment-variable, or secret
  APIs.
- Returning, checkpointing, recording, or serializing open handles or raw bytes.
- `agent.run({content})`, video Provider negotiation, or dashboard media preview.
- Unrelated input-watcher, source-loading, Work staging, or Petri refactoring.

## Original documents

- `docs/internal/development/plans/dynamic-workflows/dynamic-workflow-design.md`
- `docs/internal/standards/code/planning-standards.md`

## Delivery boundary

Implementation is complete only when the runtime behavior, injected filesystem
edge, Factory Session and standalone compositions, safe observations, replay,
published contracts, generated artifacts, documentation, and required CI are
terminal and passing; blocking review feedback and conflicts are resolved; and
the pull request is merged.
