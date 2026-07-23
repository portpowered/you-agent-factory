# `@you-agent-factory/packaged-factories`

`@you-agent-factory/packaged-factories` publishes the first-party Factory
catalog as JSON and YAML data. Its exports are stable artifact locators, not
JavaScript module APIs.

## Public exports

| Export | Raw artifact |
| --- | --- |
| `@you-agent-factory/packaged-factories/manifest` | Catalog manifest JSON with Factory slugs, artifact locators, hashes, descriptions, and invocation examples |
| `@you-agent-factory/packaged-factories/schemas/factory.json` | Factory JSON Schema serialized as JSON |
| `@you-agent-factory/packaged-factories/schemas/factory.yaml` | The equivalent Factory JSON Schema serialized as YAML |
| `@you-agent-factory/packaged-factories/factories/<slug>.json` | Flattened JSON Factory named by a manifest entry |
| `@you-agent-factory/packaged-factories/factories/<slug>.yaml` | The equivalent flattened YAML Factory named by a manifest entry |

The supported Factory slugs are declared by the package's manifest. Only these
public subpaths are supported. Authored Factory sources, prompts, scripts, and
package-internal generated paths are not part of the npm contract.

## Reading an artifact

Resolve a public subpath through the installed package, then read the resulting
file as data. For example, in Node.js:

```js
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const manifestURL = import.meta.resolve(
  "@you-agent-factory/packaged-factories/manifest",
);
const manifest = JSON.parse(
  await readFile(fileURLToPath(manifestURL), "utf8"),
);

const factoryURL = import.meta.resolve(
  `@you-agent-factory/packaged-factories/factories/${manifest.factories[0].slug}.json`,
);
const factory = JSON.parse(
  await readFile(fileURLToPath(factoryURL), "utf8"),
);
```

Use the same resolution-and-read pattern for YAML artifacts, passing their
contents to the YAML reader chosen by your application.

## Data-only package

This package contains no JavaScript runtime, executable, dependency, lifecycle
hook, or install-time behavior. It does not provide validators or generated
clients. Applications choose their own JSON Schema and YAML tooling and consume
the exported files strictly as data.
