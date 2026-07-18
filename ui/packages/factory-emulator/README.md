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
