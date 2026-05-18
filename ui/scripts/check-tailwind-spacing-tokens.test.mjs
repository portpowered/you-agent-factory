import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanTailwindSpacingTokens } from "./check-tailwind-spacing-tokens.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-tailwind-spacing-tokens.mjs",
);

async function createSourceTree(files) {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), "tailwind-spacing-guard-"));
  const srcDir = path.join(tempRoot, "src");
  await mkdir(srcDir, { recursive: true });

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(srcDir, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return { srcDir, tempRoot };
}

test("scanTailwindSpacingTokens allows standard tokens and intrinsic sizing exceptions", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "feature.tsx": `
      export function Feature() {
        return (
          <div className="grid gap-4 rounded-2xl p-4 md:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
            <div className="w-[min(92vw,60rem)] max-h-[24rem] min-h-[34rem]" />
          </div>
        );
      }
    `,
  });

  try {
    await expect(scanTailwindSpacingTokens(srcDir)).resolves.toEqual([]);
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanTailwindSpacingTokens rejects arbitrary spacing utilities and custom breakpoint variants", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "feature.tsx": `
      export function Feature() {
        return (
          <div className="gap-[0.8rem] rounded-[1.25rem] p-4 max-[900px]:p-3 min-[901px]:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]" />
        );
      }
    `,
  });

  try {
    await expect(scanTailwindSpacingTokens(srcDir)).resolves.toEqual([
      expect.objectContaining({ kind: "arbitrary-spacing", token: "gap-[0.8rem]" }),
      expect.objectContaining({ kind: "arbitrary-spacing", token: "rounded-[1.25rem]" }),
      expect.objectContaining({ kind: "custom-breakpoint", token: "max-[900px]:p-3" }),
      expect.objectContaining({
        kind: "custom-breakpoint",
        token: "min-[901px]:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]",
      }),
    ]);
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanTailwindSpacingTokens skips story, test, and generated files", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "feature.tsx": `
      export function Feature() {
        return <div className="gap-4 p-4 md:grid-cols-2" />;
      }
    `,
    "feature.test.tsx": `
      export function FeatureTest() {
        return <div className="p-[18px] max-[720px]:grid" />;
      }
    `,
    "feature.stories.tsx": `
      export function FeatureStory() {
        return <div className="gap-[0.8rem]" />;
      }
    `,
    "generated/openapi.tsx": `
      export function GeneratedArtifact() {
        return <div className="rounded-[1.25rem]" />;
      }
    `,
  });

  try {
    await expect(scanTailwindSpacingTokens(srcDir)).resolves.toEqual([]);
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI output reports actionable violations", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "feature.tsx": `
      export function Feature() {
        return <div className="p-[18px] max-[720px]:grid" />;
      }
    `,
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: { ...process.env, AGENT_FACTORY_UI_SRC_DIR: srcDir },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Tailwind spacing token guard failed."),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
