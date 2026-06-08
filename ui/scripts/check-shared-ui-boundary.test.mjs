import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanSharedUiBoundary } from "./check-shared-ui-boundary.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-shared-ui-boundary.mjs",
);

async function createSharedUiTree(files) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "shared-ui-boundary-guard-"),
  );
  const sharedUiDir = path.join(tempRoot, "src", "components", "ui");
  await mkdir(sharedUiDir, { recursive: true });

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(sharedUiDir, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return { sharedUiDir, tempRoot };
}

async function createFeatureFixture(tempRoot, featurePath, contents) {
  const featureFilePath = path.join(tempRoot, "src", "features", featurePath);
  await mkdir(path.dirname(featureFilePath), { recursive: true });
  await writeFile(featureFilePath, contents);
}

test("scanSharedUiBoundary passes when shared UI files only import shared owners", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "button.tsx": "export function Button() { return null; }\n",
    "panel.tsx":
      'import { cn } from "../../lib/cn";\nexport function Panel() { return null; }\n',
  });

  try {
    await expect(scanSharedUiBoundary(sharedUiDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanSharedUiBoundary flags feature imports as hard-fail violations", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "widget-frame.tsx":
      'import { AgentCard } from "../../features/bento/components/agent-bento";\nexport function WidgetFrame() { return null; }\n',
  });

  await createFeatureFixture(
    tempRoot,
    "bento/components/agent-bento.tsx",
    "export function AgentCard() { return null; }\n",
  );

  try {
    await expect(scanSharedUiBoundary(sharedUiDir, [])).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          kind: "feature-import",
          relativeFilePath: "src/components/ui/widget-frame.tsx",
          specifier: "../../features/bento/components/agent-bento",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanSharedUiBoundary distinguishes allowlisted debt from new hard-fail violations", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "widget-frame.tsx":
      'import { AgentCard } from "../../features/bento/components/agent-bento";\nexport function WidgetFrame() { return null; }\n',
    "network-shell.tsx":
      'import { useDashboard } from "../../features/dashboard/hooks/use-dashboard";\nexport function NetworkShell() { return null; }\n',
  });

  await createFeatureFixture(
    tempRoot,
    "bento/components/agent-bento.tsx",
    "export function AgentCard() { return null; }\n",
  );
  await createFeatureFixture(
    tempRoot,
    "dashboard/hooks/use-dashboard.ts",
    "export function useDashboard() { return null; }\n",
  );

  try {
    await expect(
      scanSharedUiBoundary(sharedUiDir, [
        {
          importSpecifiers: ["../../features/bento/components/agent-bento"],
          relativeFilePath: "src/components/ui/widget-frame.tsx",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [
        expect.objectContaining({
          relativeFilePath: "src/components/ui/widget-frame.tsx",
        }),
      ],
      staleAllowlistEntries: [],
      violations: [
        expect.objectContaining({
          kind: "feature-network-import",
          relativeFilePath: "src/components/ui/network-shell.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanSharedUiBoundary flags feature state and network runtime imports", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "stateful.tsx":
      'import { create } from "zustand";\nexport const useUi = create(() => ({}));\n',
    "networked.tsx":
      'import { useQuery } from "@tanstack/react-query";\nexport function Networked() { return useQuery({ queryKey: ["x"], queryFn: async () => null }); }\n',
  });

  try {
    const report = await scanSharedUiBoundary(sharedUiDir, []);
    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "feature-state-runtime",
          relativeFilePath: "src/components/ui/stateful.tsx",
        }),
        expect.objectContaining({
          kind: "feature-network-runtime",
          relativeFilePath: "src/components/ui/networked.tsx",
        }),
      ]),
    );
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanSharedUiBoundary reports stale allowlist entries", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "button.tsx": "export function Button() { return null; }\n",
  });

  try {
    await expect(
      scanSharedUiBoundary(sharedUiDir, [
        {
          importSpecifiers: ["../../features/bento/components/agent-bento"],
          relativeFilePath: "src/components/ui/widget-frame.tsx",
        },
      ]),
    ).resolves.toEqual({
      allowlistedDebt: [],
      staleAllowlistEntries: [
        expect.objectContaining({
          allowlistEntry: expect.objectContaining({
            relativeFilePath: "src/components/ui/widget-frame.tsx",
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
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "widget-frame.tsx":
      'import { AgentCard } from "../../features/bento/components/agent-bento";\nexport function WidgetFrame() { return null; }\n',
  });

  await createFeatureFixture(
    tempRoot,
    "bento/components/agent-bento.tsx",
    "export function AgentCard() { return null; }\n",
  );

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_SHARED_DIR: sharedUiDir,
          AGENT_FACTORY_UI_SHARED_UI_BOUNDARY_ALLOWLIST: JSON.stringify([
            {
              importSpecifiers: ["../../features/bento/components/agent-bento"],
              relativeFilePath: "src/components/ui/widget-frame.tsx",
            },
          ]),
        },
      }),
    ).resolves.toMatchObject({
      stdout: expect.stringContaining(
        "Shared UI boundary guard passed with allowlisted legacy debt.",
      ),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI fails with actionable hard-violation output", async () => {
  const { sharedUiDir, tempRoot } = await createSharedUiTree({
    "widget-frame.tsx":
      'import { AgentCard } from "../../features/bento/components/agent-bento";\nexport function WidgetFrame() { return null; }\n',
  });

  await createFeatureFixture(
    tempRoot,
    "bento/components/agent-bento.tsx",
    "export function AgentCard() { return null; }\n",
  );

  try {
    const cliEnv = {
      ...process.env,
      AGENT_FACTORY_UI_SHARED_DIR: sharedUiDir,
      AGENT_FACTORY_UI_SHARED_UI_BOUNDARY_ALLOWLIST: "[]",
    };

    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: cliEnv,
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining("Shared UI boundary guard failed."),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: cliEnv,
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("src/components/ui/widget-frame.tsx"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
