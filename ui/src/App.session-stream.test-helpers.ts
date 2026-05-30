import type { DashboardSnapshot } from "./api/dashboard";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "./features/dashboard/state/dashboardStreamStore";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";
import type { MockEventSource } from "./testing/app-shell-test-utils";
import { selectedTickTimelineEvents } from "./testing/app-shell-timeline-test-utils";

export function requireEventStream(
  instances: MockEventSource[],
): MockEventSource {
  const stream = instances.at(-1);

  if (!stream) {
    throw new Error("expected factory event stream to be opened");
  }

  return stream;
}

export function resetTimelineForInitialStreamLoad(): void {
  useFactoryTimelineStore.setState({
    events: [],
    latestTick: 0,
    mode: "current",
    receivedEventIDs: [],
    selectedTick: 0,
    worldViewCache: {},
  });
  useDashboardStreamStore.setState({
    streamState: createDefaultDashboardStreamState(),
  });
}

export function emitTimelineMessages(
  stream: MockEventSource,
  events = selectedTickTimelineEvents,
): void {
  for (const event of events) {
    stream.emit("message", event);
  }
}

export function buildBetaSessionSnapshot(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.tick_count = 108;
  snapshot.runtime.session.completed_count = 3;
  snapshot.runtime.session.dispatched_count = 5;
  snapshot.runtime.session.failed_count = 2;

  const renameWorkItem = (
    workItem: NonNullable<
      DashboardSnapshot["runtime"]["current_work_items_by_place_id"]
    >[string][number],
  ) => ({
    ...workItem,
    display_name:
      workItem.display_name === "Active Story"
        ? "Beta Story"
        : workItem.display_name,
    trace_id:
      workItem.trace_id === "trace-active-story"
        ? "trace-beta-story"
        : workItem.trace_id,
    work_id:
      workItem.work_id === "work-active-story"
        ? "work-beta-story"
        : workItem.work_id,
  });

  snapshot.runtime.current_work_items_by_place_id = Object.fromEntries(
    Object.entries(snapshot.runtime.current_work_items_by_place_id ?? {}).map(
      ([placeID, workItems]) => [placeID, workItems?.map(renameWorkItem)],
    ),
  );

  return snapshot;
}
