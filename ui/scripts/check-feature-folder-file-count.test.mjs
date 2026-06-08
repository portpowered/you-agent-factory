import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanFeatureFolderFileCount } from "./check-feature-folder-file-count.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-feature-folder-file-count.mjs",
);

async function createFeatureTree(features) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "feature-folder-file-count-guard-"),
  );
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

function createFiles(prefix, count) {
  const files = {};
  for (let index = 1; index <= count; index += 1) {
    files[`${prefix}${index}.ts`] = `export const value${index} = ${index};\n`;
  }
  return files;
}

test("scanFeatureFolderFileCount passes when folders stay within the file limit", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 10),
      "public/index.ts": "export * from '../components/panel-1';\n",
    },
  });

  try {
    await expect(scanFeatureFolderFileCount(featuresDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      folderFileLimit: 10,
      growthViolations: [],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFolderFileCount distinguishes allowlisted debt from new hard-fail violations", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 12),
    },
    beta: {
      ...createFiles("hooks/use-", 11),
    },
  });

  try {
    await expect(
      scanFeatureFolderFileCount(featuresDir, [
        {
          maxFileCount: 12,
          relativeDirectoryPath: "src/features/alpha/components",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [
        expect.objectContaining({
          fileCount: 12,
          relativeDirectoryPath: "src/features/alpha/components",
        }),
      ],
      folderFileLimit: 10,
      growthViolations: [],
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          fileCount: 11,
          relativeDirectoryPath: "src/features/beta/hooks",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFolderFileCount reports growth when an allowlisted folder adds files", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 13),
    },
  });

  try {
    await expect(
      scanFeatureFolderFileCount(featuresDir, [
        {
          maxFileCount: 12,
          relativeDirectoryPath: "src/features/alpha/components",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [],
      folderFileLimit: 10,
      growthViolations: [
        expect.objectContaining({
          allowlistedMaxFileCount: 12,
          fileCount: 13,
          relativeDirectoryPath: "src/features/alpha/components",
        }),
      ],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFolderFileCount reports stale allowlist entries when the folder shrinks below the limit", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 8),
    },
  });

  try {
    await expect(
      scanFeatureFolderFileCount(featuresDir, [
        {
          maxFileCount: 12,
          relativeDirectoryPath: "src/features/alpha/components",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [],
      folderFileLimit: 10,
      growthViolations: [],
      staleAllowlistEntries: ["src/features/alpha/components"],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI reports allowlisted legacy debt during a passing run", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 12),
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining(
        "Feature folder file-count guard passed with allowlisted legacy debt.",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining("12 files"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI fails with actionable hard-violation output and still shows allowlisted debt", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      ...createFiles("components/panel-", 12),
    },
    beta: {
      ...createFiles("hooks/use-", 11),
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "Feature folder file-count guard failed.",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/features/beta/hooks"),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
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
      ...createFiles("components/panel-", 8),
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
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
          AGENT_FACTORY_UI_FEATURE_FOLDER_FILE_COUNT_ALLOWLIST: JSON.stringify([
            {
              maxFileCount: 12,
              relativeDirectoryPath: "src/features/alpha/components",
            },
          ]),
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/features/alpha/components"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
