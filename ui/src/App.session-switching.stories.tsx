import { useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { App } from "./App";
import type { DashboardSnapshot } from "./api/dashboard";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import { submitWorkCardControls } from "./stories/dashboardStoryTestUtils";

const defaultSession = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: {
    kind: "default" as const,
  },
};

const betaSession = {
  factoryDir: "/workspace/root/beta",
  folderPath: "/workspace/root",
  id: "session-beta",
  isDefault: false,
  project: "beta",
  target: {
    kind: "named" as const,
    name: "beta",
  },
};

const defaultSessionSnapshot = semanticWorkflowDashboardSnapshot;
const betaSessionSnapshot = buildSessionSnapshot({
  activeWorkLabel: "Beta Story",
  completedCount: 3,
  completedWorkLabel: "Beta Complete",
  dispatchedCount: 5,
  failedCount: 2,
  failedWorkLabel: "Beta Failure",
  inFlightDispatchCount: 1,
  sessionSlug: "beta",
  tickCount: 108,
});

export default {
  title: "Infinite You/Workflow Dashboard/Session Switching",
  component: App,
  tags: ["test"],
};

export const Verification = {
  parameters: {
    dashboardApi: {
      eventSourceMocks: [
        {
          path: "/factories/~default/events",
          snapshot: defaultSessionSnapshot,
        },
        {
          path: "/factories/session-beta/events",
          snapshot: betaSessionSnapshot,
        },
      ],
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: {
              sessions: [defaultSession, betaSession],
            },
          },
        },
      ],
      timelineSnapshots: [defaultSessionSnapshot],
    },
  },
  render: () => <SessionSwitchingStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const currentSelection = await canvas.findByRole("article", {
      name: "Current selection",
    });
    await expect(await within(currentSelection).findByText("Active Story")).toBeVisible();
    await expect(
      await within(currentSelection).findByText("work-active-story"),
    ).toBeVisible();

    const {
      requestField,
      requestNameField,
      workTypeField,
    } = await submitWorkCardControls(canvasElement);
    await userEvent.selectOptions(workTypeField, "story");
    await userEvent.type(requestNameField, "Root session request");
    await userEvent.type(requestField, "Keep the default session isolated.");

    const betaTab = canvas.getByRole("tab", {
      name: "root / beta beta",
    });
    await userEvent.click(betaTab);

    await waitFor(() => {
      expect(betaTab).toHaveAttribute("aria-selected", "true");
    });
    const betaStoryButtons = await canvas.findAllByRole("button", {
      name: /Beta Story/,
    });
    const betaStoryButton = betaStoryButtons[0];
    if (!betaStoryButton) {
      throw new Error("expected at least one Beta Story button");
    }
    await expect(betaStoryButton).toBeVisible();
    const refreshedCurrentSelection = await canvas.findByRole("article", {
      name: "Current selection",
    });
    const refreshedRequestNameField = canvas.getByRole("textbox", {
      name: "Request name",
    });
    const refreshedRequestField = canvas.getByRole("textbox", {
      name: "Request",
    });
    const refreshedWorkTypeField = canvas.getByRole("combobox", {
      name: "Work type",
    });
    await expect(
      await within(refreshedCurrentSelection).findByText("Beta Story"),
    ).toBeVisible();
    await expect(
      await within(refreshedCurrentSelection).findByText("work-beta-story"),
    ).toBeVisible();
    await waitFor(() => {
      expect(
        within(refreshedCurrentSelection).queryByText("work-active-story"),
      ).toBeNull();
    });
    await expect(refreshedRequestNameField).toHaveValue("");
    await expect(refreshedRequestField).toHaveValue("");
    await expect(refreshedWorkTypeField).toHaveValue("");
  },
};

function SessionSwitchingStory() {
  useEffect(() => {
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  }, []);

  return (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  );
}

function buildSessionSnapshot({
  activeWorkLabel,
  completedCount,
  completedWorkLabel,
  dispatchedCount,
  failedCount,
  failedWorkLabel,
  inFlightDispatchCount,
  sessionSlug,
  tickCount,
}: {
  activeWorkLabel: string;
  completedCount: number;
  completedWorkLabel: string;
  dispatchedCount: number;
  failedCount: number;
  failedWorkLabel: string;
  inFlightDispatchCount: number;
  sessionSlug: string;
  tickCount: number;
}): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  renameActiveStory(snapshot, {
    dispatchID: `dispatch-review-${sessionSlug}`,
    label: activeWorkLabel,
    providerSessionID: `sess-${sessionSlug}-story`,
    traceID: `trace-${sessionSlug}-story`,
    workID: `work-${sessionSlug}-story`,
  });

  snapshot.tick_count = tickCount;
  snapshot.runtime.in_flight_dispatch_count = inFlightDispatchCount;
  snapshot.runtime.session.completed_count = completedCount;
  snapshot.runtime.session.completed_work_labels = [completedWorkLabel];
  snapshot.runtime.session.dispatched_count = dispatchedCount;
  snapshot.runtime.session.failed_count = failedCount;
  snapshot.runtime.session.failed_work_labels = [failedWorkLabel];

  if (snapshot.runtime.session.failed_work_details_by_work_id) {
    for (const details of Object.values(
      snapshot.runtime.session.failed_work_details_by_work_id,
    )) {
      details.work_item.display_name = failedWorkLabel;
    }
  }

  return snapshot;
}

