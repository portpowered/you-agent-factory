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
  "react-dom",
]);
const forbiddenDependencies = new Set(["@tanstack/react-query", "zustand"]);
const forbiddenRuntimeNames = [
  "EventSource",
  "WebSocket",
  "XMLHttpRequest",
  "fetch",
  "indexedDB",
  "localStorage",
  "sessionStorage",
];
const editableGraphNames = [
  "addEdge",
  "applyEdgeChanges",
  "applyNodeChanges",
  "useEdgesState",
  "useNodesState",
];

function runtimeImports(source) {
  return [...source.matchAll(/(?:from\s+|import\s*\()(["'])([^"']+)\1/g)].map(
    (match) => match[2],
  );
}

function dependencyName(specifier) {
  const [first, second] = specifier.split("/");
  return first.startsWith("@") ? `${first}/${second}` : first;
}

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

export async function verifyPackageBoundary() {
  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  const declared = new Set([
    ...Object.keys(manifest.dependencies ?? {}),
    ...Object.keys(manifest.peerDependencies ?? {}),
  ]);
  const unexpected = [...declared].filter(
    (name) => !allowedDependencies.has(name),
  );
  const forbidden = [...declared].filter((name) =>
    forbiddenDependencies.has(name),
  );
  if (unexpected.length > 0 || forbidden.length > 0) {
    throw new Error(
      `[factory-visualizers-boundary] forbidden package dependencies: ${[...new Set([...unexpected, ...forbidden])].join(", ")}`,
    );
  }

  const files = await runtimeFiles(distRoot);
  for (const file of files) {
    const source = await readFile(path.join(distRoot, file), "utf8");
    if (/dashboard|factory-editor|graph-editor/i.test(source)) {
      throw new Error(
        `[factory-visualizers-boundary] ${file} references dashboard or editor code`,
      );
    }
    for (const name of [...forbiddenRuntimeNames, ...editableGraphNames]) {
      if (new RegExp(`\\b${name}\\b`).test(source)) {
        throw new Error(
          `[factory-visualizers-boundary] ${file} uses forbidden ${name}`,
        );
      }
    }
    for (const specifier of runtimeImports(source)) {
      if (specifier.startsWith(".") || specifier.startsWith("..")) continue;
      const dependency = dependencyName(specifier);
      if (!declared.has(dependency)) {
        throw new Error(
          `[factory-visualizers-boundary] ${file} imports undeclared dependency ${specifier}`,
        );
      }
    }
  }
  return files.sort();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const files = await verifyPackageBoundary();
  process.stdout.write(
    `[factory-visualizers-boundary] verified ${files.length} runtime files\n`,
  );
}
