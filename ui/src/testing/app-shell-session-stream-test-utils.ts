import type { DashboardSnapshot } from "../api/dashboard";
import type { FactorySessionSummary } from "../api/factory-sessions/api";
import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { seedTimelineSnapshot } from "./app-shell-timeline-seed-utils";

export class MockEventSource {
  public static instances: MockEventSource[] = [];

  public closed = false;
  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;

  private readonly listeners = new Map<string, EventListener[]>();

  public constructor(public readonly url: string) {
    MockEventSource.instances.push(this);
  }

  public addEventListener(type: string, listener: EventListener): void {
    const existing = this.listeners.get(type) ?? [];
    existing.push(listener);
    this.listeners.set(type, existing);
  }

  public close(): void {
    this.closed = true;
  }

  public emit(type: string, data: unknown): void {
    if (type === "snapshot") {
      const state = useFactoryTimelineStore.getState();
      const tracesByWorkID =
        state.worldViewCache[state.selectedTick]?.tracesByWorkID ?? {};
      seedTimelineSnapshot(data as DashboardSnapshot, tracesByWorkID);
    }

    const event = new MessageEvent(type, {
      data: JSON.stringify(data),
    });

    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

export const defaultFactorySessionSummary: FactorySessionSummary = {
  factoryDir: "/workspace/default",
  folderPath: "/workspace",
  id: DEFAULT_FACTORY_SESSION_ID,
  isDefault: true,
  project: "default",
  target: {
    kind: "default",
  },
};

export function fetchRequestPath(input: RequestInfo | URL): string {
  if (typeof input === "string") {
    return input.startsWith("http") ? new URL(input).pathname : input;
  }

  if (input instanceof URL) {
    return `${input.pathname}${input.search}`;
  }

  return input.url.startsWith("http") ? new URL(input.url).pathname : input.url;
}
