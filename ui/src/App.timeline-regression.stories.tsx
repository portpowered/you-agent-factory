import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";

import { App } from "./App";
import {
  failureAnalysisTimelineEvents,
  resourceCountTimelineEvents,
} from "./components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useExportDialogStore } from "./features/export/state";
import {
  activeStoryTrace,
  expectCompactedTopDashboardSection,
  currentSelectionCard,
  expectCurrentSelectionCardID,
  expectNoPageHorizontalOverflow,
  expectTimelineToolbarAlignment,
  expectTypographyRegressionSurface,
  expectWorkOutcomeSeries,
  failedStoryTrace,
  historicalWorkOutcomeSnapshot,
  inferenceDetailsSnapshot,
  liveWorkOutcomeSnapshot,
} from "./stories/dashboardStorySupport";

export default {
  title: "you-agent-factory/Workflow Dashboard",
  component: App,
};

const defaultFactorySessionSummary = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: {
    kind: "default" as const,
  },
};

function renderDashboardAtWidth(width: number) {
  return (
    <div style={{ maxWidth: "100%", width: `${width}px` }}>
      <App />
    </div>
  );
}

export const WorkChartTimelineVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: {
              sessions: [defaultFactorySessionSummary],
            },
          },
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const outcomeChart = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });

    await expect(outcomeChart).toBeVisible();
    await expect(
      within(outcomeChart).getByRole("img", {
        name: "Work outcome chart for Session",
      }),
    ).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);

    const slider = await canvas.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    fireEvent.change(slider, { target: { value: "2" } });

    await expect(await canvas.findByText("2/5")).toBeVisible();
    expect(canvas.queryByText("Current")).toBeNull();
    expectWorkOutcomeSeries(outcomeChart);

    fireEvent.change(slider, { target: { value: "5" } });

    await expect(await canvas.findByText("5/5")).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);
  },
};

export const HeaderActionButtonsVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: {
              sessions: [defaultFactorySessionSummary],
            },
          },
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    try {
      const toolbar = await canvas.findByRole("region", {
        name: "dashboard summary",
      });
      const exportButton = within(toolbar).getByRole("button", {
        name: "Export PNG",
      });
      const sessionButton = within(toolbar).getByRole("button", {
        name: "Open another session",
      });

      await expect(exportButton).toHaveAttribute(
        "data-dashboard-header-action",
        "neutral",
      );
      await expect(sessionButton).toHaveAttribute(
        "data-dashboard-header-action",
        "neutral",
      );
      expect(
        within(toolbar).queryByRole("button", { name: "Return to current tick" }),
      ).toBeNull();

      const slider = await canvas.findByRole<HTMLInputElement>("slider", {
        name: "Timeline tick",
      });
      fireEvent.change(slider, { target: { value: "2" } });

      await expect(await canvas.findByText("2/5")).toBeVisible();
      sessionButton.focus();
      await expect(sessionButton).toHaveFocus();
      fireEvent.change(slider, { target: { value: "5" } });
      await expect(await canvas.findByText("5/5")).toBeVisible();

      exportButton.focus();
      await userEvent.keyboard("{Enter}");
      const dialog = await within(canvasElement.ownerDocument.body).findByRole(
        "dialog",
        {
          name: "Export factory",
        },
      );
      await expect(dialog).toBeVisible();

      const cancelButton = within(dialog).getByRole("button", {
        name: "Cancel",
      });
      cancelButton.focus();
      await userEvent.keyboard("{Enter}");
      await waitFor(() => {
        expect(
          within(canvasElement.ownerDocument.body).queryByRole("dialog", {
            name: "Export factory",
          }),
        ).toBeNull();
      });
    } finally {
      useExportDialogStore.setState({ isExportDialogOpen: false });
    }
  },
};

export const HeaderTimelineAlignmentVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: {
              sessions: [defaultFactorySessionSummary],
            },
          },
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectTimelineToolbarAlignment(canvasElement);
  },
};

function compactTopSectionStory(width: number) {
  return {
    tags: ["test"],
    parameters: {
      dashboardApi: {
        fetchMocks: [
          {
            method: "GET",
            path: "/factory-sessions",
            response: {
              body: {
                sessions: [defaultFactorySessionSummary],
              },
            },
          },
        ],
        timelineSnapshots: [
          historicalWorkOutcomeSnapshot,
          liveWorkOutcomeSnapshot,
        ],
      },
    },
    render: () => renderDashboardAtWidth(width),
    play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
      await expectCompactedTopDashboardSection(canvasElement);
    },
  };
}

