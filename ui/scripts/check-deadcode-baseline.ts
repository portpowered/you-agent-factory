import { spawnSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";

type KnipSymbol = {
  name: string;
  namespace?: string;
};

type KnipIssue = {
  file: string;
  exports?: KnipSymbol[];
  files?: KnipSymbol[];
  types?: KnipSymbol[];
};

type KnipReport = {
  issues?: KnipIssue[];
};

type NormalizedIssue = {
  file: string;
  kind: "export" | "file" | "type";
  name: string;
  namespace?: string;
};

const uiRoot = resolve(import.meta.dir, "..");
const repoRoot = resolve(uiRoot, "..");
const baselinePath = resolve(
  repoRoot,
  "docs/development/frontend-deadcode-baseline.json",
);
const currentPath = resolve(repoRoot, "bin/frontend-deadcode-current.json");
const updateBaseline = process.argv.includes("--update-baseline");

const knip = spawnSync(
  "bunx",
  [
    "--no-install",
    "knip",
    "--include",
    "exports,types,files",
    "--no-progress",
    "--reporter",
    "json",
    "--no-exit-code",
  ],
  {
    cwd: uiRoot,
    encoding: "utf8",
  },
);

if (knip.error) {
  throw knip.error;
}

if (knip.status !== 0) {
  process.stderr.write(knip.stderr);
  process.exit(knip.status ?? 1);
}

const current = normalizeReport(JSON.parse(knip.stdout) as KnipReport);
writeJson(currentPath, current);

if (updateBaseline) {
  writeJson(baselinePath, current);
  process.stdout.write(
    `Updated ${relativeToRepo(baselinePath)} with ${current.length} accepted issues.\n`,
  );
  process.exit(0);
}

const baseline = readJson(baselinePath);
const baselineText = JSON.stringify(baseline, null, 2);
const currentText = JSON.stringify(current, null, 2);

if (currentText === baselineText) {
  process.stdout.write(
    `Frontend dead-code baseline matched ${current.length} accepted issues.\n`,
  );
  process.exit(0);
}

const baselineKeys = new Set(baseline.map(issueKey));
const currentKeys = new Set(current.map(issueKey));
const added = current.filter((issue) => !baselineKeys.has(issueKey(issue)));
const removed = baseline.filter((issue) => !currentKeys.has(issueKey(issue)));

process.stderr.write("Frontend dead-code baseline drift detected.\n");
if (added.length > 0) {
  process.stderr.write(`New unused frontend code:\n${formatIssues(added)}\n`);
}
if (removed.length > 0) {
  process.stderr.write(
    `Baseline entries no longer reported:\n${formatIssues(removed)}\n`,
  );
}
process.stderr.write(
  `Current report written to ${relativeToRepo(currentPath)}.\n`,
);
process.stderr.write(
  "Remove the unused code or intentionally update the reviewed baseline.\n",
);
process.exit(1);

function normalizeReport(report: KnipReport): NormalizedIssue[] {
  const issues: NormalizedIssue[] = [];
  for (const issue of report.issues ?? []) {
    for (const entry of issue.files ?? []) {
      issues.push({ file: issue.file, kind: "file", name: entry.name });
    }
    for (const entry of issue.exports ?? []) {
      issues.push(normalizeSymbol(issue.file, "export", entry));
    }
    for (const entry of issue.types ?? []) {
      issues.push(normalizeSymbol(issue.file, "type", entry));
    }
  }
  return issues.sort((a, b) => issueKey(a).localeCompare(issueKey(b)));
}

function normalizeSymbol(
  file: string,
  kind: NormalizedIssue["kind"],
  symbol: KnipSymbol,
): NormalizedIssue {
  const issue: NormalizedIssue = { file, kind, name: symbol.name };
  if (symbol.namespace) {
    issue.namespace = symbol.namespace;
  }
  return issue;
}

function readJson(path: string): NormalizedIssue[] {
  return JSON.parse(readFileSync(path, "utf8")) as NormalizedIssue[];
}

function writeJson(path: string, issues: NormalizedIssue[]) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `${JSON.stringify(issues, null, 2)}\n`);
}

function issueKey(issue: NormalizedIssue): string {
  return [issue.file, issue.kind, issue.namespace ?? "", issue.name].join("\0");
}

function formatIssues(issues: NormalizedIssue[]): string {
  return issues
    .map(
      (issue) =>
        `- ${issue.file} ${issue.kind} ${issue.namespace ? `${issue.namespace}.` : ""}${issue.name}`,
    )
    .join("\n");
}

function relativeToRepo(path: string): string {
  return path.startsWith(`${repoRoot}/`)
    ? path.slice(repoRoot.length + 1)
    : path;
}
