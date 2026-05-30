import { execFile } from "node:child_process";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { expect, test } from "bun:test";

import { scanButtonUsage } from "./check-button-usage.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-button-usage.mjs",
);

async function createSourceTree(files) {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "button-usage-guard-"));
  const srcDir = path.join(tempRoot, "src");
  await mkdir(srcDir, { recursive: true });

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(srcDir, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return { srcDir, tempRoot };
}

test("scanButtonUsage allows approved primitive owners and narrow semantic exceptions", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "components/ui/button.tsx": `
      export function ButtonOwner() {
        return buttonVariants({ tone: "secondary" });
      }
    `,
    "features/selection/components/detail.tsx": `
      export function Detail() {
        const expanded = false;
        return <button aria-expanded={expanded} type="button">Toggle details</button>;
      }
    `,
  });

  try {
    await expect(
      scanButtonUsage(srcDir, [
        {
          buttonVariantsCount: 1,
          buttonVariantsReason: "shared owner",
          relativeFilePath: "src/components/ui/button.tsx",
        },
        {
          rawButtonFingerprints: ['aria-expanded={expanded}'],
          rawButtonReason: "semantic disclosure shell",
          relativeFilePath: "src/features/selection/components/detail.tsx",
        },
      ]),
    ).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanButtonUsage rejects unallowlisted raw buttons and direct buttonVariants ownership", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/forms/components/export-dialog.tsx": `
      export function ExportDialog() {
        const buttonClassName = buttonVariants({ tone: "secondary" });
        return <button className={buttonClassName} type="button">Export</button>;
      }
    `,
  });

  try {
    await expect(scanButtonUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          kind: "raw-button",
          relativeFilePath: "src/features/forms/components/export-dialog.tsx",
        }),
        expect.objectContaining({
          kind: "button-variants",
          relativeFilePath: "src/features/forms/components/export-dialog.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanButtonUsage rejects raw-button drift in an allowlisted file when the count still matches", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/selection/components/detail.tsx": `
      export function Detail() {
        return <button type="button">Save changes</button>;
      }
    `,
  });

  try {
    await expect(
      scanButtonUsage(srcDir, [
        {
          rawButtonFingerprints: ['aria-expanded={expanded}'],
          rawButtonReason: "semantic disclosure shell",
          relativeFilePath: "src/features/selection/components/detail.tsx",
        },
      ]),
    ).resolves.toEqual({
      staleAllowlistEntries: [
        expect.objectContaining({
          reason: expect.stringContaining("Allowlisted raw <button> fingerprints are stale"),
          relativeFilePath: "src/features/selection/components/detail.tsx",
        }),
      ],
      violations: [
        expect.objectContaining({
          kind: "raw-button",
          relativeFilePath: "src/features/selection/components/detail.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI output reports actionable button-lane guidance", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "feature.tsx": `
      export function Feature() {
        return <button type="button">Save</button>;
      }
    `,
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_BUTTON_USAGE_ALLOWLIST: "[]",
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Button usage guard failed."),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_BUTTON_USAGE_ALLOWLIST: "[]",
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("Button"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
