import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const prohibitedPackage = "@you-agent-factory/packaged-factories";
const sourceExtensions = new Set([".ts", ".tsx", ".js", ".jsx"]);

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...(await sourceFiles(target)));
    else if (sourceExtensions.has(path.extname(entry.name))) files.push(target);
  }
  return files;
}

const manifest = JSON.parse(
  await readFile(path.join(uiRoot, "package.json"), "utf8"),
);
const dependencies = [
  ...Object.keys(manifest.dependencies ?? {}),
  ...Object.keys(manifest.devDependencies ?? {}),
  ...Object.keys(manifest.optionalDependencies ?? {}),
];
const violations = dependencies.includes(prohibitedPackage)
  ? [`package.json declares ${prohibitedPackage}`]
  : [];

for (const file of await sourceFiles(path.join(uiRoot, "src"))) {
  const text = await readFile(file, "utf8");
  const importPattern = new RegExp(
    `(?:from\\s*|import\\s*\\()(["'])${prohibitedPackage.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`,
  );
  if (importPattern.test(text)) {
    violations.push(`${path.relative(uiRoot, file)} imports ${prohibitedPackage}`);
  }
}

if (violations.length > 0) {
  process.stderr.write(`Packaged Factories dashboard boundary failed:\n- ${violations.join("\n- ")}\n`);
  process.exitCode = 1;
} else {
  process.stdout.write("Packaged Factories dashboard boundary passed.\n");
}
