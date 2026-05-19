// @vitest-environment node

import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

import { scanSourceTextForHardcodedCopy } from "./check-hardcoded-ui-copy";

const execFileAsync = promisify(execFile);
const scriptPath = fileURLToPath(
  new URL("./check-hardcoded-ui-copy.ts", import.meta.url),
);

test("scanSourceTextForHardcodedCopy catches rendered string expressions and visible component prop copy", () => {
  const findings = scanSourceTextForHardcodedCopy(
    "src/features/current-selection/provider-session-detail-panel.tsx",
    `
      export function SessionPanel({ index }: { index: number }) {
        return (
          <section>
            {"Retry request"}
            <DetailMetric label="Input" value={3} />
            <DetailMetric label="dispatchedCount" value={3} />
            <strong>{\`Turn \${index}\`}</strong>
          </section>
        );
      }
    `,
  );

  expect(findings).toEqual([
    expect.objectContaining({ kind: "jsx-expression", text: "Retry request" }),
    expect.objectContaining({ kind: "jsx-prop", text: "Input" }),
    expect.objectContaining({ kind: "jsx-expression", text: "Turn" }),
  ]);
});

test("scanSourceTextForHardcodedCopy catches non-JSX rendered string literals", () => {
  const findings = scanSourceTextForHardcodedCopy(
    "src/features/current-selection/provider-session-attempts.tsx",
    `
      export function History({
        collapseActionLabel = "Collapse",
        title = "Run history",
      }: {
        collapseActionLabel?: string;
        title?: string;
      }) {
        return title;
      }

      export function errorState() {
        return {
          message: "Provider-session details are unavailable.",
        };
      }

      export function outcomeLabel(outcome: string) {
        if (outcome === "FAILED") {
          return "Failed";
        }
        return \`Raw outcome: \${outcome}\`;
      }
    `,
  );

  expect(findings).toEqual([
    expect.objectContaining({ kind: "string-literal", text: "Collapse" }),
    expect.objectContaining({ kind: "string-literal", text: "Run history" }),
    expect.objectContaining({
      kind: "string-literal",
      text: "Provider-session details are unavailable.",
    }),
    expect.objectContaining({ kind: "string-literal", text: "Failed" }),
    expect.objectContaining({ kind: "string-literal", text: "Raw outcome:" }),
  ]);
});

test("scanSourceTextForHardcodedCopy catches rendered fallback and validation assignment strings", () => {
  const findings = scanSourceTextForHardcodedCopy(
    "src/features/current-selection/state-node-detail.tsx",
    `
      export function StateNodeDetail({ value }: { value?: string }) {
        const validationErrors: { model?: string } = {};
        validationErrors.model = "Enter a model before saving this workstation.";

        return <dd>{value || "Unknown"}</dd>;
      }
    `,
  );

  expect(findings).toEqual([
    expect.objectContaining({
      kind: "string-literal",
      text: "Enter a model before saving this workstation.",
    }),
    expect.objectContaining({ kind: "string-literal", text: "Unknown" }),
  ]);
});

test("scanSourceTextForHardcodedCopy ignores documented non-product diagnostic exceptions", () => {
  const findings = scanSourceTextForHardcodedCopy(
    "src/features/current-selection/provider-session-detail-panel.tsx",
    `
      export function SessionPanel({ eventType }: { eventType: string }) {
        return (
          <section>
            {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
            <p>{\`type=\${eventType}\`}</p>
          </section>
        );
      }
    `,
  );

  expect(findings).toEqual([]);
});

test("CLI output reports actionable hardcoded-copy failures", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "hardcoded-copy-guard-"),
  );
  const srcDir = path.join(tempRoot, "src");
  const baselinePath = path.join(tempRoot, "hardcoded-ui-copy-baseline.txt");

  try {
    await mkdir(path.join(srcDir, "features"), { recursive: true });
    await writeFile(
      path.join(srcDir, "features", "feature.tsx"),
      `
        export function Feature() {
          return <section>{"Retry request"}</section>;
        }
      `,
    );
    await writeFile(
      baselinePath,
      "# Baseline for the hardcoded UI copy check.\n# Entries are path|line|column|kind|text.\n",
    );

    await expect(
      execFileAsync("bun", [scriptPath], {
        cwd: tempRoot,
        env: {
          ...process.env,
          AGENT_FACTORY_UI_COPY_BASELINE_PATH: baselinePath,
          AGENT_FACTORY_UI_SRC_DIR: srcDir,
        },
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "Move user-facing copy into a feature-owned catalog",
      ),
    });
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
