import { execFile, execSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";

const execFileAsync = promisify(execFile);
const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../..",
);
// Make subprocess harness checks can exceed Vitest's default 5s under parallel load.
const makeHarnessTestTimeoutMs = 60_000;

describe("component package harness wiring", () => {
  let tempRoots: string[] = [];

  afterEach(async () => {
    await Promise.all(
      tempRoots.map((tempRoot) =>
        rm(tempRoot, { force: true, recursive: true }),
      ),
    );
    tempRoots = [];
  });

  it("runs ui-components-typecheck successfully on the real package", () => {
    execSync("make ui-components-typecheck", {
      cwd: repoRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  }, 60_000);

  it("runs ui-components-boundary successfully on the real package", () => {
    execSync("make ui-components-boundary", {
      cwd: repoRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
  }, 60_000);

  it(
    "fails ui-components-boundary when package source violates boundary rules",
    async () => {
      const tempRoot = await mkdtemp(
        path.join(os.tmpdir(), "package-harness-boundary-failure-"),
      );
      tempRoots.push(tempRoot);

      const packageSrcDir = path.join(tempRoot, "src");
      await mkdir(path.join(packageSrcDir, "widgets"), { recursive: true });
      await writeFile(
        path.join(packageSrcDir, "widgets/bad.tsx"),
        'import { toast } from "sonner";\nexport function BadWidget() { toast("nope"); return null; }\n',
      );

      await expect(
        execFileAsync("make", ["ui-components-boundary"], {
          cwd: repoRoot,
          env: {
            ...process.env,
            AGENT_FACTORY_COMPONENTS_SRC_DIR: packageSrcDir,
            AGENT_FACTORY_DASHBOARD_SRC_DIR: path.join(
              tempRoot,
              "dashboard-src",
            ),
          },
        }),
      ).rejects.toMatchObject({
        code: 2,
        stderr: expect.stringContaining(
          "@you-agent-factory/components package boundary check failed:",
        ),
      });
    },
    makeHarnessTestTimeoutMs,
  );
});
