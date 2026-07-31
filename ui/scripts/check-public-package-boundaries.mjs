import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultPackagesRoot = path.resolve(scriptDir, "..", "packages");
const sourceExtensions = new Set([".js", ".jsx", ".ts", ".tsx"]);
const skippedSourceSuffixes = [
  ".test.js",
  ".test.jsx",
  ".test.ts",
  ".test.tsx",
  ".stories.js",
  ".stories.jsx",
  ".stories.ts",
  ".stories.tsx",
  "-story-support.ts",
  "-story-support.tsx",
  "overlay-storybook-play.ts",
  "compile-package-token-styles.ts",
  "vite-aliases.ts",
  "vitest.setup.ts",
];
const publicPackagePrefix = "@you-agent-factory/";

export const publicPackagePolicies = {
  client: {
    packageName: "@you-agent-factory/client",
    allowedRuntimeDependencies: new Set(["ajv", "ajv-formats", "marked"]),
    allowedPublicPackageDependencies: new Set(),
  },
  "factory-replay": {
    packageName: "@you-agent-factory/factory-replay",
    allowedRuntimeDependencies: new Set(["@you-agent-factory/client"]),
    allowedPublicPackageDependencies: new Set(["@you-agent-factory/client"]),
  },
  "factory-graph": {
    packageName: "@you-agent-factory/factory-graph",
    allowedRuntimeDependencies: new Set([
      "@you-agent-factory/client",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-replay",
      "react",
      "@xyflow/react",
    ]),
    allowedPublicPackageDependencies: new Set([
      "@you-agent-factory/client",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-replay",
    ]),
  },
  "factory-emulator": {
    packageName: "@you-agent-factory/factory-emulator",
    allowedRuntimeDependencies: new Set([
      "@you-agent-factory/client",
      "ajv",
      "ajv-formats",
    ]),
    allowedPublicPackageDependencies: new Set(["@you-agent-factory/client"]),
    prohibitedSourcePatterns: [
      ["browser timer", /\b(?:setInterval|setTimeout)\s*\(/],
      ["Web Worker", /\b(?:SharedWorker|Worker)\s*\(/],
    ],
  },
  components: {
    packageName: "@you-agent-factory/components",
    allowedRuntimeDependencies: new Set([
      "@radix-ui/react-collapsible",
      "@radix-ui/react-dialog",
      "@radix-ui/react-popover",
      "@radix-ui/react-scroll-area",
      "@radix-ui/react-select",
      "@radix-ui/react-slot",
      "@testing-library/react",
      "@testing-library/user-event",
      "@xyflow/react",
      "react",
      "react-dom",
      "recharts",
    ]),
    allowedPublicPackageDependencies: new Set(),
  },
  "factory-visualizers": {
    packageName: "@you-agent-factory/factory-visualizers",
    allowedRuntimeDependencies: new Set([
      "@you-agent-factory/client",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-graph",
      "@you-agent-factory/factory-replay",
      "@xyflow/react",
      "react",
      "react-dom",
    ]),
    allowedPublicPackageDependencies: new Set([
      "@you-agent-factory/client",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-graph",
      "@you-agent-factory/factory-replay",
    ]),
  },
};

const prohibitedRuntimePatterns = [
  [
    "network transport",
    /\b(?:EventSource|WebSocket|XMLHttpRequest)\b|\bfetch\s*\(/,
  ],
  ["browser persistence", /\b(?:indexedDB|localStorage|sessionStorage)\b/],
];

function toPosixPath(filePath) {
  return filePath.split(path.sep).join(path.posix.sep);
}

function dependencyName(specifier) {
  const [first, second] = specifier.split("/");
  return first.startsWith("@") ? `${first}/${second}` : first;
}

function isWithinDirectory(candidatePath, directory) {
  const relativePath = path.relative(directory, candidatePath);
  return (
    relativePath === "" ||
    (!relativePath.startsWith("..") && !path.isAbsolute(relativePath))
  );
}

function shouldScanSource(filePath) {
  return (
    sourceExtensions.has(path.extname(filePath)) &&
    !skippedSourceSuffixes.some((suffix) => filePath.endsWith(suffix))
  );
}

async function collectSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await collectSourceFiles(entryPath)));
    } else if (shouldScanSource(entryPath)) {
      files.push(entryPath);
    }
  }

  return files;
}

function scriptKind(filePath) {
  const extension = path.extname(filePath);
  if (extension === ".tsx") return ts.ScriptKind.TSX;
  if (extension === ".jsx") return ts.ScriptKind.JSX;
  if (extension === ".js") return ts.ScriptKind.JS;
  return ts.ScriptKind.TS;
}

function collectModuleSpecifiers(sourceText, filePath) {
  const sourceFile = ts.createSourceFile(
    filePath,
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    scriptKind(filePath),
  );
  const specifiers = [];

  function visit(node) {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      specifiers.push(node.moduleSpecifier.text);
    }
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length === 1 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      specifiers.push(node.arguments[0].text);
    }
    ts.forEachChild(node, visit);
  }

  visit(sourceFile);
  return specifiers;
}

