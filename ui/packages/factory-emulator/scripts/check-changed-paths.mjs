import { execFile } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const repositoryRoot = path.resolve(packageRoot, "../../..");

async function git(...args) {
  const { stdout } = await execFileAsync("git", args, {
    cwd: repositoryRoot,
    maxBuffer: 10 * 1024 * 1024,
  });
  return stdout
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

async function implementationBase() {
  const configured = process.env.FACTORY_EMULATOR_IMPLEMENTATION_BASE;
  if (configured) return configured;
  for (const candidate of ["origin/main", "main"]) {
    try {
      const [base] = await git("merge-base", "HEAD", candidate);
      if (base) return base;
    } catch {
      // Try the next local main reference.
    }
  }
  throw new Error(
    "Unable to derive the implementation base. Set FACTORY_EMULATOR_IMPLEMENTATION_BASE.",
  );
}

const base = await implementationBase();
const paths = new Set([
  ...(await git("diff", "--name-only", `${base}...HEAD`)),
  ...(await git("diff", "--name-only")),
  ...(await git("ls-files", "--others", "--exclude-standard")),
]);
const violations = [...paths].filter(
  (changedPath) => !changedPath.replaceAll("\\", "/").startsWith("ui/"),
);

if (violations.length > 0) {
  throw new Error(
    `Factory emulator changed-path boundary failed relative to ${base}:\n${violations
      .map((changedPath) => `- ${changedPath}`)
      .join("\n")}`,
  );
}

console.log(
  `Factory emulator changed-path boundary passed for ${paths.size} paths relative to ${base}.`,
);
