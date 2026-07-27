import { access, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const surfacePath = path.join(
  uiRoot,
  "src",
  "features",
  "workflow-activity",
  "components",
  "react-flow-current-activity-card-surface.tsx",
);
const retiredDashboardFiles = [
  "src/features/dashboard/components/topology-replay/hosted-topology-replay.tsx",
  "src/features/dashboard/hooks/topology-replay/use-hosted-topology-replay-adapter.ts",
  "src/features/dashboard/messages/hosted-topology-replay.ts",
].map((relativePath) => path.join(uiRoot, relativePath));

const surface = await readFile(surfacePath, "utf8");
for (const required of ["CurrentActivityGraphViewport", "NODE_TYPES"]) {
  if (!surface.includes(required)) {
    throw new Error(
      `[factory-graph-renderer-path] workflow activity must render through ${required}`,
    );
  }
}
for (const retired of ["HostedTopologyReplay", "factoryTopologyNodeId"]) {
  if (surface.includes(retired)) {
    throw new Error(
      `[factory-graph-renderer-path] workflow activity must not restore ${retired}`,
    );
  }
}

for (const retiredFile of retiredDashboardFiles) {
  try {
    await access(retiredFile);
  } catch (error) {
    if (error && typeof error === "object" && error.code === "ENOENT") {
      continue;
    }
    throw error;
  }
  throw new Error(
    `[factory-graph-renderer-path] retired dashboard renderer remains at ${path.relative(uiRoot, retiredFile)}`,
  );
}

process.stdout.write("Factory graph renderer-path check passed.\n");
