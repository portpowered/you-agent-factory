import { readFileSync } from "node:fs";

export const VITEST_COMPONENT_MARKER = "@component-test-runner vitest";

const COMPONENT_TEST_PATTERNS = [
  "src/**/*.test.tsx",
  "src/**/*.component.test.ts",
];
const EXPLICIT_BUN_COMPONENT_SUFFIX = ".bun.component.test.tsx";
const PERFORMANCE_TEST_PATTERN = /(?:^|\/)performance\//;

const VITEST_COMPATIBILITY_PATTERNS: Array<{
  pattern: RegExp;
  reason: string;
}> = [
  {
    pattern: /@vitest-environment/,
    reason: "declares a Vitest environment",
  },
];
const BUN_COMPATIBLE_VI_APIS = new Set(["fn", "mocked"]);

function findUnsupportedViApis(source: string): string[] {
  return [
    ...new Set(
      [...source.matchAll(/\bvi\.([A-Za-z_]+)/g)]
        .map((match) => match[1])
        .filter((api) => !BUN_COMPATIBLE_VI_APIS.has(api)),
    ),
  ].sort();
}

export interface ClassifiedComponentTestFile {
  path: string;
  reason: string;
  runner: "bun" | "vitest";
}

export function classifyComponentTestSource(
  path: string,
  source: string,
): ClassifiedComponentTestFile {
  if (path.endsWith(EXPLICIT_BUN_COMPONENT_SUFFIX)) {
    return {
      path,
      reason: "explicit native Bun component test",
      runner: "bun",
    };
  }

  if (source.includes(VITEST_COMPONENT_MARKER)) {
    return {
      path,
      reason: "explicitly documented Vitest compatibility exception",
      runner: "vitest",
    };
  }

  for (const compatibility of VITEST_COMPATIBILITY_PATTERNS) {
    if (compatibility.pattern.test(source)) {
      return {
        path,
        reason: compatibility.reason,
        runner: "vitest",
      };
    }
  }

  const unsupportedViApis = findUnsupportedViApis(source);
  if (unsupportedViApis.length > 0) {
    return {
      path,
      reason: `uses unsupported Vitest APIs: ${unsupportedViApis.join(", ")}`,
      runner: "vitest",
    };
  }

  return {
    path,
    reason: "browserless component test without Vitest-only APIs",
    runner: "bun",
  };
}

export function listComponentTestFiles(
  cwd = process.cwd(),
): ClassifiedComponentTestFile[] {
  const paths = new Set<string>();
  for (const pattern of COMPONENT_TEST_PATTERNS) {
    const glob = new Bun.Glob(pattern);
    for (const path of glob.scanSync({ cwd })) {
      if (!PERFORMANCE_TEST_PATTERN.test(path)) {
        paths.add(path);
      }
    }
  }

  return [...paths]
    .sort()
    .map((path) =>
      classifyComponentTestSource(path, readFileSync(`${cwd}/${path}`, "utf8")),
    );
}
