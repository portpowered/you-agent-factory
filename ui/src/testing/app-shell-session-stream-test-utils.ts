import type { DashboardSnapshot } from "../api/dashboard";
import type { FactorySessionSummary } from "../api/factory-sessions/api";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "./app-shell-session-preflight-test-utils";
import { seedTimelineSnapshot } from "./app-shell-timeline-seed-utils";

export class MockEventSource {
  public static instances: MockEventSource[] = [];

  public closed = false;
  public messageEventCount = 0;
  public onerror: ((event: Event) => void) | null = null;
  public onopen: ((event: Event) => void) | null = null;
  public opened = false;

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

  public open(): void {
    this.opened = true;
    this.onopen?.(new Event("open"));
  }

  public emit(type: string, data: unknown): void {
    if (type === "message") {
      this.messageEventCount += 1;
    }
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
  id: APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
  isDefault: true,
  project: "default",
  target: {
    kind: "default",
  },
};
