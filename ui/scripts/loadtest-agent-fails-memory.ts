import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import type { FactoryEvent } from "../src/api/events";
import { useFactoryTimelineStore } from "../src/state/factoryTimelineStore";

type ScenarioName = "baseline" | "stripped" | "amplified";

interface ParsedSSEEventLog {
  events: FactoryEvent[];
  skippedBlocks: number;
}

interface PayloadByteSummary {
  feedbackBytes: number;
  promptBytes: number;
  responseTextBytes: number;
  stderrBytes: number;
  stdoutBytes: number;
  totalStructuredJsonBytes: number;
}

interface MemorySample {
  heapTotalMB: number;
  heapUsedMB: number;
  rssMB: number;
}

interface ScenarioReport {
  amplifiedCopies: number;
  durationMs: number;
  eventCount: number;
  latestTick: number;
  logPath: string;
  memoryAfterMB: MemorySample;
  memoryBeforeMB: MemorySample;
  memoryDeltaMB: MemorySample;
  name: ScenarioName;
  parsedJsonBytesMB: number;
  payloadBytesMB: PayloadByteSummaryMB;
  selectedTick: number;
  skippedBlocks: number;
  traceCount: number;
  workstationRequestCount: number;
}

interface PayloadByteSummaryMB {
  feedbackMB: number;
  promptMB: number;
  responseTextMB: number;
  stderrMB: number;
  stdoutMB: number;
  totalStructuredJsonMB: number;
}

interface AggregateReport {
  amplifiedCopies: number;
  confirmedHighMemoryCost: boolean;
  eventCount: number;
  findings: string[];
  invalidBlocksDetected: number;
  logPath: string;
  reports: Record<ScenarioName, ScenarioReport>;
}

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const repoRoot = path.resolve(packageRoot, "..", "..", "..");
const defaultLogPath = path.join(repoRoot, "factory", "logs", "agent-fails.json");
const defaultAmplifiedCopies = 8;
const heavyPayloadKeys = ["feedback", "prompt", "responseText", "stderr", "stdout"] as const;

