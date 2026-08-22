import { execFile } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const bunExecutable = process.env.BUN_EXECUTABLE ?? "bun";

function outputFrom(error) {
  return [error.stdout, error.stderr, error.message].filter(Boolean).join("\n");
}

function importSpecifiers(source) {
  const specifiers = [];
  const pattern = /(?:from|import)\s*["']([^"']+)["']/g;
  let match = pattern.exec(source);
  while (match !== null) {
    specifiers.push(match[1]);
    match = pattern.exec(source);
  }
  return specifiers;
}

async function reachableExternalSpecifiers(entry) {
  const seen = new Set();
  const external = new Set();
  const queue = [entry];

  while (queue.length > 0) {
    const current = queue.pop();
    if (current === undefined || seen.has(current)) {
      continue;
    }
    seen.add(current);

    const source = await readFile(
      path.join(packageRoot, "dist", current),
      "utf8",
    );
    for (const specifier of importSpecifiers(source)) {
      if (specifier.startsWith(".")) {
        queue.push(path.normalize(path.join(path.dirname(current), specifier)));
      } else {
        external.add(specifier);
      }
    }
  }

  return [...external];
}

const manifest = JSON.parse(
  await readFile(path.join(packageRoot, "package.json"), "utf8"),
);
const visualStateExport = manifest.exports?.["./visual-state"];
if (
  JSON.stringify(visualStateExport) !==
  JSON.stringify({
    types: "./dist/visual-state.d.ts",
    import: "./dist/visual-state.js",
    default: "./dist/visual-state.js",
  })
) {
  throw new Error(
    `[factory-graph-visual-state-runtime] unexpected ./visual-state export: ${JSON.stringify(visualStateExport)}`,
  );
}

const external = await reachableExternalSpecifiers("visual-state.js");
const workspaceImports = external.filter((specifier) =>
  specifier.startsWith("@you-agent-factory/"),
);
if (workspaceImports.length > 0) {
  throw new Error(
    `[factory-graph-visual-state-runtime] visual-state reaches workspace packages: ${workspaceImports.join(", ")}`,
  );
}

const consumerRoot = await mkdtemp(
  path.join(tmpdir(), "you-factory-graph-visual-state-consumer-"),
);

try {
  const consumerPackageRoot = path.join(
    consumerRoot,
    "node_modules",
    "@you-agent-factory",
    "factory-graph",
  );
  await mkdir(consumerPackageRoot, { recursive: true });
  await cp(
    path.join(packageRoot, "dist"),
    path.join(consumerPackageRoot, "dist"),
    {
      recursive: true,
    },
  );
  await cp(
    path.join(packageRoot, "package.json"),
    path.join(consumerPackageRoot, "package.json"),
  );
  await writeFile(
    path.join(consumerRoot, "package.json"),
    JSON.stringify({ private: true, type: "module" }),
  );

  const probe = `
import { factoryGraphVisualNestedAccentRole } from "@you-agent-factory/factory-graph/visual-state";

const expected = {
  quiet: "neutral",
  waiting: "info",
  active: "warning",
  success: "success",
  danger: "danger",
};

for (const [status, role] of Object.entries(expected)) {
  if (factoryGraphVisualNestedAccentRole(status) !== role) {
    throw new Error(
      "unexpected nested accent role for " +
        status +
        ": " +
        factoryGraphVisualNestedAccentRole(status),
    );
  }
}
`;

  const { stdout, stderr } = await execFileAsync(
    bunExecutable,
    ["--eval", probe],
    {
      cwd: consumerRoot,
      maxBuffer: 10 * 1024 * 1024,
      windowsHide: true,
    },
  );
  process.stdout.write([stdout, stderr].filter(Boolean).join("\n"));
  process.stdout.write(
    "[factory-graph-visual-state-runtime] passed (clean consumer import).\n",
  );
} catch (error) {
  process.stderr.write(
    `${[
      "[factory-graph-visual-state-runtime] clean consumer resolution check failed:",
      outputFrom(error),
    ]
      .filter(Boolean)
      .join("\n")}\n`,
  );
  process.exitCode = Number.isInteger(error?.code) ? error.code : 1;
} finally {
  await rm(consumerRoot, { force: true, recursive: true });
}
