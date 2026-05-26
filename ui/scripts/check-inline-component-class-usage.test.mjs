import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanInlineComponentClassUsage } from "./check-inline-component-class-usage.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-inline-component-class-usage.mjs",
);
const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function createSourceTree(files) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "inline-component-class-guard-"),
  );
  const srcDir = path.join(tempRoot, "src");
  await mkdir(srcDir, { recursive: true });

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(srcDir, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return { srcDir, tempRoot };
}

test("scanInlineComponentClassUsage flags a single-use static class constant used only as a direct JSX className", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/example/panel.tsx": `
      import { cn } from "../../lib/cn";
      import { DASHBOARD_PANEL_SHELL_CLASS } from "../../components/ui/dashboard-shell";

      const PANEL_CLASS = cn(DASHBOARD_PANEL_SHELL_CLASS, "p-4");

      export function Panel() {
        return <section className={PANEL_CLASS}>Body</section>;
      }
    `,
  });

  try {
    await expect(scanInlineComponentClassUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          constantName: "PANEL_CLASS",
          relativeFilePath: "src/features/example/panel.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanInlineComponentClassUsage preserves exported tokens, merge helpers, and .ts style-contract modules", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "components/ui/tokens.tsx": `
      export const SHARED_PANEL_CLASS = "rounded-lg border border-af-border";

      export function Panel({ className = "" }: { className?: string }) {
        const cardClassName = cn(SHARED_PANEL_CLASS, className);
        return <section className={cardClassName}>Body</section>;
      }
    `,
    "features/example/style-contract.ts": `
      const PANEL_CLASS = "rounded-lg border border-af-border";

      export const contract = PANEL_CLASS;
    `,
  });

  try {
    await expect(scanInlineComponentClassUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanInlineComponentClassUsage still flags a top-level class constant when a nested scope shadows the same identifier", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/example/panel.tsx": `
      const PANEL_CLASS = "rounded-lg border border-af-border";

      export function Panel() {
        return <section className={PANEL_CLASS}>Body</section>;
      }

      function helper(PANEL_CLASS: string) {
        return PANEL_CLASS;
      }

      void helper;
    `,
  });

  try {
    await expect(scanInlineComponentClassUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          constantName: "PANEL_CLASS",
          relativeFilePath: "src/features/example/panel.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanInlineComponentClassUsage supports a narrow explicit allowlist and reports stale entries", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/example/panel.tsx": `
      const PANEL_CLASS = "rounded-lg border border-af-border";

      export function Panel({ children }: { children?: React.ReactNode }) {
        return <section className={PANEL_CLASS}>{children}</section>;
      }
    `,
  });

  try {
    await expect(
      scanInlineComponentClassUsage(srcDir, [
        "src/features/example/panel.tsx#PANEL_CLASS",
        "src/features/example/panel.tsx#MISSING_CLASS",
      ]),
    ).resolves.toEqual({
      staleAllowlistEntries: ["src/features/example/panel.tsx#MISSING_CLASS"],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI output reports the offending file and constant name", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/example/panel.tsx": `
      const PANEL_CLASS = "rounded-lg border border-af-border";

      export function Panel() {
        return <section className={PANEL_CLASS}>Body</section>;
      }
    `,
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "Inline component class usage guard failed.",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/features/example/panel.tsx"),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("PANEL_CLASS"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("package command wiring matches the direct guard result for the same violation", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/example/panel.tsx": `
      const PANEL_CLASS = "rounded-lg border border-af-border";

      export function Panel({ children }: { children?: React.ReactNode }) {
        return <section className={PANEL_CLASS}>{children}</section>;
      }
    `,
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("src/features/example/panel.tsx"),
    });
    await expect(
      execFileAsync("bun", ["run", "check:inline-component-class-usage"], {
        cwd: uiRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("src/features/example/panel.tsx"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
