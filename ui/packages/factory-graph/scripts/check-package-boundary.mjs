import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");
const allowedDependencies = new Set([
  "@you-agent-factory/client",
  "@you-agent-factory/components",
  "@you-agent-factory/factory-replay",
  "@xyflow/react",
  "react",
]);
const forbiddenRuntimeNames = [
  "EventSource",
  "WebSocket",
  "XMLHttpRequest",
  "fetch",
  "indexedDB",
  "localStorage",
  "sessionStorage",
];

async function runtimeFiles(directory, relativeRoot = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  return (
    await Promise.all(
      entries.map((entry) => {
        const relativePath = path.posix.join(relativeRoot, entry.name);
        return entry.isDirectory()
          ? runtimeFiles(path.join(directory, entry.name), relativePath)
          : relativePath.endsWith(".js")
            ? [relativePath]
            : [];
      }),
    )
  ).flat();
}

const manifest = JSON.parse(
  await readFile(path.join(packageRoot, "package.json"), "utf8"),
);
const declared = new Set([
  ...Object.keys(manifest.dependencies ?? {}),
  ...Object.keys(manifest.peerDependencies ?? {}),
]);
for (const dependency of declared) {
  if (!allowedDependencies.has(dependency)) {
    throw new Error(
      `[factory-graph-boundary] unsupported runtime dependency ${dependency}`,
    );
  }
}

for (const file of await runtimeFiles(distRoot)) {
  const source = await readFile(path.join(distRoot, file), "utf8");
  if (/dashboard|zustand|react-query|factory-editor|graph-editor/i.test(source)) {
    throw new Error(
      `[factory-graph-boundary] ${file} references host-owned graph state`,
    );
  }
  for (const name of forbiddenRuntimeNames) {
    if (new RegExp(`\\b${name}\\b`).test(source)) {
      throw new Error(
        `[factory-graph-boundary] ${file} uses forbidden ${name}`,
      );
    }
  }
}

process.stdout.write("Factory graph package boundary passed.\n");