function runtimeDependencies(manifest) {
  return new Set([
    ...Object.keys(manifest.dependencies ?? {}),
    ...Object.keys(manifest.optionalDependencies ?? {}),
    ...Object.keys(manifest.peerDependencies ?? {}),
  ]);
}

function manifestViolations(packageKey, policy, manifest) {
  const violations = [];
  if (manifest.name !== policy.packageName) {
    violations.push({
      packageKey,
      kind: "package-identity",
      message: `expected package name ${policy.packageName}, found ${manifest.name ?? "<missing>"}`,
    });
  }

  for (const dependency of runtimeDependencies(manifest)) {
    if (!policy.allowedRuntimeDependencies.has(dependency)) {
      violations.push({
        packageKey,
        kind: "prohibited-runtime-dependency",
        message: `${policy.packageName} declares unsupported runtime dependency ${dependency}`,
      });
    }
    if (
      dependency.startsWith(publicPackagePrefix) &&
      !policy.allowedPublicPackageDependencies.has(dependency)
    ) {
      violations.push({
        packageKey,
        kind: "public-package-direction",
        message: `${policy.packageName} must not depend on ${dependency}`,
      });
    }
  }

  return violations;
}

async function sourceViolations(packageKey, policy, packageRoot, manifest) {
  const sourceRoot = path.join(packageRoot, "src");
  const declaredRuntimeDependencies = runtimeDependencies(manifest);
  const violations = [];

  for (const filePath of (await collectSourceFiles(sourceRoot)).sort()) {
    const sourceText = await readFile(filePath, "utf8");
    const relativeFilePath = toPosixPath(path.relative(packageRoot, filePath));

    for (const [label, pattern] of prohibitedRuntimePatterns) {
      if (pattern.test(sourceText)) {
        violations.push({
          packageKey,
          relativeFilePath,
          kind: "prohibited-runtime-ownership",
          message: `${relativeFilePath} contains prohibited ${label}`,
        });
      }
    }
    for (const [label, pattern] of policy.prohibitedSourcePatterns ?? []) {
      if (pattern.test(sourceText)) {
        violations.push({
          packageKey,
          relativeFilePath,
          kind: "prohibited-runtime-ownership",
          message: `${relativeFilePath} contains prohibited ${label}`,
        });
      }
    }

    for (const specifier of collectModuleSpecifiers(sourceText, filePath)) {
      if (specifier.startsWith(".")) {
        const resolvedPath = path.resolve(path.dirname(filePath), specifier);
        if (!isWithinDirectory(resolvedPath, packageRoot)) {
          violations.push({
            packageKey,
            relativeFilePath,
            kind: "package-source-escape",
            message: `${relativeFilePath} imports outside its package: ${specifier}`,
          });
        }
        continue;
      }

      const dependency = dependencyName(specifier);
      if (!declaredRuntimeDependencies.has(dependency)) {
        violations.push({
          packageKey,
          relativeFilePath,
          kind: "undeclared-runtime-dependency",
          message: `${relativeFilePath} imports undeclared runtime dependency ${specifier}`,
        });
      }
      if (
        dependency.startsWith(publicPackagePrefix) &&
        !policy.allowedPublicPackageDependencies.has(dependency)
      ) {
        violations.push({
          packageKey,
          relativeFilePath,
          kind: "public-package-direction",
          message: `${relativeFilePath} must not import ${specifier}`,
        });
      }
    }
  }

  return violations;
}

export async function inspectPublicPackageBoundaries(
  packagesRoot = defaultPackagesRoot,
) {
  const violations = [];

  for (const [packageKey, policy] of Object.entries(publicPackagePolicies)) {
    const packageRoot = path.join(packagesRoot, packageKey);
    const manifest = JSON.parse(
      await readFile(path.join(packageRoot, "package.json"), "utf8"),
    );
    violations.push(...manifestViolations(packageKey, policy, manifest));
    violations.push(
      ...(await sourceViolations(packageKey, policy, packageRoot, manifest)),
    );
  }

  return { packagesRoot, violations };
}

async function main() {
  const packagesRoot = process.env.AGENT_FACTORY_PUBLIC_PACKAGES_ROOT
    ? path.resolve(process.env.AGENT_FACTORY_PUBLIC_PACKAGES_ROOT)
    : defaultPackagesRoot;
  const report = await inspectPublicPackageBoundaries(packagesRoot);

  if (report.violations.length === 0) {
    process.stdout.write("Public package dependency-boundary check passed.\n");
    return;
  }

  process.stderr.write("Public package dependency-boundary check failed:\n");
  for (const violation of report.violations) {
    process.stderr.write(`- [${violation.kind}] ${violation.message}\n`);
  }
  process.exitCode = 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
