import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

/**
 * The package root eagerly re-exports semantic-workstation-presentation, which
 * imports @you-agent-factory/client. A consumer that only wants the visual-state
 * policy -- workstation-icon-metadata.ts, for instance -- must not be forced to
 * install that dependency to resolve the package, or clean Bun/component jobs
 * fail with `Cannot find module '@you-agent-factory/client'` before a single
 * test runs.
 *
 * These assertions pin the ./visual-state subpath as a runtime-safe consumption
 * path: it must exist, and nothing reachable from it may import another
 * workspace package.
 */

const packageRoot = path.join(import.meta.dirname, "..");
const distRoot = path.join(packageRoot, "dist");

function readDist(file: string): string {
  return readFileSync(path.join(distRoot, file), "utf8");
}

function importSpecifiers(source: string): string[] {
  const specifiers: string[] = [];
  const pattern = /(?:from|import)\s*["']([^"']+)["']/g;
  let match = pattern.exec(source);
  while (match !== null) {
    specifiers.push(match[1]);
    match = pattern.exec(source);
  }
  return specifiers;
}

/** Every dist module reachable from the given entry, entry included. */
function reachableDistModules(entry: string): string[] {
  const seen = new Set<string>();
  const external: string[] = [];
  const queue = [entry];

  while (queue.length > 0) {
    const current = queue.pop();
    if (current === undefined || seen.has(current)) {
      continue;
    }
    seen.add(current);

    for (const specifier of importSpecifiers(readDist(current))) {
      if (specifier.startsWith(".")) {
        queue.push(path.normalize(path.join(path.dirname(current), specifier)));
        continue;
      }
      external.push(specifier);
    }
  }

  return external;
}

describe("factory-graph ./visual-state subpath", () => {
  it("is declared as a public export backed by a dist entry", () => {
    const manifest = JSON.parse(
      readFileSync(path.join(packageRoot, "package.json"), "utf8"),
    ) as { exports?: Record<string, Record<string, string>> };

    expect(manifest.exports?.["./visual-state"]).toEqual({
      types: "./dist/visual-state.d.ts",
      import: "./dist/visual-state.js",
      default: "./dist/visual-state.js",
    });
  });

  it("resolves without depending on any other workspace package", () => {
    const external = reachableDistModules("visual-state.js");

    expect(
      external.filter((specifier) =>
        specifier.startsWith("@you-agent-factory/"),
      ),
    ).toEqual([]);
  });

  it("still reaches @you-agent-factory/client from the package root, which is why the subpath exists", () => {
    const external = reachableDistModules("index.js");

    expect(external).toContain("@you-agent-factory/client");
  });
});
