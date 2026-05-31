import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { expect, test } from "vitest";

import { scanDashboardExpandDisclosure } from "./check-dashboard-expand-disclosure.mjs";

async function createUiTree(files) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "dashboard-expand-guard-"),
  );

  for (const [relativeFilePath, contents] of Object.entries(files)) {
    const filePath = path.join(tempRoot, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return tempRoot;
}

test("scanDashboardExpandDisclosure accepts ExpandablePanelTrigger ownership", async () => {
  const tempRoot = await createUiTree({
    "src/features/terminal-work/components/terminal-work-card.tsx": `
      import { ExpandablePanelTrigger } from "../../../components/ui";

      export function TerminalWorkCard() {
        return (
          <ExpandablePanelTrigger controlsID="panel" expanded={false} variant="compact">
            Expand
          </ExpandablePanelTrigger>
        );
      }
    `,
  });

  try {
    await expect(
      scanDashboardExpandDisclosure(tempRoot, [
        {
          owner: "expandable-panel-trigger",
          relativeFilePath:
            "src/features/terminal-work/components/terminal-work-card.tsx",
        },
      ]),
    ).resolves.toEqual({ violations: [] });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanDashboardExpandDisclosure rejects raw aria-expanded buttons in guarded paths", async () => {
  const tempRoot = await createUiTree({
    "src/features/terminal-work/components/terminal-work-card.tsx": `
      export function TerminalWorkCard() {
        const expanded = false;
        return <button aria-expanded={expanded} type="button">Expand</button>;
      }
    `,
  });

  try {
    await expect(
      scanDashboardExpandDisclosure(tempRoot, [
        {
          owner: "expandable-panel-trigger",
          relativeFilePath:
            "src/features/terminal-work/components/terminal-work-card.tsx",
        },
      ]),
    ).resolves.toEqual({
      violations: [
        expect.objectContaining({
          kind: "owner",
          relativeFilePath:
            "src/features/terminal-work/components/terminal-work-card.tsx",
        }),
        expect.objectContaining({
          kind: "raw-disclosure-button",
          relativeFilePath:
            "src/features/terminal-work/components/terminal-work-card.tsx",
        }),
      ],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanDashboardExpandDisclosure accepts legend icon-shell ownership", async () => {
  const tempRoot = await createUiTree({
    "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx": `
      import { DisclosureButton } from "../../../components/ui/disclosure-button";
      import { ExpandablePanelIcon } from "../../../components/ui/expandable-panel-icon";

      export function DashboardFlowAxisLegend() {
        return (
          <DisclosureButton controlsID="legend" expanded={false}>
            <ExpandablePanelIcon expanded={false} />
          </DisclosureButton>
        );
      }
    `,
  });

  try {
    await expect(
      scanDashboardExpandDisclosure(tempRoot, [
        {
          owner: "expandable-panel-icon-shell",
          relativeFilePath:
            "src/features/workflow-activity/components/dashboard-flow-axis-legend.tsx",
        },
      ]),
    ).resolves.toEqual({ violations: [] });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
