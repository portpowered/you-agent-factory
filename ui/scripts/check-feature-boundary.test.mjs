import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanCrossFeatureBoundary } from "./check-feature-boundary.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-feature-boundary.mjs",
);

async function createFeatureTree(features) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "feature-boundary-guard-"),
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

test("scanCrossFeatureBoundary passes when features import through public boundaries", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "components/panel.tsx": "export function Panel() { return null; }\n",
      "public/index.ts": "export * from '../components/panel';\n",
    },
    beta: {
      "components/widget.tsx":
        'import { Panel } from "../../alpha/public";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(scanCrossFeatureBoundary(featuresDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanCrossFeatureBoundary skips feature-local test-support helpers", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
    },
    beta: {
      "components/test-support/widget.test-helpers.tsx":
        'import { useAlpha } from "../../../alpha/hooks/use-alpha";\nexport function renderWidget() { return null; }\n',
    },
  });

  try {
    await expect(scanCrossFeatureBoundary(featuresDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanCrossFeatureBoundary flags cross-feature internal imports as hard-fail violations", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
    },
    beta: {
      "components/widget.tsx":
        'import { useAlpha } from "../../alpha/hooks/use-alpha";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(scanCrossFeatureBoundary(featuresDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          relativeFilePath: "src/features/beta/components/widget.tsx",
          specifier: "../../alpha/hooks/use-alpha",
          targetFeatureName: "alpha",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanCrossFeatureBoundary distinguishes allowlisted debt from new hard-fail violations", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
      "hooks/use-beta.ts": "export function useBeta() { return null; }\n",
    },
    beta: {
      "components/widget.tsx":
        'import { useAlpha } from "../../alpha/hooks/use-alpha";\nimport { useBeta } from "../../alpha/hooks/use-beta";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(
      scanCrossFeatureBoundary(featuresDir, [
        {
          importSpecifiers: ["../../alpha/hooks/use-alpha"],
          reason: "legacy",
          relativeFilePath: "src/features/beta/components/widget.tsx",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [
        expect.objectContaining({
          relativeFilePath: "src/features/beta/components/widget.tsx",
        }),
      ],
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          relativeFilePath: "src/features/beta/components/widget.tsx",
          specifier: "../../alpha/hooks/use-beta",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanCrossFeatureBoundary reports stale allowlist entries when the import no longer exists", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    beta: {
      "components/widget.tsx": "export function Widget() { return null; }\n",
    },
  });

  try {
    await expect(
      scanCrossFeatureBoundary(featuresDir, [
        {
          importSpecifiers: ["../../alpha/hooks/use-alpha"],
          reason: "legacy",
          relativeFilePath: "src/features/beta/components/widget.tsx",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [
        expect.objectContaining({
          allowlistEntry: expect.objectContaining({
            relativeFilePath: "src/features/beta/components/widget.tsx",
            specifier: "../../alpha/hooks/use-alpha",
          }),
        }),
      ],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI reports allowlisted legacy debt during a passing run", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
    },
    beta: {
      "components/widget.tsx":
        'import { useAlpha } from "../../alpha/hooks/use-alpha";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST: JSON.stringify([
            {
              importSpecifiers: ["../../alpha/hooks/use-alpha"],
              reason: "legacy",
              relativeFilePath: "src/features/beta/components/widget.tsx",
            },
          ]),
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT: "1",
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining(
        "Feature boundary guard passed with allowlisted legacy debt.",
      ),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI fails with actionable hard-violation output and still shows allowlisted debt", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
      "hooks/use-beta.ts": "export function useBeta() { return null; }\n",
    },
    beta: {
      "components/widget.tsx":
        'import { useAlpha } from "../../alpha/hooks/use-alpha";\nimport { useBeta } from "../../alpha/hooks/use-beta";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST: JSON.stringify([
            {
              importSpecifiers: ["../../alpha/hooks/use-alpha"],
              reason: "legacy",
              relativeFilePath: "src/features/beta/components/widget.tsx",
            },
          ]),
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT: "1",
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Feature boundary guard failed."),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST: JSON.stringify([
            {
              importSpecifiers: ["../../alpha/hooks/use-alpha"],
              reason: "legacy",
              relativeFilePath: "src/features/beta/components/widget.tsx",
            },
          ]),
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT: "1",
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining(
        "src/features/beta/components/widget.tsx",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST: JSON.stringify([
            {
              importSpecifiers: ["../../alpha/hooks/use-alpha"],
              reason: "legacy",
              relativeFilePath: "src/features/beta/components/widget.tsx",
            },
          ]),
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_STRICT: "1",
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("Allowlisted legacy debt:"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI permits focused cross-feature imports in advisory mode", async () => {
  const { featuresDir, tempRoot } = await createFeatureTree({
    alpha: {
      "hooks/use-alpha.ts": "export function useAlpha() { return null; }\n",
    },
    beta: {
      "components/widget.tsx":
        'import { useAlpha } from "../../alpha/hooks/use-alpha";\nexport function Widget() { return null; }\n',
    },
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_CROSS_FEATURE_BOUNDARY_ALLOWLIST: "[]",
          AGENT_FACTORY_UI_FEATURES_DIR: featuresDir,
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining("Feature boundary advisory passed."),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
