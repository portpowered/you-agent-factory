import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");
const forbiddenDependencies = new Set([
  "@xyflow/react",
  "react",
  "react-dom",
  "reactflow",
  "zustand",
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

function runtimeImports(source) {
  return [...source.matchAll(/(?:from\s+|import\s*\()(["'])([^"']+)\1/g)].map(
    (match) => match[2],
  );
}

function dependencyName(specifier) {
  const [first, second] = specifier.split("/");
  return first.startsWith("@") ? `${first}/${second}` : first;
}

async function packageFiles(directory, relativeRoot = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const relativePath = path.posix.join(relativeRoot, entry.name);
      return entry.isDirectory()
        ? packageFiles(path.join(directory, entry.name), relativePath)
        : [relativePath];
    }),
  );
  return files.flat();
}

export async function verifyPackageBoundary() {
  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  const declaredRuntimeDependencies = new Set(
    Object.keys(manifest.dependencies ?? {}),
  );
  const forbiddenDeclared = [...declaredRuntimeDependencies].filter((name) =>
    forbiddenDependencies.has(name),
  );
  if (forbiddenDeclared.length > 0) {
    throw new Error(
      `[client-boundary] forbidden dependencies: ${forbiddenDeclared.join(", ")}`,
    );
  }

  const packageFilesInDist = await packageFiles(distRoot);
  const fileSet = new Set(packageFilesInDist);
  const runtimeFiles = packageFilesInDist.filter((file) => file.endsWith(".js"));
  for (const file of runtimeFiles) {
    const source = await readFile(path.join(distRoot, file), "utf8");
    for (const name of forbiddenRuntimeNames) {
      if (new RegExp(`\\b${name}\\b`).test(source)) {
        throw new Error(`[client-boundary] ${file} uses forbidden ${name}`);
      }
    }
    if (/dashboard/i.test(source)) {
      throw new Error(`[client-boundary] ${file} references dashboard code`);
    }
    for (const specifier of runtimeImports(source)) {
      if (!specifier.startsWith(".") && !specifier.startsWith("..")) {
        const dependency = dependencyName(specifier);
        if (!declaredRuntimeDependencies.has(dependency)) {
          throw new Error(
            `[client-boundary] ${file} imports undeclared runtime dependency ${specifier}`,
          );
        }
        if (forbiddenDependencies.has(dependency)) {
          throw new Error(
            `[client-boundary] ${file} imports forbidden dependency ${specifier}`,
          );
        }
        continue;
      }
      const resolved = path.posix.normalize(
        path.posix.join(path.posix.dirname(file), specifier),
      );
      if (!fileSet.has(resolved)) {
        throw new Error(
          `[client-boundary] ${file} has missing runtime import ${resolved}`,
        );
      }
    }
  }
  return runtimeFiles.sort();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const files = await verifyPackageBoundary();
  process.stdout.write(
    `[client-boundary] verified ${files.length} runtime files\n`,
  );
}
