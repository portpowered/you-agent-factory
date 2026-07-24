# Packaged Service Structure integration metadata

Checked-in scheduling and ownership metadata for the Packaged Service Structure
program. These files are architecture / integration metadata only: they do not
move packages or cut over HTTP, CLI, or MCP transports.

| Artifact | Role |
| --- | --- |
| [`shared-surface-ownership-model.md`](./shared-surface-ownership-model.md) | Human-readable shared-surface ownership model |
| [`shared-surface-ownership.schema.json`](./shared-surface-ownership.schema.json) | Machine-checkable inventory schema |
| [`shared-surface-ownership.json`](./shared-surface-ownership.json) | Live shared-surface ownership inventory |

Focused validation lives in `internal/sharedsurfaceownership`.
