import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";

import { App } from "./App";
import {
  failureAnalysisTimelineEvents,
  resourceCountTimelineEvents,
} from "./components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { useExportDialogStore } from "./features/export/state/exportDialogStore";
import {
  activeStoryTrace,
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
  title: "Infinite You/Workflow Dashboard",
  component: App,
};

const providerSessionVerificationSessionID =
  "019e44f4-580e-7f32-981e-1e54ec6907d6";
const providerSessionVerificationSnapshot = {
  ...inferenceDetailsSnapshot,
  runtime: {
    ...inferenceDetailsSnapshot.runtime,
    session: {
      ...inferenceDetailsSnapshot.runtime.session,
      provider_sessions: (
        inferenceDetailsSnapshot.runtime.session.provider_sessions ?? []
      ).map((attempt) =>
        attempt.dispatch_id === "dispatch-review-active"
          ? {
              ...attempt,
              provider_session: attempt.provider_session
                ? {
                    ...attempt.provider_session,
                    id: providerSessionVerificationSessionID,
                  }
                : attempt.provider_session,
            }
          : attempt,
      ),
    },
  },
};

export const WorkChartTimelineVerification = {
  parameters: {
    dashboardApi: {
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

    await expect(await canvas.findByText("Tick 2 of 5")).toBeVisible();
    expect(canvas.queryByText("Current")).toBeNull();
    expectWorkOutcomeSeries(outcomeChart);

    await userEvent.click(
      await canvas.findByRole("button", { name: "Return to current tick" }),
    );

    await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);
  },
};

export const HeaderActionButtonsVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
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
      const currentButton = within(toolbar).getByRole("button", {
        name: "Return to current tick",
      });

      await expect(exportButton).toHaveAttribute(
        "data-dashboard-header-action",
        "neutral",
      );
      await expect(currentButton).toHaveAttribute(
        "data-dashboard-header-action",
        "neutral",
      );

      const slider = await canvas.findByRole<HTMLInputElement>("slider", {
        name: "Timeline tick",
      });
      fireEvent.change(slider, { target: { value: "2" } });

      await expect(await canvas.findByText("Tick 2 of 5")).toBeVisible();
      await expect(currentButton).toBeEnabled();

      currentButton.focus();
      await userEvent.keyboard("{Enter}");
      await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();

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

export const ProviderSessionDetailVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: `/provider-sessions/detail?id=${providerSessionVerificationSessionID}&kind=session_id&provider=codex`,
          response: {
            body: {
              parse: {
                eventCount: 3,
                functionCalls: [],
                lineCount: 3,
                malformedLineCount: 0,
                parseErrors: [],
                reasoning: [],
                tokenUsage: {
                  cachedInputTokens: 0,
                  inputTokens: 18,
                  outputTokens: 9,
                  reasoningOutputTokens: 0,
                  totalTokens: 27,
                },
                turns: [
                  {
                    eventCount: 3,
                    functionCallCount: 0,
                    index: 1,
                    reasoningCount: 0,
                    responseItemCount: 1,
                    startedAt: "2026-05-20T17:35:24Z",
                  },
                ],
                unknownEventCount: 0,
                unknownEvents: [],
              },
              providerSession: {
                id: providerSessionVerificationSessionID,
                kind: "session_id",
                provider: "codex",
              },
              source: {
                modifiedAt: "2026-05-20T17:35:24Z",
                relativePath:
                  "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
                sizeBytes: 2048,
              },
            },
          },
        },
      ],
      snapshot: providerSessionVerificationSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const selectProviderSessionButton = await currentSelection.findByRole(
      "button",
      {
        name: `Select provider session codex / session_id / ${providerSessionVerificationSessionID} for dispatch dispatch-review-active`,
      },
    );

    await userEvent.click(selectProviderSessionButton);

    await expect(
      await currentSelection.findByRole("heading", {
        name: "Selected session details",
      }),
    ).toBeVisible();
    await expect(
      await currentSelection.findByRole("heading", { name: "Source file" }),
    ).toBeVisible();
    await expect(
      await currentSelection.findByText(
        "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
      ),
    ).toBeVisible();
    await expect(
      await currentSelection.findByRole("heading", { name: "Token usage" }),
    ).toBeVisible();
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
    await expect(await canvas.findByText("Tick 4 of 4")).toBeVisible();

    await userEvent.click(
      await canvas.findByRole("button", { name: "Blocked Analysis Story" }),
    );

    const failedSelection = within(currentSelectionCard(canvasElement));
    await expect(
      failedSelection.getByRole("heading", { name: "Error details" }),
    ).toBeVisible();
    expect(
      failedSelection.getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    await expect(
      failedSelection.getByText("No inference events are available for this selected work item."),
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
    await expect(await canvas.findByText("Tick 4 of 4")).toBeVisible();
    await expect(
      await canvas.findByLabelText("2 resource tokens"),
    ).toBeVisible();

    fireEvent.change(slider, { target: { value: "3" } });

    await expect(await canvas.findByText("Tick 3 of 4")).toBeVisible();
    await expect(
      await canvas.findByLabelText("1 resource tokens"),
    ).toBeVisible();

    fireEvent.change(slider, { target: { value: "1" } });

    await expect(await canvas.findByText("Tick 1 of 4")).toBeVisible();
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
