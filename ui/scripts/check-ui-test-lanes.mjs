import { readdirSync, readFileSync } from "node:fs";
import { relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

import {
  BUN_UNIT_TEST_MARKER_PATTERN,
  BUN_UNIT_TEST_PATH_PATTERN,
} from "./ui-test-lane-contract.mjs";

const testFilePattern = /\.(?:test|spec)\.(?:[cm]?[jt]sx?)$/;
const rootAppTestPattern = /^App(?:\.[^.]+)*\.test\.tsx$/;
const appImportPattern = /from\s+["'][^"']*(?:^|\/)App["']/m;
const unitDomImportPattern =
  /from\s+["'](?:@testing-library\/react|react-dom(?:\/[^"']*)?|jsdom|playwright|@playwright\/test)["']/m;
const bunUnitDomImportPattern =
  /from\s+["'](?:@testing-library\/react|react-dom(?:\/[^"']*)?|jsdom)["']/m;
const bunUnitBrowserImportPattern =
  /from\s+["'](?:playwright|@playwright\/test|@vitest\/browser|[^"']*browser-test-harness[^"']*)["']/m;
const domEnvironmentDirectivePattern =
  /@vitest-environment\s+(?:jsdom|happy-dom)/m;
const componentBrowserImportPattern =
  /from\s+["'](?:playwright|@playwright\/test|[^"']*browser-test-harness[^"']*)["']/m;
const optionalDomCapabilityImportPattern = /vitest-dom-capabilities\.setup/m;
const aggregateFeaturePublicImportPattern =
  /(?:from\s+|import\s*\(|(?:vi|mock)\.mock\()\s*["'][^"']*(?:\/public|\.\.?\/public)["']/m;
const generalUiBarrelImportPattern = /from\s+["'][^"']*components\/ui["']/m;
const rootComponentsPackageImportPattern =
  /from\s+["']@you-agent-factory\/components["']/m;
const optionalGlobalSetupPattern =
  /(?:ResizeObserver|HTMLAnchorElement|queryCommandSupported|monaco|test-browser-shims|vitest-dom-capabilities)/i;
const dashboardCompositionImportPattern =
  /from\s+["'][^"']*features\/dashboard\/(?:components\/dashboard-screen|public(?:\/screen)?)["']/m;
const bunTestImportPattern = /^\s*import\b[^\n;]*\bfrom\s+["']bun:test["']/m;
const vitestImportPattern = /^\s*import\b[^\n;]*\bfrom\s+["']vitest["']/m;

export function classifiedUiTestLane(relativePath) {
  const normalized = relativePath.split(sep).join("/");
  if (BUN_UNIT_TEST_MARKER_PATTERN.test(normalized)) {
    return BUN_UNIT_TEST_PATH_PATTERN.test(normalized) ? "bun-unit" : null;
  }
  if (
    /(?:^|\/)performance\//.test(normalized) ||
    /\.performance\.test\.(?:[cm]?[jt]sx?)$/.test(normalized)
  ) {
    return "performance";
  }
  if (
    /(?:^|\/)integration\//.test(normalized) ||
    /\.browser\.test\.(?:[cm]?[jt]sx?)$/.test(normalized)
  ) {
    return "browser";
  }
  if (
    /\.component\.test\.(?:[cm]?[jt]sx?)$/.test(normalized) ||
    /\.test\.(?:[cm]?[jt]sx)$/.test(normalized)
  ) {
    return "component";
  }
  if (/\.(?:test|spec)\.(?:[cm]?[jt]s)$/.test(normalized)) {
    return "unit";
  }
  return null;
}

export function auditUiTestFile({ relativePath, source }) {
  const errors = [];
  const normalized = relativePath.split(sep).join("/");
  const basename = normalized.split("/").at(-1) ?? normalized;

  if (rootAppTestPattern.test(basename) && !normalized.includes("/")) {
    errors.push(
      `${normalized}: root App tests are retired; move the contract to its unit or component owner`,
    );
  }
  if (appImportPattern.test(source)) {
    errors.push(
      `${normalized}: tests must not import App.tsx; test route resolution or DashboardScreen ownership instead`,
    );
  }

  const lane = classifiedUiTestLane(normalized);
  if (
    BUN_UNIT_TEST_MARKER_PATTERN.test(normalized) &&
    !BUN_UNIT_TEST_PATH_PATTERN.test(normalized)
  ) {
    errors.push(
      `${normalized}: Bun-native unit tests must use the exact .bun.unit.test.ts suffix; this path is ambiguous or unowned`,
    );
  }
  if (lane === "bun-unit" && bunUnitDomImportPattern.test(source)) {
    errors.push(
      `${normalized}: Bun unit tests cannot import DOM dependencies; move rendered behavior to a component test`,
    );
  }
  if (lane === "bun-unit" && bunUnitBrowserImportPattern.test(source)) {
    errors.push(
      `${normalized}: Bun unit tests cannot import browser runners or integration harnesses; move browser behavior to an integration test`,
    );
  }
  if (lane === "bun-unit" && vitestImportPattern.test(source)) {
    errors.push(
      `${normalized}: Bun unit tests must use Bun's native test API; importing Vitest leaves the file on an ambiguous runner boundary`,
    );
  }
  if (lane === "unit" && bunTestImportPattern.test(source)) {
    errors.push(
      `${normalized}: files importing bun:test must use the exact .bun.unit.test.ts suffix so the Bun unit lane owns them`,
    );
  }
  if (lane === "unit" && unitDomImportPattern.test(source)) {
    errors.push(
      `${normalized}: unit tests cannot import DOM or browser runners`,
    );
  }
  if (lane === "unit" && domEnvironmentDirectivePattern.test(source)) {
    errors.push(
      `${normalized}: unit tests cannot request a DOM environment; rename the file as a component test`,
    );
  }
  if (lane === "unit" && optionalDomCapabilityImportPattern.test(source)) {
    errors.push(
      `${normalized}: unit tests cannot install optional DOM capabilities`,
    );
  }
  if (lane === "bun-unit" && domEnvironmentDirectivePattern.test(source)) {
    errors.push(
      `${normalized}: Bun unit tests cannot request a DOM environment; move the file to a component test`,
    );
  }
  if (lane === "bun-unit" && optionalDomCapabilityImportPattern.test(source)) {
    errors.push(
      `${normalized}: Bun unit tests cannot install optional DOM capabilities; move the file to a component test`,
    );
  }
  if (lane === "component" && componentBrowserImportPattern.test(source)) {
    errors.push(`${normalized}: component tests cannot import browser runners`);
  }

  return errors;
}

export function auditUiSourceFile({ relativePath, source }) {
  const errors = [];
  const normalized = relativePath.split(sep).join("/");

  if (aggregateFeaturePublicImportPattern.test(source)) {
    errors.push(
      `${normalized}: import the owning module instead of an aggregate feature public barrel`,
    );
  }
  if (generalUiBarrelImportPattern.test(source)) {
    errors.push(
      `${normalized}: import a focused UI module instead of the general components/ui barrel`,
    );
  }
  if (
    rootComponentsPackageImportPattern.test(source) &&
    !/\.(?:test|integration)\.[cm]?[jt]sx?$/.test(normalized) &&
    !/\.stories\.[cm]?[jt]sx?$/.test(normalized)
  ) {
    errors.push(
      `${normalized}: import a focused @you-agent-factory/components subpath instead of its package root`,
    );
  }
  if (
    normalized === "testing/vitest.setup.ts" &&
    optionalGlobalSetupPattern.test(source)
  ) {
    errors.push(
      `${normalized}: optional browser and editor capabilities must be installed by the tests that need them`,
    );
  }
  if (
    normalized.startsWith("testing/") &&
    dashboardCompositionImportPattern.test(source)
  ) {
    errors.push(
      `${normalized}: generic test helpers cannot import DashboardScreen; keep dashboard renderers feature-owned`,
    );
  }

  return errors;
}

function collectTestFiles(directory) {
  const files = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...collectTestFiles(path));
    else if (entry.isFile() && testFilePattern.test(entry.name))
      files.push(path);
  }
  return files;
}

function collectSourceFiles(directory) {
  const files = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...collectSourceFiles(path));
    else if (entry.isFile() && /\.(?:[cm]?[jt]sx?)$/.test(entry.name))
      files.push(path);
  }
  return files;
}

export function auditUiTestLanes(sourceRoot) {
  const testErrors = collectTestFiles(sourceRoot).flatMap((path) =>
    auditUiTestFile({
      relativePath: relative(sourceRoot, path),
      source: readFileSync(path, "utf8"),
    }),
  );
  const sourceErrors = collectSourceFiles(sourceRoot).flatMap((path) =>
    auditUiSourceFile({
      relativePath: relative(sourceRoot, path),
      source: readFileSync(path, "utf8"),
    }),
  );
  return [...testErrors, ...sourceErrors];
}

const isMain = process.argv[1]
  ? resolve(process.argv[1]) === fileURLToPath(import.meta.url)
  : false;

if (isMain) {
  const sourceRoot = resolve(process.cwd(), "src");
  const errors = auditUiTestLanes(sourceRoot);
  if (errors.length > 0) {
    console.error(["UI test lane audit failed:", ...errors].join("\n"));
    process.exitCode = 1;
  } else {
    console.log("UI test lane audit passed.");
  }
}