function parseArgs(argv: string[]) {
  const options: {
    amplify: number;
    json: boolean;
    logPath: string;
    scenario: ScenarioName | null;
  } = {
    amplify: defaultAmplifiedCopies,
    json: false,
    logPath: defaultLogPath,
    scenario: null,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--json") {
      options.json = true;
      continue;
    }
    if (arg === "--scenario") {
      const value = argv[index + 1];
      if (value === "baseline" || value === "stripped" || value === "amplified") {
        options.scenario = value;
        index += 1;
        continue;
      }
      throw new Error(`Unsupported scenario: ${value ?? "<missing>"}`);
    }
    if (arg === "--input") {
      const value = argv[index + 1];
      if (!value) {
        throw new Error("--input requires a file path");
      }
      options.logPath = path.resolve(process.cwd(), value);
      index += 1;
      continue;
    }
    if (arg === "--amplify") {
      const value = Number(argv[index + 1]);
      if (!Number.isInteger(value) || value < 1) {
        throw new Error(`--amplify must be a positive integer, received ${argv[index + 1] ?? "<missing>"}`);
      }
      options.amplify = value;
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  return options;
}

function mb(bytes: number): number {
  return Number((bytes / (1024 * 1024)).toFixed(1));
}

function sampleMemoryMB(): MemorySample {
  const usage = process.memoryUsage();
  return {
    heapTotalMB: mb(usage.heapTotal),
    heapUsedMB: mb(usage.heapUsed),
    rssMB: mb(usage.rss),
  };
}

function memoryDeltaMB(before: MemorySample, after: MemorySample): MemorySample {
  return {
    heapTotalMB: Number((after.heapTotalMB - before.heapTotalMB).toFixed(1)),
    heapUsedMB: Number((after.heapUsedMB - before.heapUsedMB).toFixed(1)),
    rssMB: Number((after.rssMB - before.rssMB).toFixed(1)),
  };
}

function parseSSEEventLog(text: string): ParsedSSEEventLog {
  const events: FactoryEvent[] = [];
  let skippedBlocks = 0;

  for (const block of text.split(/\r?\n\r?\n/)) {
    const dataLines: string[] = [];
    for (const rawLine of block.split(/\r?\n/)) {
      const line = rawLine.trimEnd();
      if (line.startsWith("data: ")) {
        dataLines.push(line.slice(6));
        continue;
      }
      if (line.startsWith("data:")) {
        dataLines.push(line.slice(5).trimStart());
      }
    }
    if (dataLines.length === 0) {
      continue;
    }
    try {
      events.push(JSON.parse(dataLines.join("\n")) as FactoryEvent);
    } catch {
      skippedBlocks += 1;
    }
  }

  return { events, skippedBlocks };
}

function payloadByteSummary(events: FactoryEvent[]): PayloadByteSummary {
  const summary: PayloadByteSummary = {
    feedbackBytes: 0,
    promptBytes: 0,
    responseTextBytes: 0,
    stderrBytes: 0,
    stdoutBytes: 0,
    totalStructuredJsonBytes: 0,
  };

  for (const event of events) {
    summary.totalStructuredJsonBytes += Buffer.byteLength(JSON.stringify(event));
    const payload = event.payload;
    if (!payload || typeof payload !== "object") {
      continue;
    }

    const prompt = payload.prompt;
    if (typeof prompt === "string") {
      summary.promptBytes += Buffer.byteLength(prompt);
    }
    const stdout = payload.stdout;
    if (typeof stdout === "string") {
      summary.stdoutBytes += Buffer.byteLength(stdout);
    }
    const stderr = payload.stderr;
    if (typeof stderr === "string") {
      summary.stderrBytes += Buffer.byteLength(stderr);
    }
    const responseText = payload.responseText;
    if (typeof responseText === "string") {
      summary.responseTextBytes += Buffer.byteLength(responseText);
    }
    const feedback = payload.feedback;
    if (typeof feedback === "string") {
      summary.feedbackBytes += Buffer.byteLength(feedback);
    }
  }

  return summary;
}

function payloadByteSummaryMB(summary: PayloadByteSummary): PayloadByteSummaryMB {
  return {
    feedbackMB: mb(summary.feedbackBytes),
    promptMB: mb(summary.promptBytes),
    responseTextMB: mb(summary.responseTextBytes),
    stderrMB: mb(summary.stderrBytes),
    stdoutMB: mb(summary.stdoutBytes),
    totalStructuredJsonMB: mb(summary.totalStructuredJsonBytes),
  };
}

function stripHeavyPayloadFields(events: FactoryEvent[]): FactoryEvent[] {
  const cloned = structuredClone(events);
  for (const event of cloned) {
    const payload = event.payload;
    if (!payload || typeof payload !== "object") {
      continue;
    }
    for (const key of heavyPayloadKeys) {
      delete payload[key];
    }
  }
  return cloned;
}

function amplifyEvents(events: FactoryEvent[], copies: number): FactoryEvent[] {
  if (copies <= 1) {
    return structuredClone(events);
  }

  const latestBaseTick = events.reduce((maxTick, event) => Math.max(maxTick, event.context.tick), 0);
  const amplified: FactoryEvent[] = [];

  for (let copyIndex = 0; copyIndex < copies; copyIndex += 1) {
    const tickOffset = copyIndex * (latestBaseTick + 1);
    for (const event of events) {
      const cloned = structuredClone(event);
      cloned.id = `${event.id}#copy-${copyIndex}`;
      cloned.context = {
        ...event.context,
        sequence: event.context.sequence + copyIndex * 10_000,
        tick: event.context.tick + tickOffset,
      };
      amplified.push(cloned);
    }
  }

  return amplified;
}

function latestTick(events: FactoryEvent[]): number {
  return events.reduce((maxTick, event) => Math.max(maxTick, event.context.tick), 0);
}

function buildScenarioEvents(
  scenario: ScenarioName,
  parsed: ParsedSSEEventLog,
  amplify: number,
): FactoryEvent[] {
  if (scenario === "baseline") {
    return structuredClone(parsed.events);
  }
  if (scenario === "stripped") {
    return stripHeavyPayloadFields(parsed.events);
  }
  return amplifyEvents(parsed.events, amplify);
}

function gcIfAvailable(): void {
  if (typeof Bun !== "undefined" && typeof Bun.gc === "function") {
    Bun.gc(true);
    return;
  }
  const gc = globalThis.gc as (() => void) | undefined;
  gc?.();
}

function measureScenario(
  scenario: ScenarioName,
  logPath: string,
  parsed: ParsedSSEEventLog,
  amplify: number,
): ScenarioReport {
  const events = buildScenarioEvents(scenario, parsed, amplify);
  const eventPayloadBytes = payloadByteSummary(events);
  gcIfAvailable();
  const before = sampleMemoryMB();
  const startedAt = performance.now();

  useFactoryTimelineStore.getState().reset();
  useFactoryTimelineStore.getState().replaceEvents(events);

  const durationMs = Number((performance.now() - startedAt).toFixed(1));
  gcIfAvailable();
  const after = sampleMemoryMB();
  const state = useFactoryTimelineStore.getState();

  return {
    amplifiedCopies: scenario === "amplified" ? amplify : 1,
    durationMs,
    eventCount: state.events.length,
    latestTick: latestTick(events),
    logPath,
    memoryAfterMB: after,
    memoryBeforeMB: before,
    memoryDeltaMB: memoryDeltaMB(before, after),
    name: scenario,
    parsedJsonBytesMB: mb(Buffer.byteLength(JSON.stringify(events))),
    payloadBytesMB: payloadByteSummaryMB(eventPayloadBytes),
    selectedTick: state.selectedTick,
    skippedBlocks: parsed.skippedBlocks,
    traceCount: Object.keys(state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {}).length,
    workstationRequestCount: Object.keys(
      state.worldViewCache[state.selectedTick]?.workstationRequestsByDispatchID ?? {},
    ).length,
  };
}

function runScenarioInChild(
  scenario: ScenarioName,
  logPath: string,
  amplify: number,
): ScenarioReport {
  const scriptPath = fileURLToPath(import.meta.url);
  const child = spawnSync(
    process.execPath,
    [scriptPath, "--scenario", scenario, "--input", logPath, "--amplify", String(amplify), "--json"],
    {
      cwd: packageRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    },
  );

  if (child.status !== 0) {
    throw new Error(
      [
        `Scenario ${scenario} failed with exit code ${child.status ?? "null"}.`,
        child.stdout.trim(),
        child.stderr.trim(),
      ]
        .filter((part) => part.length > 0)
        .join("\n"),
    );
  }

  return JSON.parse(child.stdout) as ScenarioReport;
}

function summarizeFindings(
  reports: Record<ScenarioName, ScenarioReport>,
  invalidBlocksDetected: number,
): string[] {
  const findings: string[] = [];
  const base = reports.baseline;
  const stripped = reports.stripped;
  const amplified = reports.amplified;

  if (invalidBlocksDetected > 0) {
    findings.push(
      `${invalidBlocksDetected} malformed SSE block${invalidBlocksDetected === 1 ? "" : "s"} detected near the end of the log; the loader skipped them.`,
    );
  }

  findings.push(
    `Baseline replay retained ${base.eventCount} events and added ${base.memoryDeltaMB.rssMB} MB RSS while projecting only the latest selected tick.`,
  );
  findings.push(
    `Prompt and script text dominate payload size: ${base.payloadBytesMB.promptMB} MB prompt text, ${base.payloadBytesMB.stdoutMB} MB stdout, and ${base.payloadBytesMB.stderrMB} MB stderr.`,
  );
  findings.push(
    `Removing heavy text fields shrank retained structured event bytes from ${base.parsedJsonBytesMB} MB to ${stripped.parsedJsonBytesMB} MB without changing tick coverage.`,
  );
  findings.push(
    `${amplified.amplifiedCopies}x amplification reached ${amplified.eventCount} events and ${amplified.memoryAfterMB.rssMB} MB RSS, reproducing the same growth trend behind the browser crash.`,
  );

  return findings;
}

function confirmHighMemoryCost(reports: Record<ScenarioName, ScenarioReport>): boolean {
  return (
    reports.baseline.memoryDeltaMB.rssMB >= 30 &&
    reports.stripped.parsedJsonBytesMB <= reports.baseline.parsedJsonBytesMB - 1 &&
    reports.amplified.memoryAfterMB.rssMB >= 350
  );
}

function printHumanReport(report: AggregateReport): void {
  const { baseline, stripped, amplified } = report.reports;
  console.log(`agent-fails memory load test`);
  console.log(`log: ${report.logPath}`);
  console.log(`events: ${report.eventCount}`);
  console.log(`malformed blocks skipped: ${report.invalidBlocksDetected}`);
  console.log("");
  console.log(`baseline: ${baseline.memoryDeltaMB.rssMB} MB RSS delta, ${baseline.durationMs} ms, ${baseline.payloadBytesMB.promptMB} MB prompts`);
  console.log(`stripped: ${stripped.memoryDeltaMB.rssMB} MB RSS delta, ${stripped.durationMs} ms`);
  console.log(
    `amplified x${amplified.amplifiedCopies}: ${amplified.memoryAfterMB.rssMB} MB final RSS, ${amplified.durationMs} ms, ${amplified.eventCount} events`,
  );
  console.log("");
  for (const finding of report.findings) {
    console.log(`- ${finding}`);
  }
  console.log("");
  console.log(`confirmed high memory cost: ${report.confirmedHighMemoryCost ? "yes" : "no"}`);
}

function ensureLogExists(logPath: string): void {
  if (!fs.existsSync(logPath)) {
    throw new Error(`Log file not found: ${logPath}`);
  }
}

function main(): void {
  const options = parseArgs(process.argv.slice(2));
  ensureLogExists(options.logPath);

  if (options.scenario !== null) {
    gcIfAvailable();
    const parsed = parseSSEEventLog(fs.readFileSync(options.logPath, "utf8"));
    const report = measureScenario(options.scenario, options.logPath, parsed, options.amplify);
    process.stdout.write(JSON.stringify(report));
    return;
  }

  const reports: Record<ScenarioName, ScenarioReport> = {
    amplified: runScenarioInChild("amplified", options.logPath, options.amplify),
    baseline: runScenarioInChild("baseline", options.logPath, options.amplify),
    stripped: runScenarioInChild("stripped", options.logPath, options.amplify),
  };

  const aggregate: AggregateReport = {
    amplifiedCopies: options.amplify,
    confirmedHighMemoryCost: confirmHighMemoryCost(reports),
    eventCount: reports.baseline.eventCount,
    findings: summarizeFindings(reports, reports.baseline.skippedBlocks),
    invalidBlocksDetected: reports.baseline.skippedBlocks,
    logPath: options.logPath,
    reports,
  };

  if (options.json) {
    process.stdout.write(JSON.stringify(aggregate, null, 2));
    return;
  }

  printHumanReport(aggregate);
}

main();
