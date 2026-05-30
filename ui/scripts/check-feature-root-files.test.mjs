import { execFile } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { expect, test } from "bun:test";

import { scanFeatureRootFiles } from "./check-feature-root-files.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-feature-root-files.mjs",
);

async function createFeatureTree(features) {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "feature-root-file-guard-"));
  const featuresDir = path.join(tempRoot, "src", "features");
  await mkdir(featuresDir, { recursive: true });

  for (const [featureName, files] of Object.entries(features)) {
    const featureDir = path.join(featuresDir, featureName);
    await mkdir(featureDir, { recursive: true });

    for (const [relativeFilePath, contents] of Object.entries(files)) {
      const filePath = path.join(featureDir, relativeFilePath);
      await mkdir(path.dirname(filePath), { recursive: true });
      await writeFile(filePath, contents);
    }
  }

  return { featuresDir, tempRoot };
}

test("scanFeatureRootFiles passes when feature roots contain directories only", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "components/panel.tsx": "export function Panel() { return null; }\n",
      "public/index.ts": "export * from '../components/panel';\n",
    },
    beta: {
      "hooks/use-beta.ts": "export function useBeta() { return null; }\n",
    },
  });

  try {
    await expect(scanFeatureRootFiles(featuresDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureRootFiles distinguishes allowlisted debt from new hard-fail violations for any file type", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "index.ts": "export const alpha = true;\n",
      "README.md": "# alpha\n",
      "components/panel.tsx": "export function Panel() { return null; }\n",
    },
    beta: {
      "widget.test.ts": "export const beta = true;\n",
      "stories/widget.stories.tsx": "export const Story = {};\n",
    },
  });

  try {
    await expect(
      scanFeatureRootFiles(featuresDir, ["src/features/alpha/index.ts", "src/features/beta/widget.test.ts"]),
    ).resolves.toEqual({
      allowlistedDebt: [
        expect.objectContaining({ relativeFilePath: "src/features/alpha/index.ts" }),
        expect.objectContaining({ relativeFilePath: "src/features/beta/widget.test.ts" }),
      ],
      staleAllowlistEntries: [],
      violations: [expect.objectContaining({ relativeFilePath: "src/features/alpha/README.md" })],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureRootFiles reports stale allowlist entries when the root file no longer exists", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "components/panel.tsx": "export function Panel() { return null; }\n",
      "public/index.ts": "export * from '../components/panel';\n",
    },
  });

  try {
    await expect(scanFeatureRootFiles(featuresDir, ["src/features/alpha/index.ts"])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: ["src/features/alpha/index.ts"],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI reports allowlisted legacy debt during a passing run", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    bento: {
      "index.ts": "export const alpha = true;\n",
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST: "src/features/bento/index.ts",
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining("Feature root file guard passed with allowlisted legacy debt."),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI fails with actionable hard-violation output and still shows allowlisted debt", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    bento: {
      "index.ts": "export const alpha = true;\n",
    },
    alpha: {
      "notes.md": "legacy notes\n",
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST: "src/features/bento/index.ts",
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Feature root file guard failed."),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST: "src/features/bento/index.ts",
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/features/alpha/notes.md"),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST: "src/features/bento/index.ts",
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("Allowlisted legacy debt:"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI fails when the allowlist contains a stale entry", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "components/panel.tsx": "export function Panel() { return null; }\n",
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST:
            "src/features/alpha/index.ts",
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Stale allowlist entries:"),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_ROOT_FILE_ALLOWLIST:
            "src/features/alpha/index.ts",
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/features/alpha/index.ts"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
