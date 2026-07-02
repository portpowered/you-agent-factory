import { execSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const brokenStoryBasename = "__storybook-build-failure__.stories.tsx";
const brokenStoryPath = path.join(
  packageRoot,
  "src",
  "primitives",
  brokenStoryBasename,
);

afterEach(() => {
  rmSync(brokenStoryPath, { force: true });
});

describe("package Storybook build harness", () => {
  it(
    "exits non-zero when a story imports a missing module",
    () => {
      writeFileSync(
        brokenStoryPath,
        `import type { Meta, StoryObj } from "@storybook/react-vite";
import { MissingStorybookModule } from "./missing-storybook-module";

const meta = {
  title: "Harness/Broken",
  component: MissingStorybookModule,
} satisfies Meta<typeof MissingStorybookModule>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
`,
      );

      const outputDir = mkdtempSync(
        path.join(os.tmpdir(), "youagentfactory-components-storybook-failure-"),
      );

      try {
        expect(() => {
          execSync(
            `bun run build-storybook -- --output-dir "${outputDir}" --loglevel error`,
            {
              cwd: packageRoot,
              encoding: "utf8",
              stdio: ["ignore", "pipe", "pipe"],
            },
          );
        }).toThrow();
      } finally {
        rmSync(outputDir, { force: true, recursive: true });
      }
    },
    120_000,
  );
});
