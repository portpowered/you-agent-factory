import { access, readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const defaultDistRoot = path.join(packageRoot, "dist");
const declarationExtensions = [".d.ts", ".d.mts", ".d.cts"];
const runtimeExtensions = [".js", ".mjs", ".cjs", ".jsx"];

function toPosixPath(filePath) {
  return filePath.replaceAll(path.sep, "/");
}

function isDeclarationFile(filePath) {
  return declarationExtensions.some((extension) =>
    filePath.endsWith(extension),
  );
}

function isRuntimeFile(filePath) {
  return runtimeExtensions.includes(path.extname(filePath));
}

function isWithinDirectory(filePath, directoryPath) {
  const relativePath = path.relative(directoryPath, filePath);
  return (
    relativePath === "" ||
    (!relativePath.startsWith("..") && !path.isAbsolute(relativePath))
  );
}

async function listFilesRecursively(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(directory, entry.name);
      if (!entry.isDirectory()) return [entryPath];
      return listFilesRecursively(entryPath);
    }),
  );
  return files
    .flat(Infinity)
    .filter((filePath) => typeof filePath === "string")
    .sort((left, right) => left.localeCompare(right));
}

async function fileExists(filePath) {
  try {
    await access(filePath);
    return true;
  } catch {
    return false;
  }
}

function stripKnownModuleExtension(filePath) {
  for (const extension of [
    ...declarationExtensions,
    ...runtimeExtensions,
    ".ts",
    ".tsx",
  ]) {
    if (filePath.endsWith(extension)) {
      return filePath.slice(0, -extension.length);
    }
  }
  return filePath;
}

