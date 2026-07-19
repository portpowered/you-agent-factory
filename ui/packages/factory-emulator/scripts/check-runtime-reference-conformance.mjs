import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const runtime = await import(
  pathToFileURL(path.join(packageRoot, "dist", "index.js")).href
);

const references = runtime.loadFactoryEmulatorRuntimeReferences();
const firstRun = await Promise.all(
  references.map((reference) =>
    runtime.compareFactoryEmulatorRuntimeReference(reference),
  ),
);
const secondRun = await Promise.all(
  references.map((reference) =>
    runtime.compareFactoryEmulatorRuntimeReference(reference),
  ),
);

for (const [index, result] of firstRun.entries()) {
  if (!result.matches) {
    throw new Error(
      `[factory-emulator-runtime-references] ${result.divergence.fixture} diverged at tick ${result.divergence.logicalTick} on ${result.divergence.surface}: expected ${JSON.stringify(result.divergence.expected)}, actual ${JSON.stringify(result.divergence.actual)}`,
    );
  }
  if (JSON.stringify(result) !== JSON.stringify(secondRun[index])) {
    throw new Error(
      `[factory-emulator-runtime-references] ${references[index]?.id ?? "unknown"} was not deterministic across consecutive runs.`,
    );
  }
}

process.stdout.write(
  `[factory-emulator-runtime-references] verified ${references.length} frozen references from compiled package output\n`,
);
