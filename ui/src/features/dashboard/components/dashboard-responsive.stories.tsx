import { expect, userEvent, within } from "storybook/test";
import { App } from "../../../App";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  activeStoryTrace,
  expectNoPageHorizontalOverflow,
} from "../../../stories/dashboardStorySupport";

export default {
  title: "you-agent-factory/Workflow Dashboard",
  component: App,
  decorators: [
    (Story: () => JSX.Element) => (
      <div style={{ height: "960px", minHeight: "760px" }}>
        <Story />
      </div>
    ),
  ],
};

export const DashboardImprovementsSmokeNarrow = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => (
    <div
      data-dashboard-narrow-frame
      style={{ maxWidth: "100%", width: "320px" }}
    >
      <App />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const frame = canvasElement.querySelector("[data-dashboard-narrow-frame]");

    await expect(
      await canvas.findByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await userEvent.click(
      (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = await canvas.findByRole("region", {
      name: "you-agent-factory bento board",
    });
    const dashboardScope = within(dashboardGrid);

    await expect(
      dashboardScope.getByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Current selection" }),
    ).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Trace drill-down" }),
    ).toBeVisible();
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(320);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const DashboardResponsiveEmpty = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: { body: { sessions: [] } },
        },
      ],
    },
  },
  render: () => <App />,
};

export const DashboardResponsiveError = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: { code: "NETWORK_ERROR", message: "Sessions are offline." },
            status: 503,
          },
        },
      ],
    },
  },
  render: () => <App />,
};

export const DashboardResponsiveLoading = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: () => new Promise(() => {}),
        },
      ],
    },
  },
  render: () => <App />,
};
