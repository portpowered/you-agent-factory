import type { FactoryEvent } from "../../../api/events";

const textEncoder = new TextEncoder();

const DEFAULT_MAX_EVENT_TEXT_CHARS = 2_048;
const HEAVY_PAYLOAD_KEYS = [
  "feedback",
  "output",
  "prompt",
  "response",
  "stderr",
  "stdout",
] as const;

const MAX_TOP_EVENTS = 5;
const TRUNCATION_SUFFIX_PREFIX = "\n\n[truncated ";

export const FACTORY_TIMELINE_DEBUG_GLOBAL = "__agentFactoryTimelineDebug__";
export const FACTORY_TIMELINE_DEBUG_STORAGE_KEY =
  "agentFactory.timelineDebugSummary";

type HeavyPayloadKey = (typeof HEAVY_PAYLOAD_KEYS)[number];

export interface FactoryTimelineDebugOptions {
  compactEventText: boolean;
  maxEventTextChars: number;
  memoryDebug: boolean;
}

export interface FactoryTimelineHeavyPayloadSummary {
  feedbackBytesMB: number;
  outputBytesMB: number;
  promptBytesMB: number;
  responseBytesMB: number;
  stderrBytesMB: number;
  stdoutBytesMB: number;
}

export interface FactoryTimelineTopEventSummary {
  estimatedJsonBytesMB: number;
  heavyPayloadBytesMB: number;
  id: string;
  tick: number;
  type: string;
}

export interface FactoryTimelineMemorySummary {
  approxStructuredEventBytesMB: number;
  eventCount: number;
  heavyPayloadBytesMB: FactoryTimelineHeavyPayloadSummary;
  jsHeapLimitMB?: number;
  jsHeapUsedMB?: number;
  selectedTick: number;
  timestamp: string;
  topEvents: FactoryTimelineTopEventSummary[];
}

interface BrowserStorageLike {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
}

interface BrowserWindowLike {
  location: {
    search: string;
  };
  localStorage?: BrowserStorageLike;
  performance?: Performance & {
    memory?: {
      jsHeapSizeLimit: number;
      totalJSHeapSize: number;
      usedJSHeapSize: number;
    };
  };
}

function byteLength(value: string): number {
  return textEncoder.encode(value).length;
}

function mb(bytes: number): number {
  return Number((bytes / (1024 * 1024)).toFixed(2));
}

function truncateText(value: string, maxChars: number): string {
  if (value.length <= maxChars) {
    return value;
  }
  const removedChars = value.length - maxChars;
  return `${value.slice(0, maxChars)}${TRUNCATION_SUFFIX_PREFIX}${removedChars} chars]`;
}

function parseBooleanFlag(rawValue: string | null): boolean {
  return rawValue === "1" || rawValue === "true";
}

function parsePositiveInteger(rawValue: string | null): number | null {
  if (rawValue === null) {
    return null;
  }
  const parsed = Number(rawValue);
  if (!Number.isInteger(parsed) || parsed < 1) {
    return null;
  }
  return parsed;
}

function clonePayloadWithCompaction(
  payload: Record<string, unknown>,
  maxEventTextChars: number,
): Record<string, unknown> {
  let changed = false;
  const clonedPayload = { ...payload };

  for (const key of HEAVY_PAYLOAD_KEYS) {
    const currentValue = clonedPayload[key];
    if (typeof currentValue !== "string") {
      continue;
    }
    const nextValue = truncateText(currentValue, maxEventTextChars);
    if (nextValue !== currentValue) {
      clonedPayload[key] = nextValue;
      changed = true;
    }
  }

  return changed ? clonedPayload : payload;
}

function heavyPayloadByteSummary(
  events: FactoryEvent[],
): Record<HeavyPayloadKey, number> {
  const summary: Record<HeavyPayloadKey, number> = {
    feedback: 0,
    output: 0,
    prompt: 0,
    response: 0,
    stderr: 0,
    stdout: 0,
  };

  for (const event of events) {
    if (!event.payload || typeof event.payload !== "object") {
      continue;
    }
    for (const key of HEAVY_PAYLOAD_KEYS) {
      const value = (event.payload as Record<string, unknown>)[key];
      if (typeof value === "string") {
        summary[key] += byteLength(value);
      }
    }
  }

  return summary;
}

function heavyPayloadBytesForEvent(event: FactoryEvent): number {
  if (!event.payload || typeof event.payload !== "object") {
    return 0;
  }

  return HEAVY_PAYLOAD_KEYS.reduce((total, key) => {
    const value = (event.payload as Record<string, unknown>)[key];
    return total + (typeof value === "string" ? byteLength(value) : 0);
  }, 0);
}