function moduleSpecifierCandidates(filePath, specifier, extensions) {
  const specifierPath = specifier.split(/[?#]/, 1)[0];
  const basePath = path.resolve(path.dirname(filePath), specifierPath);
  const explicitExtension = path.extname(basePath);
  const extensionlessPath = stripKnownModuleExtension(basePath);

  if (explicitExtension && extensions.includes(explicitExtension)) {
    return [basePath];
  }

  return [
    ...extensions.map((extension) => `${extensionlessPath}${extension}`),
    ...extensions.map((extension) =>
      path.join(extensionlessPath, `index${extension}`),
    ),
  ];
}

async function resolveExistingCandidate(candidates) {
  for (const candidate of candidates) {
    if (await fileExists(candidate)) return candidate;
  }
  return null;
}

function extractRelativeModuleSpecifiers(source, filePath = "module.d.ts") {
  const specifiers = new Set();
  const sourceFile = ts.createSourceFile(
    filePath,
    source,
    ts.ScriptTarget.Latest,
    true,
    isDeclarationFile(filePath) ? ts.ScriptKind.TS : ts.ScriptKind.JS,
  );

  function record(specifier) {
    if (specifier.startsWith(".")) specifiers.add(specifier);
  }

  function visit(node) {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      record(node.moduleSpecifier.text);
    }

    if (
      ts.isImportTypeNode(node) &&
      ts.isLiteralTypeNode(node.argument) &&
      ts.isStringLiteral(node.argument.literal)
    ) {
      record(node.argument.literal.text);
    }

    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments.length === 1 &&
      ts.isStringLiteral(node.arguments[0])
    ) {
      record(node.arguments[0].text);
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);

  return [...specifiers].sort((left, right) => left.localeCompare(right));
}

export async function scanDeclarationRuntimeReferences({
  distRoot,
  declarationPaths,
} = {}) {
  const resolvedDistRoot = path.resolve(distRoot ?? defaultDistRoot);
  const allDeclarationPaths = (
    await listFilesRecursively(resolvedDistRoot)
  ).filter(isDeclarationFile);
  const files = declarationPaths
    ? [...declarationPaths].map((filePath) => path.resolve(filePath)).sort()
    : allDeclarationPaths;
  const violations = [];
  let referenceCount = 0;

  for (const declarationPath of files) {
    const source = await readFile(declarationPath, "utf8");
    const specifiers = extractRelativeModuleSpecifiers(source, declarationPath);
    referenceCount += specifiers.length;

    for (const specifier of specifiers) {
      const candidates = moduleSpecifierCandidates(
        declarationPath,
        specifier,
        runtimeExtensions,
      );
      const runtimePath = await resolveExistingCandidate(
        candidates.filter((candidate) =>
          isWithinDirectory(candidate, resolvedDistRoot),
        ),
      );
      if (runtimePath) continue;

      violations.push({
        declarationPath,
        specifier,
        candidates,
      });
    }
  }

  return {
    declarationCount: files.length,
    referenceCount,
    violations,
  };
}

async function scanDeclarationGraph({ distRoot, declarationPaths }) {
  const resolvedDistRoot = path.resolve(distRoot);
  const files =
    declarationPaths ??
    (await listFilesRecursively(resolvedDistRoot)).filter(isDeclarationFile);
  const violations = [];

  for (const declarationPath of files) {
    const source = await readFile(declarationPath, "utf8");
    for (const specifier of extractRelativeModuleSpecifiers(
      source,
      declarationPath,
    )) {
      const candidates = moduleSpecifierCandidates(
        declarationPath,
        specifier,
        declarationExtensions,
      );
      if (
        await resolveExistingCandidate(
          candidates.filter((candidate) =>
            isWithinDirectory(candidate, resolvedDistRoot),
          ),
        )
      ) {
        continue;
      }
      violations.push({ declarationPath, specifier, candidates });
    }
  }

  return violations;
}

async function scanRuntimeGraph({ distRoot }) {
  const resolvedDistRoot = path.resolve(distRoot);
  const runtimePaths = (await listFilesRecursively(resolvedDistRoot)).filter(
    isRuntimeFile,
  );
  const violations = [];

  for (const runtimePath of runtimePaths) {
    const source = await readFile(runtimePath, "utf8");
    for (const specifier of extractRelativeModuleSpecifiers(
      source,
      runtimePath,
    )) {
      const candidates = moduleSpecifierCandidates(
        runtimePath,
        specifier,
        runtimeExtensions,
      );
      if (
        await resolveExistingCandidate(
          candidates.filter((candidate) =>
            isWithinDirectory(candidate, resolvedDistRoot),
          ),
        )
      ) {
        continue;
      }
      violations.push({ runtimePath, specifier, candidates });
    }
  }

  return { runtimeCount: runtimePaths.length, violations };
}

function collectExportTargets(exports, exportPath = "exports") {
  if (typeof exports === "string") {
    return [{ exportPath, target: exports }];
  }
  if (!exports || typeof exports !== "object") return [];

  return Object.entries(exports).flatMap(([condition, target]) =>
    collectExportTargets(target, `${exportPath}[${JSON.stringify(condition)}]`),
  );
}

function formatPath(root, filePath) {
  return toPosixPath(path.relative(root, filePath));
}

function formatCandidates(root, candidates) {
  return candidates.map((candidate) => formatPath(root, candidate)).join(", ");
}

export function formatDeclarationRuntimeViolations({ distRoot, violations }) {
  const resolvedDistRoot = path.resolve(distRoot ?? defaultDistRoot);
  return violations.map(
    ({ declarationPath, specifier, candidates }) =>
      `- ${formatPath(resolvedDistRoot, declarationPath)} -> ${specifier} (runtime sibling not found; tried ${formatCandidates(resolvedDistRoot, candidates)})`,
  );
}

function formatRuntimeGraphViolations(distRoot, violations) {
  return violations.map(
    ({ runtimePath, specifier, candidates }) =>
      `- ${formatPath(distRoot, runtimePath)} -> ${specifier} (runtime module not found; tried ${formatCandidates(distRoot, candidates)})`,
  );
}

export async function verifyBundledPackage({
  packageRoot: root = packageRoot,
} = {}) {
  const resolvedPackageRoot = path.resolve(root);
  const distRoot = path.join(resolvedPackageRoot, "dist");
  const manifest = JSON.parse(
    await readFile(path.join(resolvedPackageRoot, "package.json"), "utf8"),
  );
  const diagnostics = [];
  const exportTargets = collectExportTargets(manifest.exports);
  const typeTargets = exportTargets.filter(({ exportPath }) =>
    exportPath.endsWith('["types"]'),
  );
  const runtimeTargets = exportTargets.filter(
    ({ exportPath }) =>
      exportPath.endsWith('["import"]') || exportPath.endsWith('["default"]'),
  );

  for (const { exportPath, target } of exportTargets) {
    if (target.endsWith(".css")) continue;
    const targetPath = path.resolve(resolvedPackageRoot, target);
    if (!(await fileExists(targetPath))) {
      diagnostics.push(
        `missing package export target ${exportPath}: ${toPosixPath(target)}`,
      );
    }
  }

  for (const { exportPath, target } of runtimeTargets) {
    if (isDeclarationFile(target)) {
      diagnostics.push(
        `runtime export target must be JavaScript ${exportPath}: ${toPosixPath(target)}`,
      );
    }
  }

  const runtimeTargetByExportPath = new Map(
    runtimeTargets.map(({ exportPath, target }) => [
      exportPath.replace(/\[(?:"import"|"default")\]$/, ""),
      target,
    ]),
  );
  for (const { exportPath, target } of typeTargets) {
    const exportBase = exportPath.replace('["types"]', "");
    const runtimeTarget = runtimeTargetByExportPath.get(exportBase);
    if (!runtimeTarget) {
      diagnostics.push(
        `types export has no runtime sibling ${exportPath}: ${toPosixPath(target)}`,
      );
    }
  }

  const declarationPaths = (await listFilesRecursively(distRoot)).filter(
    isDeclarationFile,
  );
  diagnostics.push(
    ...formatDeclarationRuntimeViolations({
      distRoot,
      violations: (
        await scanDeclarationRuntimeReferences({
          distRoot,
          declarationPaths,
        })
      ).violations,
    }),
  );
  diagnostics.push(
    ...(await scanDeclarationGraph({ distRoot, declarationPaths })).map(
      ({ declarationPath, specifier, candidates }) =>
        `- ${formatPath(distRoot, declarationPath)} -> ${specifier} (declaration target not found; tried ${formatCandidates(distRoot, candidates)})`,
    ),
  );

  const runtimeReport = await scanRuntimeGraph({ distRoot });
  diagnostics.push(
    ...formatRuntimeGraphViolations(distRoot, runtimeReport.violations),
  );

  if (diagnostics.length > 0) {
    return {
      declarationCount: declarationPaths.length,
      runtimeCount: runtimeReport.runtimeCount,
      diagnostics,
    };
  }

  return {
    declarationCount: declarationPaths.length,
    runtimeCount: runtimeReport.runtimeCount,
    diagnostics: [],
  };
}

export { extractRelativeModuleSpecifiers, moduleSpecifierCandidates };

async function main() {
  const args = process.argv.slice(2);
  const strict = args.includes("--strict");
  const distRootArgumentIndex = args.indexOf("--dist-root");
  const distRoot =
    distRootArgumentIndex >= 0
      ? path.resolve(args[distRootArgumentIndex + 1])
      : defaultDistRoot;

  if (strict) {
    const report = await scanDeclarationRuntimeReferences({ distRoot });
    if (report.violations.length > 0) {
      process.stderr.write(
        `${[
          "[components-declaration-runtime] declaration/runtime check failed:",
          ...formatDeclarationRuntimeViolations({
            distRoot,
            violations: report.violations,
          }),
          `Checked ${report.declarationCount} declarations and ${report.referenceCount} relative references.`,
        ].join("\n")}\n`,
      );
      process.exitCode = 1;
      return;
    }

    process.stdout.write(
      `[components-declaration-runtime] strict check passed (${report.declarationCount} declarations, ${report.referenceCount} relative references).\n`,
    );
    return;
  }

  const report = await verifyBundledPackage();
  if (report.diagnostics.length > 0) {
    process.stderr.write(
      `${[
        "[components-declaration-runtime] package verification failed:",
        ...report.diagnostics,
      ].join("\n")}\n`,
    );
    process.exitCode = 1;
    return;
  }

  process.stdout.write(
    `[components-declaration-runtime] package verification passed (${report.declarationCount} declarations, ${report.runtimeCount} runtime modules; every declaration reference resolves).\n`,
  );
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
