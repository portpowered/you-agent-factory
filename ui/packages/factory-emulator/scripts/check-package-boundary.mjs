import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const sourceRoot = path.join(packageRoot, "src");
const allowedRuntimeDependencies = new Set([
  "@you-agent-factory/client",
  "ajv",
  "ajv-formats",
]);
const prohibitedDependencies = new Set([
  "@you-agent-factory/components",
  "@you-agent-factory/factory-replay",
  "@you-agent-factory/factory-visualizers",
  "react",
  "react-dom",
  "zustand",
]);
const prohibitedRuntimeNames = [
  ["fetch", /\bfetch\s*\(/],
  ["EventSource", /\bEventSource\b/],
  ["browser storage", /\b(?:localStorage|sessionStorage)\b/],
  ["browser timer", /\b(?:setInterval|setTimeout)\s*\(/],
];

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map((entry) => {
      const absolute = path.join(directory, entry.name);
      return entry.isDirectory()
        ? sourceFiles(absolute)
        : entry.name.endsWith(".ts") && !entry.name.endsWith(".test.ts")
          ? [absolute]
          : [];
    }),
  );
  return nested.flat();
}

function dependencyName(specifier) {
  const [first, second] = specifier.split("/");
  return first.startsWith("@") ? `${first}/${second}` : first;
}

const manifest = JSON.parse(
  await readFile(path.join(packageRoot, "package.json"), "utf8"),
);
const declaredRuntimeDependencies = new Set([
  ...Object.keys(manifest.dependencies ?? {}),
  ...Object.keys(manifest.peerDependencies ?? {}),
]);
const violations = [];

for (const dependency of declaredRuntimeDependencies) {
  if (!allowedRuntimeDependencies.has(dependency)) {
    violations.push(`manifest declares unsupported dependency ${dependency}`);
  }
  if (prohibitedDependencies.has(dependency)) {
    violations.push(`manifest declares prohibited dependency ${dependency}`);
  }
}

for (const file of await sourceFiles(sourceRoot)) {
  const source = await readFile(file, "utf8");
  const relative = path.relative(packageRoot, file).replaceAll("\\", "/");
  const imports = [
    ...source.matchAll(/(?:from\s+|import\s*\()(["'])([^"']+)\1/g),
  ].map((match) => match[2]);
  for (const specifier of imports) {
    if (specifier.startsWith(".")) {
      const resolved = path.resolve(path.dirname(file), specifier);
      if (!resolved.startsWith(`${packageRoot}${path.sep}`)) {
        violations.push(
          `${relative} imports outside the package: ${specifier}`,
        );
      }
      continue;
    }
    if (specifier.startsWith("node:")) continue;
    const dependency = dependencyName(specifier);
    if (!declaredRuntimeDependencies.has(dependency)) {
      violations.push(`${relative} imports undeclared dependency ${specifier}`);
    }
  }
  for (const [label, pattern] of prohibitedRuntimeNames) {
    if (pattern.test(source)) {
      violations.push(`${relative} uses prohibited ${label} runtime`);
    }
  }
  if (source.includes("/src/features/") || source.includes("/src/api/")) {
    violations.push(`${relative} reaches into dashboard source`);
  }
}

if (violations.length > 0) {
  throw new Error(
    `Factory emulator package boundary check failed:\n${violations
      .map((violation) => `- ${violation}`)
      .join("\n")}`,
  );
}

console.log("Factory emulator package boundary check passed.");
