// @vitest-environment node

import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";

import {
  runHardcodedUiCopyCheck,
  scanSourceTextForHardcodedCopy,
} from "./check-hardcoded-ui-copy";

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

test("runHardcodedUiCopyCheck rejects hardcoded JSX text, attributes, and component props with an empty baseline", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "hardcoded-copy-rejects-"),
  );
  const srcDir = path.join(tempRoot, "src");
  const baselinePath = path.join(tempRoot, "hardcoded-ui-copy-baseline.txt");
  const reportMessages: string[] = [];

  try {
    await mkdir(path.join(srcDir, "features"), { recursive: true });
    await writeFile(
      path.join(srcDir, "features", "feature.tsx"),
      `
        export function Feature() {
          return (
            <section aria-label="Retry panel">
              Retry request
              <MetricCard title="Attempt history" />
            </section>
          );
        }
      `,
    );
    await writeFile(
      baselinePath,
      "# Baseline for the hardcoded UI copy check.\n# Entries are path|line|column|kind|text.\n",
    );

    await expect(
      runHardcodedUiCopyCheck({
        baselinePath,
        report: (message) => reportMessages.push(message),
        sourceRoot: srcDir,
      }),
    ).resolves.toBe(false);

    expect(reportMessages).toEqual(
      expect.arrayContaining([
        expect.stringContaining("New hardcoded UI copy was found"),
        expect.stringContaining("[jsx-attribute] Retry panel"),
        expect.stringContaining("[jsx-text] Retry request"),
        expect.stringContaining("[jsx-prop] Attempt history"),
      ]),
    );
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("runHardcodedUiCopyCheck allows catalogs, tests, stories, and documented diagnostics", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "hardcoded-copy-allows-"),
  );
  const srcDir = path.join(tempRoot, "src");
  const baselinePath = path.join(tempRoot, "hardcoded-ui-copy-baseline.txt");

  try {
    await mkdir(path.join(srcDir, "features", "orders", "messages"), {
      recursive: true,
    });
    await writeFile(
      path.join(srcDir, "features", "orders", "messages", "orders.ts"),
      `
        export const orderMessages = {
          en: {
            title: "Retry request",
          },
        };
      `,
    );
    await writeFile(
      path.join(srcDir, "features", "orders", "orders.test.tsx"),
      `
        export function Fixture() {
          return <section aria-label="Retry panel">Retry request</section>;
        }
      `,
    );
    await writeFile(
      path.join(srcDir, "features", "orders", "orders.stories.tsx"),
      `
        export function Story() {
          return <section aria-label="Retry panel">Retry request</section>;
        }
      `,
    );
    await writeFile(
      path.join(srcDir, "features", "orders", "orders.tsx"),
      `
        export function Diagnostic({ eventType }: { eventType: string }) {
          return (
            <section>
              {/* hardcoded-ui-copy-exception: non-product-diagnostic */}
              <p>{\`type=\${eventType}\`}</p>
            </section>
          );
        }
      `,
    );
    await writeFile(
      baselinePath,
      "# Baseline for the hardcoded UI copy check.\n# Entries are path|line|column|kind|text.\n",
    );

    await expect(
      runHardcodedUiCopyCheck({
        baselinePath,
        sourceRoot: srcDir,
      }),
    ).resolves.toBe(true);
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("runHardcodedUiCopyCheck writes the reviewed baseline for current findings", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "hardcoded-copy-baseline-"),
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

    await expect(
      runHardcodedUiCopyCheck({
        baselinePath,
        shouldWriteBaseline: true,
        sourceRoot: srcDir,
      }),
    ).resolves.toBe(true);

    await expect(readFile(baselinePath, "utf8")).resolves.toContain(
      "src/features/feature.tsx",
    );
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});

test("runHardcodedUiCopyCheck reports stale baseline entries in process", async () => {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "hardcoded-copy-stale-"),
  );
  const srcDir = path.join(tempRoot, "src");
  const baselinePath = path.join(tempRoot, "hardcoded-ui-copy-baseline.txt");
  const reportMessages: string[] = [];

  try {
    await mkdir(path.join(srcDir, "features"), { recursive: true });
    await writeFile(
      path.join(srcDir, "features", "feature.tsx"),
      `
        export function Feature() {
          return <section />;
        }
      `,
    );
    await writeFile(
      baselinePath,
      [
        "# Baseline for the hardcoded UI copy check.",
        "# Entries are path|line|column|kind|text.",
        "src/features/feature.tsx|3|28|jsx-expression|Retry request",
        "",
      ].join("\n"),
    );

    await expect(
      runHardcodedUiCopyCheck({
        baselinePath,
        report: (message) => reportMessages.push(message),
        sourceRoot: srcDir,
      }),
    ).resolves.toBe(false);

    expect(reportMessages).toEqual([
      "The hardcoded UI copy baseline has stale entries. Remove them or refresh the baseline after intentional cleanup.",
      "- src/features/feature.tsx|3|28|jsx-expression|Retry request",
    ]);
  } finally {
    await rm(tempRoot, { force: true, recursive: true });
  }
});
