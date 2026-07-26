# `@you-agent-factory/model-providers`

`@you-agent-factory/model-providers` publishes the versioned first-party
model-provider catalog and its explicit JSON Schemas as typed, data-only
artifacts. It performs no provider registration, discovery, environment reads,
or other runtime initialization.

## Public exports

| Export | Data |
| --- | --- |
| `@you-agent-factory/model-providers/catalog` | Sorted Provider Catalog JSON |
| `@you-agent-factory/model-providers/schemas/provider-manifest` | Provider Manifest JSON Schema |
| `@you-agent-factory/model-providers/schemas/provider-catalog` | Provider Catalog JSON Schema |
| `@you-agent-factory/model-providers/manifest` | Commit-independent development metadata and artifact SHA-256 hashes; publication may add source provenance |
| `@you-agent-factory/model-providers/types` | `ProviderCatalog`, `ProviderManifest`, and supporting TypeScript types |

The JSON exports provide TypeScript declarations through their package export
conditions. With a TypeScript configuration that uses Node's modern module
resolution, they can be imported directly:

```ts
import catalog from "@you-agent-factory/model-providers/catalog" with {
  type: "json",
};
import type {
  ProviderCatalog,
  ProviderManifest,
} from "@you-agent-factory/model-providers/types";

const published: ProviderCatalog = catalog;
const firstProvider: ProviderManifest = published.providers[0];
```

Tools that need the exact published bytes can resolve and read an export:

```js
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const catalogURL = import.meta.resolve(
  "@you-agent-factory/model-providers/catalog",
);
const catalog = JSON.parse(
  await readFile(fileURLToPath(catalogURL), "utf8"),
);
```

Only the declared package subpaths are supported. The authored provider YAML
files and Go embedding are repository concerns and are not included in the npm
package. Generated data and declarations must be refreshed through the
repository's provider package generation commands rather than edited directly.
