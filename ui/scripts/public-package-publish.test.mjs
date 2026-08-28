import { spawn } from "node:child_process";
import { access, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";
import {
  FRONTEND_ONLY_CANDIDATE_SCOPE,
  TAGGED_RELEASE_CANDIDATE_SCOPE,
} from "../../scripts/public-package-set.mjs";
import {
  assertFrontendCandidateEvidence,
  assertPublishVersion,
  PUBLIC_PACKAGES,
  packCandidate,
  patchPublicPackageManifest,
  verifyRegistryVersion,
} from "./public-package-publish.mjs";

function createVirtualClock() {
  let elapsedMs = 0;
  const scheduledDelays = [];

  return {
    now: () => elapsedMs,
    sleep: async (delayMs) => {
      scheduledDelays.push(delayMs);
      elapsedMs += delayMs;
    },
    scheduledDelays,
    get elapsedMs() {
      return elapsedMs;
    },
  };
}

function runNodeScript(source) {
  return new Promise((resolve, reject) => {
    const child = spawn(
      process.execPath,
      ["--input-type=module", "--eval", source],
      {
        cwd: path.resolve(path.dirname(fileURLToPath(import.meta.url)), ".."),
        env: process.env,
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.once("error", reject);
    child.once("close", (code, signal) => {
      if (code === 0) return resolve({ stdout, stderr });
      reject(
        new Error(
          `node subprocess failed with ${code === null ? `signal ${signal}` : `exit code ${code}`}\n${stderr}`,
        ),
      );
    });
  });
}

describe("public package structure", () => {
  test("publishes the canonical package family in dependency order", () => {
    expect(PUBLIC_PACKAGES.map(({ name }) => name)).toEqual([
      "@you-agent-factory/client",
      "@you-agent-factory/factory-replay",
      "@you-agent-factory/factory-emulator",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-graph",
      "@you-agent-factory/factory-visualizers",
    ]);
  });

  test("accepts stable and immutable development versions", () => {
    expect(assertPublishVersion("1.2.3")).toBe("1.2.3");
    expect(assertPublishVersion("0.0.0-dev.123.abcdef123456")).toBe(
      "0.0.0-dev.123.abcdef123456",
    );
    expect(() => assertPublishVersion("v1.2.3")).toThrow(
      "Invalid public package version",
    );
  });

  test("aligns every internal package dependency with the candidate version", () => {
    const manifest = {
      name: "@you-agent-factory/factory-visualizers",
      version: "0.0.0",
      dependencies: {
        "@you-agent-factory/components": "0.0.0",
        "@xyflow/react": "^12.0.0",
      },
      peerDependencies: {
        "@you-agent-factory/client": "0.0.0",
        react: "^19.0.0",
      },
      devDependencies: {
        "@you-agent-factory/factory-replay": "file:../factory-replay",
      },
    };
    expect(patchPublicPackageManifest(manifest, "2.0.0")).toEqual({
      ...manifest,
      version: "2.0.0",
      dependencies: {
        "@you-agent-factory/components": "2.0.0",
        "@xyflow/react": "^12.0.0",
      },
      peerDependencies: {
        "@you-agent-factory/client": "2.0.0",
        react: "^19.0.0",
      },
      devDependencies: {
        "@you-agent-factory/factory-replay": "2.0.0",
      },
    });
    expect(manifest.version).toBe("0.0.0");
  });
});

describe("public package artifacts", () => {
  test("candidate packing suppresses lifecycle script side effects", async () => {
    const root = await mkdtemp(
      path.join(tmpdir(), "you-package-lifecycle-fixture-"),
    );
    const stagedDirectory = path.join(root, "package");
    const outputDirectory = path.join(root, "candidate");
    const sentinel = path.join(root, "lifecycle-ran");
    try {
      await Promise.all([mkdir(stagedDirectory), mkdir(outputDirectory)]);
      await Promise.all([
        writeFile(
          path.join(stagedDirectory, "package.json"),
          `${JSON.stringify({
            name: "@you-agent-factory/lifecycle-fixture",
            version: "1.0.0",
            scripts: { prepack: "node create-sentinel.mjs" },
          })}\n`,
        ),
        writeFile(
          path.join(stagedDirectory, "create-sentinel.mjs"),
          `import { writeFileSync } from "node:fs"; writeFileSync(${JSON.stringify(sentinel)}, "ran");\n`,
        ),
      ]);
      const { stdout } = await packCandidate({
        stagedDirectory,
        outputDirectory,
      });
      expect(JSON.parse(stdout)[0].name).toBe(
        "@you-agent-factory/lifecycle-fixture",
      );
      await expect(access(sentinel)).rejects.toMatchObject({ code: "ENOENT" });
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  test("accepts only a complete frontend-only development candidate", () => {
    const evidence = {
      scope: FRONTEND_ONLY_CANDIDATE_SCOPE,
      version: "0.0.0-dev.123.abcdef123456",
      packages: PUBLIC_PACKAGES.map(({ name }) => ({
        name,
        version: "0.0.0-dev.123.abcdef123456",
      })),
    };
    expect(assertFrontendCandidateEvidence(evidence)).toBe(evidence);
    expect(() =>
      assertFrontendCandidateEvidence({
        ...evidence,
        scope: TAGGED_RELEASE_CANDIDATE_SCOPE,
      }),
    ).toThrow("expected frontend-only candidate scope");
  });
});

describe("registry version visibility", () => {
  test("UT-VIS-000 returns immediately when the expected digest is visible", async () => {
    const clock = createVirtualClock();
    const logs = [];
    let lookups = 0;

    await verifyRegistryVersion(
      "@you-agent-factory/client",
      "1.2.3",
      "a".repeat(40),
      {
        lookup: async () => {
          lookups += 1;
          return "a".repeat(40);
        },
        ...clock,
        log: (message) => logs.push(message),
      },
    );

    expect(lookups).toBe(1);
    expect(clock.scheduledDelays).toEqual([]);
    expect(logs).toEqual([]);
  });

  test("UT-VIS-001 retries with capped exponential backoff until visibility", async () => {
    const clock = createVirtualClock();
    const logs = [];
    const expectedShasum = "b".repeat(40);
    const observations = [null, null, null, expectedShasum];

    await verifyRegistryVersion(
      "@you-agent-factory/client",
      "1.2.3",
      expectedShasum,
      {
        lookup: async () => observations.shift(),
        ...clock,
        log: (message) => logs.push(message),
      },
    );

    expect(clock.scheduledDelays).toEqual([5_000, 10_000, 20_000]);
    expect(clock.elapsedMs).toBe(35_000);
    expect(logs).toEqual([
      "Registry version not visible for @you-agent-factory/client@1.2.3; retry attempt 1, elapsed 0ms, next delay 5000ms",
      "Registry version not visible for @you-agent-factory/client@1.2.3; retry attempt 2, elapsed 5000ms, next delay 10000ms",
      "Registry version not visible for @you-agent-factory/client@1.2.3; retry attempt 3, elapsed 15000ms, next delay 20000ms",
    ]);
    expect(logs.join("\n")).not.toContain(expectedShasum);
  });

  test("UT-VIS-002 fails immediately on a conflicting digest", async () => {
    const clock = createVirtualClock();
    const logs = [];
    let lookups = 0;

    await expect(
      verifyRegistryVersion(
        "@you-agent-factory/client",
        "1.2.3",
        "c".repeat(40),
        {
          lookup: async () => {
            lookups += 1;
            return "d".repeat(40);
          },
          ...clock,
          log: (message) => logs.push(message),
        },
      ),
    ).rejects.toThrow(
      "Registry digest conflict for @you-agent-factory/client@1.2.3",
    );

    expect(lookups).toBe(1);
    expect(clock.scheduledDelays).toEqual([]);
    expect(logs).toEqual([]);
  });

  test("keeps CLI success JSON on stdout while default retry logs use stderr", async () => {
    const { stdout, stderr } = await runNodeScript(`
      import { verifyRegistryVersion } from "./scripts/public-package-publish.mjs";

      let elapsedMs = 0;
      let lookupCount = 0;
      const expectedShasum = "a".repeat(40);
      await verifyRegistryVersion(
        "@you-agent-factory/client",
        "1.2.3",
        expectedShasum,
        {
          lookup: async () => {
            lookupCount += 1;
            return lookupCount === 1 ? null : expectedShasum;
          },
          now: () => elapsedMs,
          sleep: async (delayMs) => {
            elapsedMs += delayMs;
          },
        },
      );
      process.stdout.write(JSON.stringify({ elapsedMs, lookupCount }) + "\\n");
    `);

    expect(stdout).toBe('{"elapsedMs":5000,"lookupCount":2}\n');
    expect(JSON.parse(stdout)).toEqual({ elapsedMs: 5_000, lookupCount: 2 });
    expect(stderr).toContain(
      "Registry version not visible for @you-agent-factory/client@1.2.3; retry attempt 1, elapsed 0ms, next delay 5000ms",
    );
    expect(stderr).not.toContain("a".repeat(40));
  });
});

describe("registry visibility deadline", () => {
  test("UT-VIS-003 and UT-VIS-005 wait exactly through the default visibility window", async () => {
    const clock = createVirtualClock();
    const logs = [];

    await expect(
      verifyRegistryVersion(
        "@you-agent-factory/client",
        "1.2.3",
        "e".repeat(40),
        {
          lookup: async () => null,
          ...clock,
          log: (message) => logs.push(message),
        },
      ),
    ).rejects.toThrow(
      "Published version did not become visible: @you-agent-factory/client@1.2.3",
    );

    expect(clock.scheduledDelays).toEqual([
      5_000, 10_000, 20_000, 30_000, 30_000, 30_000, 30_000, 30_000, 30_000,
      30_000, 30_000, 25_000,
    ]);
    expect(clock.scheduledDelays.every((delayMs) => delayMs <= 30_000)).toBe(
      true,
    );
    expect(clock.elapsedMs).toBe(300_000);
    expect(logs).toHaveLength(clock.scheduledDelays.length);
  });

  test("UT-VIS-004 propagates non-absence lookup failures", async () => {
    const clock = createVirtualClock();
    const logs = [];
    const dependencyError = new Error("registry lookup unavailable");
    let lookups = 0;

    await expect(
      verifyRegistryVersion(
        "@you-agent-factory/client",
        "1.2.3",
        "f".repeat(40),
        {
          lookup: async () => {
            lookups += 1;
            throw dependencyError;
          },
          ...clock,
          log: (message) => logs.push(message),
        },
      ),
    ).rejects.toBe(dependencyError);

    expect(lookups).toBe(1);
    expect(clock.scheduledDelays).toEqual([]);
    expect(logs).toEqual([]);
  });
});
