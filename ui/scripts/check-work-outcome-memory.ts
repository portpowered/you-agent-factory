import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import type { FactoryEvent } from "../src/api/events";
import { buildWorkOutcomeTimelineSamplesFromEvents } from "../src/features/work-outcome/useWorkOutcomeChart";

type FixtureName =
  | "baseline"
  | "failure-analysis"
  | "graph-state-smoke"
  | "runtime-config"
  | "runtime-details";

interface Options {
  amplify: number;
  fixture: FixtureName;
  maxHeapDeltaMB: number;
}

const dirname = path.dirname(fileURLToPath(import.meta.url));
const packageRoot = path.resolve(dirname, "..");
const fixtureDirectory = path.join(packageRoot, "integration", "fixtures");
const defaultFixture: FixtureName = "runtime-config";
const defaultAmplify = 60;
const defaultMaxHeapDeltaMB = 64;

const fixturePathByName: Record<FixtureName, string> = {
  baseline: path.join(fixtureDirectory, "event-stream-replay.jsonl"),
  "failure-analysis": path.join(fixtureDirectory, "failure-analysis-replay.jsonl"),
  "graph-state-smoke": path.join(fixtureDirectory, "graph-state-smoke-replay.jsonl"),
  "runtime-config": path.join(fixtureDirectory, "event-stream-replay-2.jsonl"),
  "runtime-details": path.join(fixtureDirectory, "runtime-details-replay.jsonl"),
};

function parseArgs(argv: string[]): Options {
  const options: Options = {
    amplify: defaultAmplify,
    fixture: defaultFixture,
    maxHeapDeltaMB: defaultMaxHeapDeltaMB,
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--fixture") {
      const value = argv[index + 1] as FixtureName | undefined;
      if (!value || !(value in fixturePathByName)) {
        throw new Error(`Unknown fixture '${value ?? "<missing>"}'`);
      }
      options.fixture = value;
      index += 1;
      continue;
    }
    if (arg === "--amplify") {
      options.amplify = positiveInteger(argv[index + 1], "--amplify");
      index += 1;
      continue;
    }
    if (arg === "--max-heap-delta-mb") {
      options.maxHeapDeltaMB = positiveNumber(
        argv[index + 1],
        "--max-heap-delta-mb",
      );
      index += 1;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }

  return options;
}

function positiveInteger(value: string | undefined, flagName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${flagName} must be a positive integer, received ${value ?? "<missing>"}`);
  }
  return parsed;
}

function positiveNumber(value: string | undefined, flagName: string): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${flagName} must be a positive number, received ${value ?? "<missing>"}`);
  }
  return parsed;
}

function loadFixtureEvents(fixture: FixtureName): FactoryEvent[] {
  const fixturePath = fixturePathByName[fixture];
  const text = fs.readFileSync(fixturePath, "utf8");
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
    .map((line) => JSON.parse(line) as FactoryEvent);
}

function amplifyEvents(events: FactoryEvent[], copies: number): FactoryEvent[] {
  if (copies === 1) {
    return structuredClone(events);
  }

  const latestBaseTick = events.reduce(
    (maxTick, event) => Math.max(maxTick, event.context.tick),
    0,
  );
  const amplified: FactoryEvent[] = [];

  for (let copyIndex = 0; copyIndex < copies; copyIndex += 1) {
    const tickOffset = copyIndex * (latestBaseTick + 1);
    for (const event of events) {
      const cloned = structuredClone(event);
      cloned.id = `${event.id}#memory-copy-${copyIndex}`;
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

function gcIfAvailable(): void {
  if (typeof Bun !== "undefined" && typeof Bun.gc === "function") {
    Bun.gc(true);
  }
}

function mb(bytes: number): number {
  return Number((bytes / (1024 * 1024)).toFixed(2));
}

function sampleHeapUsedMB(): number {
  return mb(process.memoryUsage().heapUsed);
}

function latestTick(events: FactoryEvent[]): number {
  return events.reduce(
    (maxTick, event) => Math.max(maxTick, event.context.tick),
    0,
  );
}

function main(): void {
  const options = parseArgs(process.argv.slice(2));
  const sourceEvents = loadFixtureEvents(options.fixture);
  const events = amplifyEvents(sourceEvents, options.amplify);
  const selectedTick = latestTick(events);

  gcIfAvailable();
  const heapBeforeMB = sampleHeapUsedMB();
  const startedAt = performance.now();
  const samples = buildWorkOutcomeTimelineSamplesFromEvents(events, selectedTick);
  const durationMs = Number((performance.now() - startedAt).toFixed(1));
  gcIfAvailable();
  const heapAfterMB = sampleHeapUsedMB();
  const heapDeltaMB = Number((heapAfterMB - heapBeforeMB).toFixed(2));

  const summary = {
    amplify: options.amplify,
    durationMs,
    eventCount: events.length,
    fixture: options.fixture,
    heapAfterMB,
    heapBeforeMB,
    heapDeltaMB,
    maxHeapDeltaMB: options.maxHeapDeltaMB,
    sampleCount: samples.length,
    selectedTick,
  };

  console.log(JSON.stringify(summary, null, 2));

  if (heapDeltaMB > options.maxHeapDeltaMB) {
    throw new Error(
      `Work outcome timeline memory regression: heap delta ${heapDeltaMB} MB exceeded ${options.maxHeapDeltaMB} MB.`,
    );
  }
}

main();
