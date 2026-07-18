# Factory emulator

`@you-agent-factory/factory-emulator` is a transport-neutral, non-React package
for deterministic Factory emulator contracts. Scenario parsing validates both
the package-local schema and references to a caller-supplied UI client
`FactoryDefinition`.

```ts
import factory from "./factory.json" with { type: "json" };
import scenario from "./scenario.json" with { type: "json" };
import { parseFactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";

const parsed = parseFactoryEmulatorScenario(scenario, factory);
```

The parser returns a detached scenario value. `safeParseFactoryEmulatorScenario`
returns all structure and semantic diagnostics without partially accepting an
invalid scenario.

Factories can be preflighted against the deterministic v1 execution subset
before any event history is emitted:

```ts
import {
  inspectFactoryEmulatorCompatibility,
  writeFactoryEventsIfCompatible,
} from "@you-agent-factory/factory-emulator";

const compatibility = inspectFactoryEmulatorCompatibility(factory);
if (!compatibility.supported) {
  console.error(compatibility.diagnostics);
}

// Incompatible Factories return every diagnostic without calling sink.write.
await writeFactoryEventsIfCompatible(factory, eventBatch, sink);
```

The inspector treats an omitted orchestrator as the documented Petri default,
does not mutate or retain the Factory, and reports stable codes with paths into
the caller-supplied UI client `FactoryDefinition`.
