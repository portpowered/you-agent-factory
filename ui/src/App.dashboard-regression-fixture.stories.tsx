import { useLayoutEffect, useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import { App } from "./App";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import {
  dashboardRegressionSessionByID,
  dashboardRegressionSessionLists,
} from "./components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useDashboardSessionStore } from "./features/dashboard/state/dashboardSessionStore";
import { sessionTabLabel } from "./features/header/lib/dashboard-session-tabs-utils";

const regressionSessions = dashboardRegressionSessionLists.initial;

export default {
  title: "you-agent-factory/Workflow Dashboard/Regression Fixture",
  component: App,
  decorators: [
    (Story: () => JSX.Element) => (
      <div data-dashboard-regression-fixture style={{ minHeight: "760px" }}>
        <Story />
      </div>
    ),
  ],
  tags: ["test"],
};

export const CanonicalSessionScenario = {
  parameters: {
    dashboardApi: {
      eventSourceMocks: regressionSessions.map((session) => ({
        path: `/factory-sessions/${session.id}/events`,
        snapshot: semanticWorkflowDashboardSnapshot,
      })),
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: { body: { sessions: regressionSessions } },
        },
      ],
      timelineSnapshots: [semanticWorkflowDashboardSnapshot],
    },
  },
  render: () => <DashboardRegressionStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const defaultTab = await canvas.findByRole("tab", {
      name: sessionTabLabel(
        dashboardRegressionSessionByID[regressionSessions[0].id],
      ),
    });
    const secondaryTab = await canvas.findByRole("tab", {
      name: sessionTabLabel(
        dashboardRegressionSessionByID[regressionSessions[1].id],
      ),
    });

    await expect(defaultTab).toHaveAttribute("aria-selected", "true");
    await expect(secondaryTab).toHaveAttribute("aria-selected", "false");
    expect(canvasElement.querySelectorAll('[role="tab"]')).toHaveLength(2);
    expect(canvasElement.textContent).not.toContain(DEFAULT_FACTORY_SESSION_ID);

    await userEvent.click(secondaryTab);
    await expect(secondaryTab).toHaveAttribute("aria-selected", "true");
    await expect(defaultTab).toHaveAttribute("aria-selected", "false");
  },
};

function DashboardRegressionStory() {
  const [isReady, setIsReady] = useState(false);

  useLayoutEffect(() => {
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    setIsReady(true);
  }, []);

  return isReady ? <App /> : null;
}
