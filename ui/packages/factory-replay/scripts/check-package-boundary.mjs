import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");
const forbiddenDependencies = new Set(["react", "react-dom", "zustand"]);
const forbiddenRuntimeNames = [
  "EventSource",
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

async function runtimeFiles(directory, relativeRoot = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const relativePath = path.posix.join(relativeRoot, entry.name);
      return entry.isDirectory()
        ? runtimeFiles(path.join(directory, entry.name), relativePath)
        : relativePath.endsWith(".js")
          ? [relativePath]
          : [];
    }),
  );
  return files.flat();
}

export async function verifyPackageBoundary() {
  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  const declaredRuntimeDependencies = new Set([
    ...Object.keys(manifest.dependencies ?? {}),
    ...Object.keys(manifest.optionalDependencies ?? {}),
  ]);
  const forbiddenDeclared = [...declaredRuntimeDependencies].filter((name) =>
    forbiddenDependencies.has(name),
  );
  if (forbiddenDeclared.length > 0) {
    throw new Error(
      `[factory-replay-boundary] forbidden dependencies: ${forbiddenDeclared.join(", ")}`,
    );
  }

  const files = await runtimeFiles(distRoot);
  const fileSet = new Set(files);
  for (const file of files) {
    const source = await readFile(path.join(distRoot, file), "utf8");
    for (const name of forbiddenRuntimeNames) {
      if (new RegExp(`\\b${name}\\b`).test(source)) {
        throw new Error(
          `[factory-replay-boundary] ${file} uses forbidden ${name}`,
        );
      }
    }
    if (/dashboard/i.test(source)) {
      throw new Error(
        `[factory-replay-boundary] ${file} references dashboard code`,
      );
    }
    for (const specifier of runtimeImports(source)) {
      if (!specifier.startsWith(".") && !specifier.startsWith("..")) {
        const dependency = dependencyName(specifier);
        if (!declaredRuntimeDependencies.has(dependency)) {
          throw new Error(
            `[factory-replay-boundary] ${file} imports undeclared runtime dependency ${specifier}`,
          );
        }
        if (forbiddenDependencies.has(dependency)) {
          throw new Error(
            `[factory-replay-boundary] ${file} imports forbidden dependency ${specifier}`,
          );
        }
        continue;
      }
      const resolved = path.posix.normalize(
        path.posix.join(path.posix.dirname(file), specifier),
      );
      if (!fileSet.has(resolved)) {
        throw new Error(
          `[factory-replay-boundary] ${file} has missing runtime import ${resolved}`,
        );
      }
    }
  }
  return files.sort();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const files = await verifyPackageBoundary();
  process.stdout.write(
    `[factory-replay-boundary] verified ${files.length} runtime files\n`,
  );
}
