import { readFileSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const uiRoot = join(scriptDir, "..");
const entriesPath = join(uiRoot, ".warning-inventory", "console-entries.jsonl");
const rankedPath = join(uiRoot, ".warning-inventory", "ranked-warnings.json");

const raw = readFileSync(entriesPath, "utf8").trim();
if (!raw) {
  writeFileSync(rankedPath, "[]\n");
  console.log("No console entries captured.");
  process.exit(0);
}

const entries = raw
  .split("\n")
  .filter(Boolean)
  .map((line) => JSON.parse(line));

function categorize(message) {
  const normalized = message.toLowerCase();
  if (normalized.includes("not wrapped in act")) {
    return "react-act";
  }
  if (normalized.includes("react does not recognize")) {
    return "invalid-dom-props";
  }
  if (
    normalized.includes("controlled") &&
    normalized.includes("uncontrolled")
  ) {
    return "controlled-input";
  }
  if (normalized.includes("each child in a list should have a unique")) {
    return "missing-keys";
  }
  if (
    normalized.includes("aria-") ||
    normalized.includes("dialogcontent") ||
    normalized.includes("accessibility")
  ) {
    return "accessibility";
  }
  if (
    normalized.includes("deprecated") ||
    normalized.includes("legacy context")
  ) {
    return "deprecated-api";
  }
  if (
    normalized.includes("react flow") ||
    normalized.includes("@xyflow") ||
    normalized.includes("node")
  ) {
    return "graph-harness";
  }
  return "other";
}

function normalizeMessage(message) {
  return message
    .replace(/\s+/g, " ")
    .replace(/at .+$/g, "")
    .trim()
    .slice(0, 240);
}

const byCategory = new Map();
for (const entry of entries) {
  const category = categorize(entry.message);
  const messageKey = normalizeMessage(entry.message);
  const bucketKey = `${category}::${messageKey}`;
  const bucket = byCategory.get(bucketKey) ?? {
    category,
    message: messageKey,
    level: entry.level,
    count: 0,
    files: new Set(),
    tests: new Set(),
  };
  bucket.count += 1;
  if (entry.testFile) {
    bucket.files.add(entry.testFile.replace(uiRoot + "/", ""));
  }
  if (entry.testFile && entry.testName) {
    bucket.tests.add(
      `${entry.testFile.replace(uiRoot + "/", "")} > ${entry.testName}`,
    );
  }
  byCategory.set(bucketKey, bucket);
}

const ranked = [...byCategory.values()]
  .sort((a, b) => b.count - a.count)
  .map((bucket) => ({
    category: bucket.category,
    level: bucket.level,
    count: bucket.count,
    message: bucket.message,
    files: [...bucket.files].sort(),
    sampleTests: [...bucket.tests].sort().slice(0, 8),
  }));

writeFileSync(rankedPath, JSON.stringify(ranked, null, 2));
console.log(`Summarized ${entries.length} entries into ${ranked.length} buckets.`);
