import { createHash } from "node:crypto";
import { access, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, expect, test } from "vitest";
import { timePackagePhase } from "../../scripts/package-phase-timing.mjs";
import { smokeFrontendPackages } from "../../scripts/public-package-install-smoke.mjs";
import {
  registryMetadata,
  verifyInstalledRegistryPackages,
} from "../../scripts/public-package-registry.mjs";
import { FRONTEND_ONLY_CANDIDATE_SCOPE } from "../../scripts/public-package-set.mjs";
import {
  PUBLIC_PACKAGES,
  publishPublicPackageCandidates,
} from "./public-package-publish.mjs";

async function candidateFixture(operation) {
  const root = await mkdtemp(path.join(tmpdir(), "you-publish-component-"));
  const version = "0.0.0-dev.test.fixture";
  const packages = PUBLIC_PACKAGES.map(({ name }, index) => {
    const bytes = Buffer.from(`controlled tarball ${name}`);
    return {
      name,
      version,
      filename: `candidate-${index}.tgz`,
      shasum: createHash("sha1").update(bytes).digest("hex"),
      integrity: `sha512-${createHash("sha512").update(bytes).digest("base64")}`,
    };
  });
  const evidence = { scope: FRONTEND_ONLY_CANDIDATE_SCOPE, version, packages };
  try {
    for (const candidate of packages) {
      await writeFile(
        path.join(root, candidate.filename),
        `controlled tarball ${candidate.name}`,
      );
    }
    await writeFile(
      path.join(root, "public-package-candidates.json"),
      JSON.stringify(evidence),
    );
    return await operation(root, evidence);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

describe("frontend publication safety", () => {
  test("rejects a corrupt final tarball before smoke, lookup or publication", () =>
    candidateFixture(async (root, evidence) => {
      const calls = [];
      await writeFile(
        path.join(root, evidence.packages.at(-1).filename),
        "corrupt",
      );
      await expect(
        publishPublicPackageCandidates({
          candidateDirectory: root,
          tag: "dev",
          smoke: async () => calls.push("smoke"),
          lookup: async () => calls.push("lookup"),
          publish: async () => calls.push("publish"),
        }),
      ).rejects.toThrow("Candidate digest mismatch");
      expect(calls).toEqual([]);
    }));

  test("a broken installed operation prevents every registry mutation", () =>
    candidateFixture(async (root) => {
      const calls = [];
      await expect(
        publishPublicPackageCandidates({
          candidateDirectory: root,
          tag: "dev",
          smoke: async () => {
            throw new Error("broken entry");
          },
          lookup: async () => calls.push("lookup"),
          publish: async () => calls.push("publish"),
        }),
      ).rejects.toThrow("broken entry");
      expect(calls).toEqual([]);
    }));

  test.each(["conflict", "auth"])(
    "whole-family %s preflight prevents partial publication",
    (failure) =>
      candidateFixture(async (root, evidence) => {
        const published = [];
        await expect(
          publishPublicPackageCandidates({
            candidateDirectory: root,
            tag: "dev",
            smoke: async () => {},
            lookup: async (name) => {
              if (name !== evidence.packages.at(-1).name) return null;
              if (failure === "auth") throw new Error("HTTP 401");
              return "a".repeat(40);
            },
            publish: async (args) => published.push(args),
          }),
        ).rejects.toThrow(
          failure === "auth" ? "HTTP 401" : "Registry digest conflict",
        );
        expect(published).toEqual([]);
      }),
  );
});

describe("frontend publication recovery", () => {
  test("partial rerun skips matching immutable versions and verifies all before registry install", () =>
    candidateFixture(async (root, evidence) => {
      const published = [],
        verified = [],
        order = [];
      const result = await publishPublicPackageCandidates({
        candidateDirectory: root,
        tag: "dev",
        provenance: true,
        smoke: async () => order.push("smoke"),
        lookup: async (name) =>
          name === evidence.packages[0].name
            ? evidence.packages[0].shasum
            : null,
        publish: async (args) => {
          published.push(args);
          order.push("publish");
        },
        verify: async (name) => {
          verified.push(name);
          order.push("verify");
        },
        verifyInstalled: async (observed) => {
          expect(observed).toEqual(evidence);
          order.push("installed");
        },
      });
      expect(result).toEqual(evidence);
      expect(published).toHaveLength(5);
      expect(
        published.every(
          (args) => args.includes("--provenance") && args.includes("dev"),
        ),
      ).toBe(true);
      expect(verified).toEqual(evidence.packages.map(({ name }) => name));
      expect(order).toEqual([
        "smoke",
        ...Array(5).fill("publish"),
        ...Array(6).fill("verify"),
        "installed",
      ]);
    }));

  test("failed publish leaves an explicit rerunnable partial family and does not claim installation", () =>
    candidateFixture(async (root) => {
      let published = 0,
        installed = false,
        verified = false;
      await expect(
        publishPublicPackageCandidates({
          candidateDirectory: root,
          tag: "dev",
          smoke: async () => {},
          lookup: async () => null,
          publish: async () => {
            if (++published === 2) throw new Error("publish denied");
          },
          verify: async () => {
            verified = true;
          },
          verifyInstalled: async () => {
            installed = true;
          },
        }),
      ).rejects.toThrow("publish denied");
      expect(published).toBe(2);
      expect(verified).toBe(false);
      expect(installed).toBe(false);
    }));

  test("visibility checks overlap, drain and propagate failures before installed proof", () =>
    candidateFixture(async (root) => {
      const started = [],
        releases = [];
      let installed = false;
      const pending = publishPublicPackageCandidates({
        candidateDirectory: root,
        tag: "dev",
        smoke: async () => {},
        lookup: async () => null,
        publish: async () => {},
        verify: (name) =>
          new Promise((resolve, reject) => {
            started.push(name);
            releases.push(
              name.includes("client")
                ? () => reject(new Error("visibility timeout"))
                : resolve,
            );
            if (started.length === 6)
              releases.forEach((release) => {
                release();
              });
          }),
        verifyInstalled: async () => {
          installed = true;
        },
      });
      await expect(pending).rejects.toThrow("visibility timeout");
      expect(started).toHaveLength(6);
      expect(installed).toBe(false);
    }));
});

describe("consumer isolation", () => {
  test.each([false, true])(
    "cleans its own consumer after success or install failure (%s)",
    (fail) =>
      candidateFixture(async (root, evidence) => {
        let consumer;
        const operation = smokeFrontendPackages({
          candidateDirectory: root,
          evidence,
          log: () => {},
          runCommand: async (_command, _args, options) => {
            consumer = options.cwd;
            expect(options.env.NODE_PATH).toBe("");
            const manifest = JSON.parse(
              await readFile(path.join(consumer, "package.json"), "utf8"),
            );
            expect(Object.keys(manifest.overrides)).toHaveLength(6);
            expect(
              Object.values(manifest.overrides).every((tarball) =>
                tarball.endsWith(".tgz"),
              ),
            ).toBe(true);
            if (fail) throw new Error("install failed");
            return { stdout: "operation passed", stderr: "" };
          },
        });
        if (fail) await expect(operation).rejects.toThrow("install failed");
        else await operation;
        await expect(access(consumer)).rejects.toMatchObject({
          code: "ENOENT",
        });
        await expect(access(root)).resolves.toBeUndefined();
      }),
  );

  test("registry downloaded bytes must match both candidate digests before install", () =>
    candidateFixture(async (_root, evidence) => {
      let installed = false;
      await expect(
        verifyInstalledRegistryPackages(evidence, {
          metadata: async (name) => ({
            dist: {
              shasum: evidence.packages.find(
                (candidate) => candidate.name === name,
              ).shasum,
              tarball: "https://registry.npmjs.org/controlled.tgz",
            },
          }),
          request: async () => new Response("corrupt tarball"),
          smoke: async (args) => {
            const { validateFrontendTarballs } = await import(
              "../../scripts/public-package-install-smoke.mjs"
            );
            await validateFrontendTarballs(
              args.evidence,
              args.candidateDirectory,
            );
            installed = true;
          },
        }),
      ).rejects.toThrow("Candidate digest mismatch");
      expect(installed).toBe(false);
    }));
});

describe("registry metadata and phase diagnostics", () => {
  test.each([401, 429, 500])(
    "HTTP %s is a failure, never package absence",
    async (status) => {
      await expect(
        registryMetadata("@you-agent-factory/client", "1.2.3", {
          request: async (_url, options) => {
            expect(options.signal).toBeInstanceOf(AbortSignal);
            return new Response("", { status });
          },
        }),
      ).rejects.toThrow(`HTTP ${status}`);
    },
  );
  test("only HTTP 404 returns absence; malformed identities fail", async () => {
    expect(
      await registryMetadata("@you-agent-factory/client", "1.2.3", {
        request: async () => new Response("", { status: 404 }),
      }),
    ).toBeNull();
    await expect(
      registryMetadata("@you-agent-factory/client", "1.2.3", {
        request: async () => Response.json({ name: "other", version: "1.2.3" }),
      }),
    ).rejects.toThrow("invalid metadata");
  });
  test("phase timing retains result, failure and elapsed diagnostics", async () => {
    let clock = 10;
    const logs = [],
      dependencies = { now: () => clock, log: (message) => logs.push(message) };
    expect(
      await timePackagePhase(
        "prepare",
        async () => {
          clock += 3;
          return "result";
        },
        dependencies,
      ),
    ).toBe("result");
    const error = new Error("failure");
    await expect(
      timePackagePhase(
        "publish",
        async () => {
          clock += 7;
          throw error;
        },
        dependencies,
      ),
    ).rejects.toBe(error);
    expect(logs).toEqual([
      "[package-phase] prepare start",
      "[package-phase] prepare passed elapsed_ms=3",
      "[package-phase] publish start",
      "[package-phase] publish failed elapsed_ms=7",
    ]);
  });
});