export function readFactoryTimelineDebugOptions(
  browserWindow: BrowserWindowLike | undefined = globalThis.window,
): FactoryTimelineDebugOptions {
  const search = browserWindow?.location.search ?? "";
  const params = new URLSearchParams(search);

  return {
    compactEventText: parseBooleanFlag(params.get("afCompactEventText")),
    maxEventTextChars:
      parsePositiveInteger(params.get("afMaxEventTextChars")) ??
      DEFAULT_MAX_EVENT_TEXT_CHARS,
    memoryDebug: parseBooleanFlag(params.get("afMemoryDebug")),
  };
}

export function compactFactoryEventForTimeline(
  event: FactoryEvent,
  options: FactoryTimelineDebugOptions,
): FactoryEvent {
  if (!options.compactEventText) {
    return event;
  }
  if (!event.payload || typeof event.payload !== "object") {
    return event;
  }

  const payload = clonePayloadWithCompaction(
    event.payload as Record<string, unknown>,
    options.maxEventTextChars,
  );
  if (payload === event.payload) {
    return event;
  }

  return {
    ...event,
    payload,
  };
}

export function summarizeFactoryTimelineMemory(
  events: FactoryEvent[],
  selectedTick: number,
  browserWindow: BrowserWindowLike | undefined = globalThis.window,
): FactoryTimelineMemorySummary {
  const heavyPayload = heavyPayloadByteSummary(events);
  const performanceMemory = browserWindow?.performance?.memory;

  return {
    approxStructuredEventBytesMB: mb(byteLength(JSON.stringify(events))),
    eventCount: events.length,
    heavyPayloadBytesMB: {
      feedbackBytesMB: mb(heavyPayload.feedback),
      outputBytesMB: mb(heavyPayload.output),
      promptBytesMB: mb(heavyPayload.prompt),
      responseBytesMB: mb(heavyPayload.response),
      stderrBytesMB: mb(heavyPayload.stderr),
      stdoutBytesMB: mb(heavyPayload.stdout),
    },
    jsHeapLimitMB:
      performanceMemory?.jsHeapSizeLimit !== undefined
        ? mb(performanceMemory.jsHeapSizeLimit)
        : undefined,
    jsHeapUsedMB:
      performanceMemory?.usedJSHeapSize !== undefined
        ? mb(performanceMemory.usedJSHeapSize)
        : undefined,
    selectedTick,
    timestamp: new Date().toISOString(),
    topEvents: [...events]
      .map((event) => ({
        estimatedJsonBytesMB: mb(byteLength(JSON.stringify(event))),
        heavyPayloadBytesMB: mb(heavyPayloadBytesForEvent(event)),
        id: event.id,
        tick: event.context.tick,
        type: event.type,
      }))
      .sort((left, right) => {
        if (right.heavyPayloadBytesMB !== left.heavyPayloadBytesMB) {
          return right.heavyPayloadBytesMB - left.heavyPayloadBytesMB;
        }
        return right.estimatedJsonBytesMB - left.estimatedJsonBytesMB;
      })
      .slice(0, MAX_TOP_EVENTS),
  };
}

export function persistFactoryTimelineMemorySummary(
  storage: BrowserStorageLike | undefined,
  summary: FactoryTimelineMemorySummary,
): void {
  storage?.setItem(
    FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
    JSON.stringify(summary, null, 2),
  );
}

export function readPersistedFactoryTimelineMemorySummary(
  storage: BrowserStorageLike | undefined,
): FactoryTimelineMemorySummary | null {
  const rawValue = storage?.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY);
  if (!rawValue) {
    return null;
  }
  return JSON.parse(rawValue) as FactoryTimelineMemorySummary;
}

export interface FactoryTimelineDebugGlobal {
  options: FactoryTimelineDebugOptions;
  readPersistedSummary: () => FactoryTimelineMemorySummary | null;
  summarize: () => FactoryTimelineMemorySummary;
}

declare global {
  interface Window {
    __agentFactoryTimelineDebug__?: FactoryTimelineDebugGlobal;
  }
}

export function installFactoryTimelineDebugGlobal(
  browserWindow: BrowserWindowLike & Window,
  getState: () => { events: FactoryEvent[]; selectedTick: number },
  options: FactoryTimelineDebugOptions,
): void {
  browserWindow[FACTORY_TIMELINE_DEBUG_GLOBAL] = {
    options,
    readPersistedSummary: () =>
      readPersistedFactoryTimelineMemorySummary(browserWindow.localStorage),
    summarize: () => {
      const state = getState();
      return summarizeFactoryTimelineMemory(
        state.events,
        state.selectedTick,
        browserWindow,
      );
    },
  };
}


