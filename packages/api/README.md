# `@you-agent-factory/api`

`@you-agent-factory/api` publishes the raw JSON and YAML artifacts that describe
the you-agent-factory contract. Its exports are stable artifact locators; they
are not JavaScript module APIs.

## Public exports

| Export | Raw artifact |
| --- | --- |
| `@you-agent-factory/api/manifest` | Commit-independent development metadata and artifact hashes; publication candidates add source provenance |
| `@you-agent-factory/api/openapi` | Bundled OpenAPI YAML |
| `@you-agent-factory/api/cli` | Authoritative static CLI command manifest |
| `@you-agent-factory/api/schemas/cli-command-manifest` | CLI command-manifest JSON Schema |
| `@you-agent-factory/api/mcp` | MCP tool inventory JSON |
| `@you-agent-factory/api/schemas/you-config` | `you` configuration JSON Schema |
| `@you-agent-factory/api/schemas/factory` | Factory configuration JSON Schema |
| `@you-agent-factory/api/schemas/factory-event` | Standalone canonical Factory Event JSON Schema |
| `@you-agent-factory/api/schemas/factory-recording` | Standalone chapter-free Factory Recording JSON Schema |
| `@you-agent-factory/api/schemas/mock-workers` | Mock-worker configuration JSON Schema |
| `@you-agent-factory/api/javascript/runtime` | JavaScript runtime contract JSON |
| `@you-agent-factory/api/joined/*` | Raw joined JSON contract artifacts |

Only these declared package subpaths are supported. Consumers must not inspect
package-internal paths or depend on the repository layout.

## Reading an artifact

Resolve a public subpath through the installed package, then read the resulting
file as data. For example, in Node.js:

```js
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const manifestURL = import.meta.resolve("@you-agent-factory/api/manifest");
const manifest = JSON.parse(
  await readFile(fileURLToPath(manifestURL), "utf8"),
);

const openapiURL = import.meta.resolve("@you-agent-factory/api/openapi");
const openapiYAML = await readFile(fileURLToPath(openapiURL), "utf8");
```

Use the same resolution-and-read pattern for CLI, MCP, schema, JavaScript
runtime-contract, and joined artifacts. Parse `.json` files as JSON and pass
`.yaml` files to the YAML reader chosen by your application. The CLI manifest
contains the static command graph and stable input and handler bindings. Combine
its `you.run` input metadata with a selected Factory's `invocationSignature`
from the Factory schema; a missing signature selects the documented
compatibility-input mode.

## Data-only package

This package provides no JavaScript runtime library, React components,
validators, generated clients, UI output, executables, or runtime dependency
integration. It has no install-time behavior and does not make application code
depend on npm.

Authored contract files in the repository and `api/openapi.yaml` remain the
canonical sources. Content under `packages/api/generated/` is a generated
publication projection and must not be edited or used as an authoring source.
The schemas are JSON Schema Draft 2020-12 documents derived from canonical
bundled OpenAPI component graphs during contract staging. The standalone
Factory Event and Factory Recording schemas preserve the event discriminator
as data and enforce each event type against its corresponding payload schema.