function renameActiveStory(
  snapshot: DashboardSnapshot,
  {
    dispatchID,
    label,
    providerSessionID,
    traceID,
    workID,
  }: {
    dispatchID: string;
    label: string;
    providerSessionID: string;
    traceID: string;
    workID: string;
  },
) {
  const previousDispatchID = "dispatch-review-active";
  const previousTraceID = "trace-active-story";
  const previousWorkID = "work-active-story";
  const previousProviderSessionID = "sess-active-story";

  snapshot.runtime.active_dispatch_ids = snapshot.runtime.active_dispatch_ids?.map(
    (activeDispatchID) =>
      activeDispatchID === previousDispatchID ? dispatchID : activeDispatchID,
  );
  snapshot.runtime.current_work_items_by_place_id = updateWorkItemMap(
    snapshot.runtime.current_work_items_by_place_id,
    { label, traceID, workID },
  );
  snapshot.runtime.place_occupancy_work_items_by_place_id = updateWorkItemMap(
    snapshot.runtime.place_occupancy_work_items_by_place_id,
    { label, traceID, workID },
  );

  const previousExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.[previousDispatchID];
  if (previousExecution && snapshot.runtime.active_executions_by_dispatch_id) {
    delete snapshot.runtime.active_executions_by_dispatch_id[previousDispatchID];
    snapshot.runtime.active_executions_by_dispatch_id[dispatchID] = {
      ...previousExecution,
      consumed_tokens: previousExecution.consumed_tokens?.map((token) => ({
        ...token,
        name: token.name === "Active Story" ? label : token.name,
        trace_id: token.trace_id === previousTraceID ? traceID : token.trace_id,
        work_id: token.work_id === previousWorkID ? workID : token.work_id,
      })),
      dispatch_id: dispatchID,
      trace_ids: previousExecution.trace_ids?.map((value) =>
        value === previousTraceID ? traceID : value,
      ),
      work_items: previousExecution.work_items?.map((workItem) => ({
        ...workItem,
        display_name: workItem.display_name === "Active Story" ? label : workItem.display_name,
        trace_id: workItem.trace_id === previousTraceID ? traceID : workItem.trace_id,
        work_id: workItem.work_id === previousWorkID ? workID : workItem.work_id,
      })),
    };
  }

  snapshot.runtime.workstation_activity_by_node_id = Object.fromEntries(
    Object.entries(snapshot.runtime.workstation_activity_by_node_id ?? {}).map(
      ([nodeID, activity]) => [
        nodeID,
        {
          ...activity,
          active_dispatch_ids: activity.active_dispatch_ids?.map((value) =>
            value === previousDispatchID ? dispatchID : value,
          ),
          active_work_items: activity.active_work_items?.map((workItem) => ({
            ...workItem,
            display_name: workItem.display_name === "Active Story" ? label : workItem.display_name,
            trace_id: workItem.trace_id === previousTraceID ? traceID : workItem.trace_id,
            work_id: workItem.work_id === previousWorkID ? workID : workItem.work_id,
          })),
          trace_ids: activity.trace_ids?.map((value) =>
            value === previousTraceID ? traceID : value,
          ),
        },
      ],
    ),
  );

  snapshot.runtime.session.provider_sessions = snapshot.runtime.session.provider_sessions?.map(
    (attempt) => ({
      ...attempt,
      dispatch_id: attempt.dispatch_id === previousDispatchID ? dispatchID : attempt.dispatch_id,
      provider_session:
        attempt.provider_session?.id === previousProviderSessionID
          ? {
              ...attempt.provider_session,
              id: providerSessionID,
            }
          : attempt.provider_session,
      work_items: attempt.work_items?.map((workItem) => ({
        ...workItem,
        display_name: workItem.display_name === "Active Story" ? label : workItem.display_name,
        trace_id: workItem.trace_id === previousTraceID ? traceID : workItem.trace_id,
        work_id: workItem.work_id === previousWorkID ? workID : workItem.work_id,
      })),
    }),
  );
}

function updateWorkItemMap(
  workItemMap: DashboardSnapshot["runtime"]["current_work_items_by_place_id"],
  {
    label,
    traceID,
    workID,
  }: {
    label: string;
    traceID: string;
    workID: string;
  },
) {
  if (!workItemMap) {
    return workItemMap;
  }

  return Object.fromEntries(
    Object.entries(workItemMap).map(([placeID, workItems]) => [
      placeID,
      workItems?.map((workItem) => ({
        ...workItem,
        display_name: workItem.display_name === "Active Story" ? label : workItem.display_name,
        trace_id: workItem.trace_id === "trace-active-story" ? traceID : workItem.trace_id,
        work_id: workItem.work_id === "work-active-story" ? workID : workItem.work_id,
      })),
    ]),
  );
}