export const CompactedTopSectionDesktopVerification =
  compactTopSectionStory(1280);

export const CompactedTopSectionTabletVerification =
  compactTopSectionStory(768);

export const CompactedTopSectionMobileVerification =
  compactTopSectionStory(360);

export const SelectedPositionCurrentWork = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select story:implemented state",
      }),
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getByText("Current work")).toBeVisible();
    await expect(currentSelection.getByText("Active Story")).toBeVisible();
    await expect(currentSelection.getByText("work-active-story")).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const SelectedEmptyPosition = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: "Select story:blocked state" }),
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getByText("Current work")).toBeVisible();
    await expect(
      currentSelection.getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const InferenceCurrentSelectionDetails = {
  parameters: {
    dashboardApi: {
      snapshot: inferenceDetailsSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    expect(
      currentSelection.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    await expect(
      currentSelection.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeVisible();
    await expect(currentSelection.getByText("Current dispatch")).toBeVisible();
    expect(currentSelection.getAllByText(/codex/).length).toBeGreaterThan(0);
    expect(currentSelection.queryByText(/factory-renderer/)).toBeNull();
    expect(currentSelection.queryByText(/sha256:system-runtime/)).toBeNull();
    expect(
      currentSelection.queryByText(
        /Model details are not available for this selected run/,
      ),
    ).toBeNull();
    expect(currentSelection.queryByText("sha256:user-runtime")).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const TerminalFailureDetails = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-failed-story": failedStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: "Failed Story" }),
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getByText("Failed Story")).toBeVisible();
    await expect(
      currentSelection.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeVisible();
    await expect(currentSelection.getByText("Current dispatch")).toBeVisible();
    await expect(
      currentSelection.getByText("Session log unavailable"),
    ).toBeVisible();
    expect(currentSelection.queryByText("Failure reason")).toBeNull();
    expect(currentSelection.queryByText("Failure message")).toBeNull();
    expect(
      currentSelection.queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const FailureAnalysisEventReplaySmoke = {
  parameters: {
    dashboardApi: {
      timelineEvents: failureAnalysisTimelineEvents,
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    const slider = await canvas.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("4");
    await expect(await canvas.findByText("4/4")).toBeVisible();

    await userEvent.click(
      await canvas.findByRole("button", { name: "Blocked Analysis Story" }),
    );

    const failedSelection = within(currentSelectionCard(canvasElement));
    await expect(
      failedSelection.getByRole("region", { name: "Failure details" }),
    ).toBeVisible();
    expect(
      failedSelection.getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    const inferenceAttempts = failedSelection.getByRole("region", {
      name: "Inference attempts",
    });
    await userEvent.click(
      within(inferenceAttempts).getByRole("button", { name: "Expand" }),
    );
    await expect(
      failedSelection.getByText(
        "No inference attempt details were recorded before this dispatch ended.",
      ),
    ).toBeVisible();
    expect(
      failedSelection.getAllByText(
        "Provider rate limit exceeded while generating the analysis.",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      failedSelection.queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();

    await userEvent.click(
      await canvas.findByRole("button", { name: "Select story:new state" }),
    );

    const positionSelection = within(currentSelectionCard(canvasElement));
    await expect(positionSelection.getByText("Current work")).toBeVisible();
    await expect(
      positionSelection.getByText("Queued Analysis Story"),
    ).toBeVisible();
    await expect(
      positionSelection.getByText("work-queued-analysis"),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const ResourceCountEventReplaySmoke = {
  parameters: {
    dashboardApi: {
      timelineEvents: resourceCountTimelineEvents,
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    const slider = await canvas.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await expect(await canvas.findByText("4/4")).toBeVisible();
    await expect(
      await canvas.findByLabelText("2 resource tokens"),
    ).toBeVisible();

    fireEvent.change(slider, { target: { value: "3" } });

    await expect(await canvas.findByText("3/4")).toBeVisible();
    await expect(
      await canvas.findByLabelText("1 resource tokens"),
    ).toBeVisible();

    fireEvent.change(slider, { target: { value: "1" } });

    await expect(await canvas.findByText("1/4")).toBeVisible();
    await expect(
      await canvas.findByLabelText("2 resource tokens"),
    ).toBeVisible();
  },
};

export const TypographyRegression = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectTypographyRegressionSurface(canvasElement);
  },
};

export const TypographyRegressionNarrow = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const frame = canvasElement.firstElementChild;

    await expectTypographyRegressionSurface(canvasElement);
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(360);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};
