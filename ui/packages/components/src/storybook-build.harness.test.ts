import { execSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

let tempRoots: string[] = [];

afterEach(() => {
  for (const tempRoot of tempRoots) {
    rmSync(tempRoot, { force: true, recursive: true });
  }
  tempRoots = [];
});

function createIsolatedStorybookFixture() {
  const tempRoot = mkdtempSync(
    path.join(os.tmpdir(), "youagentfactory-components-storybook-failure-"),
  );
  tempRoots.push(tempRoot);

  const storybookDir = path.join(tempRoot, ".storybook");
  const storiesDir = path.join(tempRoot, "stories");
  mkdirSync(storybookDir, { recursive: true });
  mkdirSync(storiesDir, { recursive: true });

  writeFileSync(
    path.join(storybookDir, "main.ts"),
    `import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../stories/**/*.stories.@(ts|tsx)"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
};

export default config;
`,
  );

  writeFileSync(
    path.join(storiesDir, "broken.stories.tsx"),
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

  return { storybookDir };
}

describe("package Storybook build harness", () => {
  it("exits non-zero when a story imports a missing module", () => {
    const { storybookDir } = createIsolatedStorybookFixture();
    const outputDir = mkdtempSync(
      path.join(os.tmpdir(), "youagentfactory-components-storybook-output-"),
    );
    tempRoots.push(outputDir);

    expect(() => {
      execSync(
        `bun run build-storybook -- --config-dir "${storybookDir}" --output-dir "${outputDir}" --loglevel error`,
        {
          cwd: packageRoot,
          encoding: "utf8",
          stdio: ["ignore", "pipe", "pipe"],
        },
      );
    }).toThrow();
  }, 120_000);
});
