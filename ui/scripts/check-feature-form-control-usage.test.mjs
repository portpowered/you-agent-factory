import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { scanFeatureFormControlUsage } from "./check-feature-form-control-usage.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-feature-form-control-usage.mjs",
);

async function createSourceTree(files) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "feature-form-control-guard-"),
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

test("scanFeatureFormControlUsage allows approved EnumSelect helpers from the components package", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/submit-work/components/submit-work-card.tsx": `
      import { EnumSelect } from "@you-agent-factory/components";

      export function SubmitWorkCard() {
        return (
          <EnumSelect
            id="work-type"
            onValueChange={() => {}}
            options={[{ label: "Story", value: "story" }]}
            value="story"
          />
        );
      }
    `,
  });

  try {
    await expect(scanFeatureFormControlUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFormControlUsage allows approved EnumSelect helpers in feature components", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/submit-work/components/submit-work-card.tsx": `
      import { EnumSelect } from "../../../components/ui/enum-select";

      export function SubmitWorkCard() {
        return (
          <EnumSelect
            id="work-type"
            onValueChange={() => {}}
            options={[{ label: "Story", value: "story" }]}
            value="story"
          />
        );
      }
    `,
  });

  try {
    await expect(scanFeatureFormControlUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFormControlUsage rejects raw select and native select in feature components", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/forms/components/export-dialog.tsx": `
      import { NativeSelect } from "../../../components/ui/native-select";

      export function ExportDialog() {
        return (
          <>
            <select aria-label="Mode">
              <option value="csv">CSV</option>
            </select>
            <NativeSelect aria-label="Format">
              <option value="json">JSON</option>
            </NativeSelect>
          </>
        );
      }
    `,
  });

  try {
    const report = await scanFeatureFormControlUsage(srcDir, []);
    expect(report.staleAllowlistEntries).toEqual([]);
    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "blocked-select-import",
          relativeFilePath: "src/features/forms/components/export-dialog.tsx",
        }),
        expect.objectContaining({
          kind: "raw-select",
          relativeFilePath: "src/features/forms/components/export-dialog.tsx",
        }),
        expect.objectContaining({
          kind: "native-select",
          relativeFilePath: "src/features/forms/components/export-dialog.tsx",
        }),
      ]),
    );
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFormControlUsage rejects direct Radix select primitive composition", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/forms/components/work-type-field.tsx": `
      import {
        Select,
        SelectContent,
        SelectItem,
        SelectTrigger,
        SelectValue,
      } from "../../../components/ui/select";

      export function WorkTypeField() {
        return (
          <Select onValueChange={() => {}} value="story">
            <SelectTrigger id="work-type">
              <SelectValue placeholder="Choose work type" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="story">Story</SelectItem>
            </SelectContent>
          </Select>
        );
      }
    `,
  });

  try {
    await expect(scanFeatureFormControlUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: expect.arrayContaining([
        expect.objectContaining({
          kind: "blocked-select-import",
          relativeFilePath: "src/features/forms/components/work-type-field.tsx",
        }),
        expect.objectContaining({
          kind: "select-primitive",
          relativeFilePath: "src/features/forms/components/work-type-field.tsx",
        }),
      ]),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("scanFeatureFormControlUsage ignores non-feature-component files", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "components/ui/native-select.tsx": `
      export function NativeSelect() {
        return <select aria-label="Mode"><option value="csv">CSV</option></select>;
      }
    `,
  });

  try {
    await expect(scanFeatureFormControlUsage(srcDir, [])).resolves.toEqual({
      staleAllowlistEntries: [],
      violations: [],
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("CLI output reports actionable feature select guidance", async () => {
  const { srcDir, tempRoot } = await createSourceTree({
    "features/forms/components/export-dialog.tsx": `
      export function ExportDialog() {
        return <select aria-label="Mode"><option value="csv">CSV</option></select>;
      }
    `,
  });

  try {
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FORM_CONTROL_USAGE_ALLOWLIST: "[]",
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "Feature form-control usage guard failed.",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_FORM_CONTROL_USAGE_ALLOWLIST: "[]",
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining("EnumSelect"),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
