import { expect, fireEvent, userEvent, within } from "storybook/test";

import { App } from "./App";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./components/dashboard";
import {
  dashboardWorkstationRequestFixtures,
  failureAnalysisTimelineEvents,
  resourceCountTimelineEvents,
} from "./components/dashboard/fixtures";
import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";

const activeStoryTrace: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: ["work-active-story"],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      transition_id: "review",
      workstation_name: "Review",
      outcome: "ACCEPTED",
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [],
      output_mutations: [],
    },
  ],
};

const historicalWorkOutcomeSnapshot = workOutcomeSnapshot(
  semanticWorkflowDashboardSnapshot,
  2,
  {
    completed: 2,
    completedLabels: ["Historical Story"],
    dispatched: 3,
    failed: 1,
    failedByWorkType: { story: 1 },
    failedLabels: ["Historical Failure"],
    inFlight: 1,
    queued: 2,
  },
);
const liveWorkOutcomeSnapshot = workOutcomeSnapshot(
  semanticWorkflowDashboardSnapshot,
  5,
  {
    completed: 11,
    completedLabels: ["Historical Story", "Live Story"],
    dispatched: 14,
    failed: 4,
    failedByWorkType: { story: 3, task: 1 },
    failedLabels: ["Historical Failure", "Live Failure"],
    inFlight: 2,
    queued: 3,
  },
);
const inferenceDetailsSnapshot = withInferenceDetails(semanticWorkflowDashboardSnapshot);
const markdownReadyWorkstationRequest: DashboardWorkstationRequest = {
  ...dashboardWorkstationRequestFixtures.ready,
  prompt: [
    "## Review checklist",
    "",
    "- Check the latest diff",
    "- Run `bun test` before approval",
    "",
    "```text",
    "bun test",
    "```",
  ].join("\n"),
};
interface WorkOutcomeCounts {
  completed: number;
  failed: number;
  inFlight: number;
  queued: number;
}

interface WorkOutcomeSnapshotOptions extends WorkOutcomeCounts {
  completedLabels: string[];
  dispatched: number;
  failedByWorkType: Record<string, number>;
  failedLabels: string[];
}

function workOutcomeSnapshot(
  source: DashboardSnapshot,
  tickCount: number,
  options: WorkOutcomeSnapshotOptions,
): DashboardSnapshot {
  return {
    ...source,
    tick_count: tickCount,
    runtime: {
      ...source.runtime,
      in_flight_dispatch_count: options.inFlight,
      place_token_counts: {
        ...(source.runtime.place_token_counts ?? {}),
        "story:init": options.queued,
      },
      session: {
        ...source.runtime.session,
        completed_count: options.completed,
        completed_work_labels: options.completedLabels,
        dispatched_count: options.dispatched,
        failed_by_work_type: options.failedByWorkType,
        failed_count: options.failed,
        failed_work_labels: options.failedLabels,
      },
    },
  };
}

function withInferenceDetails(source: DashboardSnapshot): DashboardSnapshot {
  return {
    ...source,
    runtime: {
      ...source.runtime,
      inference_attempts_by_dispatch_id: {
        ...(source.runtime.inference_attempts_by_dispatch_id ?? {}),
        "dispatch-review-active": {
          "dispatch-review-active/inference-request/1": {
            attempt: 1,
            dispatch_id: "dispatch-review-active",
            duration_millis: 520,
            error_class: "provider_rate_limit",
            inference_request_id: "dispatch-review-active/inference-request/1",
            outcome: "FAILED",
            prompt: "Review Active Story and return a decision.",
            request_time: "2026-04-08T12:00:01Z",
            response_time: "2026-04-08T12:00:02Z",
            transition_id: "review",
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          },
          "dispatch-review-active/inference-request/2": {
            attempt: 2,
            dispatch_id: "dispatch-review-active",
            duration_millis: 740,
            inference_request_id: "dispatch-review-active/inference-request/2",
            outcome: "SUCCEEDED",
            prompt: "Retry Active Story after provider recovery.",
            request_time: "2026-04-08T12:00:03Z",
            response: "Active Story is ready for the next workstation.",
            response_time: "2026-04-08T12:00:04Z",
            transition_id: "review",
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          },
        },
      },
      session: {
        ...source.runtime.session,
        provider_sessions: (source.runtime.session.provider_sessions ?? []).map((attempt) =>
          attempt.dispatch_id === "dispatch-review-active"
            ? {
                ...attempt,
                diagnostics: {
                  provider: {
                    model: "gpt-5.4",
                    provider: "codex",
                    request_metadata: {
                      prompt_source: "factory-renderer",
                    },
                  },
                  rendered_prompt: {
                    system_prompt_hash: "sha256:system-runtime",
                    user_message_hash: "sha256:user-runtime",
                  },
                },
              }
            : attempt,
        ),
      },
    },
  };
}

const failedStoryTrace: DashboardTrace = {
  trace_id: "trace-failed-story",
  work_ids: ["work-failed-story"],
  transition_ids: ["repair"],
  workstation_sequence: ["Repair"],
  dispatches: [
    {
      dispatch_id: "dispatch-repair-failed",
      transition_id: "repair",
      workstation_name: "Repair",
      outcome: "FAILED",
      failure_message: "Provider rate limit exceeded while generating the repair.",
      failure_reason: "provider_rate_limit",
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [],
      output_mutations: [],
    },
  ],
};

async function expectGraphWorkstation(
  canvasElement: HTMLElement,
  workstationName: string,
): Promise<HTMLElement> {
  const canvas = within(canvasElement);

  await expect(
    await canvas.findByRole("region", { name: "Work graph viewport" }),
  ).toBeVisible();

  const workstation = await canvas.findByRole("button", { name: workstationName });
  await expect(workstation).toBeVisible();

  return workstation;
}

function expectCurrentSelectionCardID(canvasElement: HTMLElement): void {
  const canvas = within(canvasElement);
  const currentSelection = canvas.getByRole("article", { name: "Current selection" });
  expect(
    currentSelection.closest<HTMLElement>("[data-bento-card-id]")?.dataset.bentoCardId,
  ).toBe("current-selection");
}

function currentSelectionCard(canvasElement: HTMLElement): HTMLElement {
  return within(canvasElement).getByRole("article", { name: "Current selection" });
}

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function expectNoPageHorizontalOverflow(canvasElement: HTMLElement): void {
  const documentElement = canvasElement.ownerDocument.documentElement;
  const overflowTolerance = 1;

  expect(documentElement.scrollWidth <= documentElement.clientWidth + overflowTolerance).toBe(true);
}

async function submitWorkCardControls(canvasElement: HTMLElement): Promise<{
  requestNameField: HTMLElement;
  requestField: HTMLElement;
  scope: ReturnType<typeof within>;
  submitButton: HTMLElement;
  workTypeField: HTMLElement;
}> {
  const canvas = within(canvasElement);
  const submitWorkCard = await canvas.findByRole("article", { name: "Submit work" });
  const submitWorkScope = within(submitWorkCard);
  const workTypeField = submitWorkScope.getByRole("combobox", { name: "Work type" });
  const requestNameField = submitWorkScope.getByRole("textbox", { name: "Request name" });
  const requestField = submitWorkScope.getByRole("textbox", { name: "Request text" });

  return {
    requestNameField,
    requestField,
    scope: submitWorkScope,
    submitButton: submitWorkScope.getByRole("button", { name: "Submit work" }),
    workTypeField,
  };
}

async function fillSubmitWorkCard(
  canvasElement: HTMLElement,
  requestName: string,
  requestText: string,
): Promise<{
  requestNameField: HTMLElement;
  requestField: HTMLElement;
  scope: ReturnType<typeof within>;
  submitButton: HTMLElement;
  workTypeField: HTMLElement;
}> {
  const { requestField, requestNameField, scope, submitButton, workTypeField } =
    await submitWorkCardControls(canvasElement);

  await userEvent.selectOptions(workTypeField, "story");
  await userEvent.clear(requestNameField);
  await userEvent.type(requestNameField, requestName);
  await userEvent.clear(requestField);
  await userEvent.type(requestField, requestText);

  return {
    requestNameField,
    requestField,
    scope,
    submitButton,
    workTypeField,
  };
}

async function expectTypographyRegressionSurface(canvasElement: HTMLElement): Promise<void> {
  const canvas = within(canvasElement);
  const heading = await canvas.findByRole("heading", { name: "Agent Factory" });
  const toolbar = canvas.getByRole("region", { name: "dashboard summary" });
  const summaryList = canvas.getByText("Factory state").closest("dl");

  if (!(summaryList instanceof HTMLDListElement)) {
    throw new Error("expected dashboard summary metadata list");
  }

  expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
  expect(summaryList.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
  expect(summaryList.className).toContain(DASHBOARD_SUPPORTING_LABELS_CLASS);

  await userEvent.click(await canvas.findByRole("button", { name: "Select Review workstation" }));

  const currentSelection = currentSelectionCard(canvasElement);
  const currentSelectionScope = within(currentSelection);
  const activeWorkHeading = currentSelectionScope.getByRole("heading", { name: "Active work" });
  const activeWorkCard = currentSelectionScope.getByText("Active Story").closest("li");
  const runHistorySection = currentSelectionScope
    .getByRole("heading", { name: "Run history" })
    .closest("section");

  if (!(runHistorySection instanceof HTMLElement)) {
    throw new Error("expected current-selection run history section");
  }

  expect(activeWorkHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);
  expect(activeWorkCard?.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
  expect(within(runHistorySection).getByText("2 runs").className).toContain(
    DASHBOARD_SUPPORTING_TEXT_CLASS,
  );
  expect(toolbar.textContent).toContain(String(semanticWorkflowDashboardSnapshot.factory_state));
}

async function selectWorkstationRequest(
  canvasElement: HTMLElement,
  request: DashboardWorkstationRequest,
): Promise<void> {
  await selectWorkstationRequestByDispatchID(canvasElement, request.dispatch_id);
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function selectWorkstationRequestByDispatchID(
  canvasElement: HTMLElement,
  dispatchID: string,
): Promise<void> {
  const canvas = within(canvasElement);
  const requestButtonLabel = `Select workstation request ${dispatchID}`;

  await userEvent.click(
    await canvas.findByRole("button", { name: "Select Review workstation" }),
  );

  const currentSelection = within(currentSelectionCard(canvasElement));
  const directRequestButton = currentSelection.queryByRole("button", {
    name: requestButtonLabel,
  });

  if (directRequestButton) {
    await userEvent.click(directRequestButton);
    return;
  }

  const requestHistorySection = currentSelection
    .queryByRole("heading", { name: "Request history" })
    ?.closest("section");
  if (requestHistorySection instanceof HTMLElement) {
    const requestHistoryScope = within(requestHistorySection);
    const collapsedButton = requestHistoryScope.queryByRole("button", { name: "Expand" });
    if (collapsedButton) {
      await userEvent.click(collapsedButton);
    }

    const historyRequestButton = requestHistoryScope.queryByRole("button", {
      name: new RegExp(`\\(${escapeRegExp(dispatchID)}\\)$`),
    });
    if (historyRequestButton) {
      await userEvent.click(historyRequestButton);
      return;
    }
  }

  const runHistorySection = currentSelection
    .getByRole("heading", { name: "Run history" })
    .closest("section");
  if (runHistorySection instanceof HTMLElement) {
    const runHistoryScope = within(runHistorySection);
    const collapsedButton = runHistoryScope.queryByRole("button", { name: "Expand" });
    if (collapsedButton) {
      await userEvent.click(collapsedButton);
    }

    const historyRequestButton = runHistoryScope.queryByRole("button", {
      name: requestButtonLabel,
    });
    if (historyRequestButton) {
      await userEvent.click(historyRequestButton);
      return;
    }
  }

  throw new Error(`unable to find workstation request controls for ${dispatchID}`);
}

function workstationRequestStoryParameters(request: DashboardWorkstationRequest) {
  return {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
      workstationRequestsByDispatchID: {
        [request.dispatch_id]: request,
      },
    },
  };
}

function workstationRequestWithStartedAt(
  request: DashboardWorkstationRequest,
  startedAt: string,
): DashboardWorkstationRequest {
  return {
    ...request,
    request_view: request.request_view
      ? {
          ...request.request_view,
          request_time: startedAt,
          started_at: startedAt,
        }
      : request.request_view,
    started_at: startedAt,
  };
}

function selectedWorkDispatchHistoryStoryParameters() {
  const active = workstationRequestWithStartedAt(
    {
      ...dashboardWorkstationRequestFixtures.noResponse,
      dispatch_id: "dispatch-review-active",
      request_id: "request-active-story",
      request_view: {
        ...dashboardWorkstationRequestFixtures.noResponse.request_view,
        request_time: "2026-04-08T12:00:06Z",
        started_at: "2026-04-08T12:00:06Z",
      },
      started_at: "2026-04-08T12:00:06Z",
    },
    "2026-04-08T12:00:06Z",
  );
  const errored = workstationRequestWithStartedAt(
    dashboardWorkstationRequestFixtures.errored,
    "2026-04-08T12:00:05Z",
  );
  const rejected = workstationRequestWithStartedAt(
    dashboardWorkstationRequestFixtures.rejected,
    "2026-04-08T12:00:03Z",
  );
  const ready = workstationRequestWithStartedAt(
    dashboardWorkstationRequestFixtures.ready,
    "2026-04-08T12:00:02Z",
  );
  const scriptSuccess = workstationRequestWithStartedAt(
    dashboardWorkstationRequestFixtures.scriptSuccess,
    "2026-04-08T12:00:01Z",
  );
  const scriptFailed = workstationRequestWithStartedAt(
    dashboardWorkstationRequestFixtures.scriptFailed,
    "2026-04-08T12:00:00Z",
  );

  return {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
      workstationRequestsByDispatchID: {
        [active.dispatch_id]: active,
        [errored.dispatch_id]: errored,
        [rejected.dispatch_id]: rejected,
        [ready.dispatch_id]: ready,
        [scriptSuccess.dispatch_id]: scriptSuccess,
        [scriptFailed.dispatch_id]: scriptFailed,
      },
    },
  };
}

function dispatchHistoryCard(container: HTMLElement, dispatchId: string): HTMLElement {
  const dispatchBadge = within(container).getByText(dispatchId);
  const card = dispatchBadge.closest("article");

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchId}`);
  }

  return card;
}

function expectWorkOutcomeSeries(outcomeChart: HTMLElement): void {
  expect(outcomeChart.querySelector('[data-chart-series="queued"]')).not.toBeNull();
  expect(outcomeChart.querySelector('[data-chart-series="inFlight"]')).not.toBeNull();
  expect(outcomeChart.querySelector('[data-chart-series="completed"]')).not.toBeNull();
  expect(outcomeChart.querySelector('[data-chart-series="failed"]')).not.toBeNull();
}

export default {
  title: "Agent Factory/Workflow Dashboard",
  component: App,
};

export const SemanticGraphComposition = {
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

    await expectGraphWorkstation(canvasElement, "Select Review workstation");
    expect((await canvas.findAllByText("dispatch-review-active")).length).toBeGreaterThan(0);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Select Implement workstation" }),
    );
    const runHistorySection = within(currentSelectionCard(canvasElement))
      .getByRole("heading", { name: "Run history" })
      .closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected implement workstation run history section",
    );
    await userEvent.click(within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }));
    await expect(within(resolvedRunHistorySection).getByText("Retry Story")).toBeVisible();
    await expect(await canvas.findByText("Failed Story")).toBeVisible();
  },
};

export const SingleNodeGraph = {
  parameters: {
    dashboardApi: {
      snapshot: singleNodeDashboardSnapshot,
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectGraphWorkstation(canvasElement, "Select Intake workstation");
  },
};

export const MediumWorkflowGraph = {
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

    await expectGraphWorkstation(canvasElement, "Select Implement workstation");
    await expect(
      await canvas.findByRole("button", { name: "Select Document workstation" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Select Review workstation" }),
    ).toBeVisible();
  },
};

export const TwentyNodeWorkflowGraph = {
  parameters: {
    dashboardApi: {
      snapshot: twentyNodeDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const viewport = await canvas.findByRole("region", { name: "Work graph viewport" });
    const station20 = await expectGraphWorkstation(
      canvasElement,
      "Select Station 20 workstation",
    );

    viewport.scrollLeft = 320;
    viewport.scrollTop = 80;
    await userEvent.pointer([
      {
        keys: "[MouseLeft>]",
        target: viewport,
        coords: { x: 640, y: 280 },
      },
      {
        target: viewport,
        coords: { x: 360, y: 210 },
      },
      {
        keys: "[/MouseLeft]",
        target: viewport,
        coords: { x: 360, y: 210 },
      },
    ]);

    station20.scrollIntoView({ block: "center", inline: "center" });
    const stationRect = station20.getBoundingClientRect();
    const stationCenterX = stationRect.left + stationRect.width / 2;
    const stationCenterY = stationRect.top + stationRect.height / 2;
    const hitTarget = document.elementFromPoint(stationCenterX, stationCenterY);
    expect(station20.contains(hitTarget)).toBe(true);

    await userEvent.click(station20);
    await expect(station20).toHaveAttribute("aria-pressed", "true");
    await expect(canvas.getByRole("article", { name: "Current selection" })).toBeVisible();
  },
};

export const DashboardImprovementsSmoke = {
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

    const graphCard = await canvas.findByRole("article", { name: "Factory graph" });
    const submitWorkCard = await canvas.findByRole("article", { name: "Submit work" });
    await expect(graphCard).toBeVisible();
    await expect(submitWorkCard).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("combobox", { name: "Work type" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request name" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request text" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
    await expect(
      await canvas.findByRole("button", { name: "Move Work totals" }),
    ).toBeVisible();
    expect(canvas.queryByRole("button", { name: "Move" })).toBeNull();

    const workTotalsItem = canvasElement.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    expect(workTotalsItem?.querySelector(".react-resizable-handle-e")).not.toBeNull();
    expect(workTotalsItem?.querySelector(".react-resizable-handle-s")).not.toBeNull();
    expect(workTotalsItem?.querySelector(".react-resizable-handle-se")).not.toBeNull();

    await userEvent.click(
      await canvas.findByRole("button", { name: "Select Implement workstation" }),
    );
    await expect(within(currentSelectionCard(canvasElement)).getByText("Implement")).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(await canvas.findByRole("button", { name: /Active Story/ }));
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("work-active-story"),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: "Select story:implemented state" }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Current work"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Active Story"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("work-active-story"),
    ).toBeVisible();
    await userEvent.click(
      await canvas.findByRole("button", { name: "Select story:blocked state" }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Current work"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    expect(canvas.queryByRole("article", { name: /Retry|Rework/i })).toBeNull();
    expect(canvas.queryByRole("article", { name: /Timing/i })).toBeNull();

    const outcomeChart = await canvas.findByRole("article", { name: "Work outcome chart" });
    await expect(outcomeChart).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);
    await expect(
      within(outcomeChart).getByRole("img", { name: /Work outcome chart/ }),
    ).toBeVisible();
  },
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
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const frame = canvasElement.firstElementChild;

    await expect(await canvas.findByRole("article", { name: "Submit work" })).toBeVisible();
    await userEvent.click((await canvas.findAllByRole("button", { name: /Active Story/ }))[0]);

    const dashboardGrid = await canvas.findByRole("region", {
      name: "Agent Factory bento board",
    });
    const dashboardScope = within(dashboardGrid);

    await expect(dashboardScope.getByRole("article", { name: "Submit work" })).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Current selection" }),
    ).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Trace drill-down" }),
    ).toBeVisible();
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(360);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const DashboardSubmitWorkIntegrationSmoke = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/work",
          response: {
            body: {
              trace_id: "trace-submit-story",
            },
            status: 201,
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <App />,
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const { requestField, requestNameField, scope, submitButton, workTypeField } =
      await submitWorkCardControls(canvasElement);

    expect(
      Array.from((workTypeField as HTMLSelectElement).options, (option) => option.value),
    ).toContain("story");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(requestNameField, "Dashboard smoke request");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(requestField, "Review the failed dashboard submission smoke.");
    await expect(submitButton).toBeDisabled();
    await userEvent.selectOptions(workTypeField, "story");
    await expect(submitButton).toBeEnabled();
    await userEvent.click(submitButton);
    await expect(
      await scope.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeVisible();
    await expect(requestNameField).toHaveValue("");
    await expect(requestField).toHaveValue("");
    await expect(submitButton).toBeDisabled();
  },
};

export const DashboardSubmitWorkRetryableFailure = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/work",
          response: {
            body: {
              code: "BAD_REQUEST",
              message: "work_type_name is required",
            },
            status: 400,
            statusText: "Bad Request",
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <App />,
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const requestText = "Retry the broken submission from the dashboard shell.";
    const requestName = "Retry dashboard request";
    const { requestField, requestNameField, scope, workTypeField } = await fillSubmitWorkCard(
      canvasElement,
      requestName,
      requestText,
    );

    await userEvent.click(scope.getByRole("button", { name: "Submit work" }));
    await expect(await scope.findByText("work_type_name is required")).toBeVisible();
    await expect(workTypeField).toHaveValue("story");
    await expect(requestNameField).toHaveValue(requestName);
    await expect(requestField).toHaveValue(requestText);
  },
};

export const WorkstationRequestSelection = {
  parameters: workstationRequestStoryParameters(markdownReadyWorkstationRequest),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, markdownReadyWorkstationRequest);

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getByRole("heading", { name: "Request counts" }),
    ).toBeVisible();
    await expect(
      currentSelection.getByRole("heading", { name: "Response details" }),
    ).toBeVisible();
    await expect(
      currentSelection.getAllByText(markdownReadyWorkstationRequest.dispatch_id).length,
    ).toBeGreaterThan(0);
    await expect(currentSelection.getAllByText("request-ready-story").length).toBeGreaterThan(0);
    expect(currentSelection.queryByRole("heading", { name: "Active work" })).toBeNull();
    expect(currentSelection.queryByRole("heading", { name: "Execution details" })).toBeNull();
    expect(currentSelection.queryByRole("heading", { name: "Workstation dispatches" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionNoResponse = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.noResponse),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.noResponse);

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getByRole("heading", { name: "Request counts" }),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Response text is not available for this workstation request yet.",
      ),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Response metadata is not available for this workstation request yet.",
      ),
    ).toBeVisible();
    await expect(currentSelection.getByRole("heading", { name: "Response details" })).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Execution details" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionRejected = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.rejected),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.rejected);

    const currentSelection = within(currentSelectionCard(canvasElement));
    const responseDetails = within(currentSelection.getByRole("region", { name: "Response details" }));

    expect(currentSelection.getAllByText("request-rejected-story").length).toBeGreaterThan(0);
    await expect(
      currentSelection.getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeVisible();
    await expect(
      responseDetails.getByText("The active story needs revision before it can continue."),
    ).toBeVisible();
    await expect(currentSelection.getByRole("heading", { name: "Response details" })).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Active work" })).toBeNull();
    expect(currentSelection.queryByRole("heading", { name: "Execution details" })).toBeNull();
    expect(currentSelection.queryByRole("heading", { name: "Error details" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionErrored = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.errored),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.errored);

    const currentSelection = within(currentSelectionCard(canvasElement));
    const errorDetails = within(currentSelection.getByRole("region", { name: "Error details" }));

    expect(currentSelection.getAllByText("request-error-story").length).toBeGreaterThan(0);
    await expect(
      currentSelection.getByText("Review the blocked story and explain the failure."),
    ).toBeVisible();
    await expect(currentSelection.getByRole("heading", { name: "Inference attempts" })).toBeVisible();
    expect(currentSelection.getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    await expect(
      errorDetails.getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Response text is unavailable because this workstation request ended with an error.",
      ),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Response metadata is unavailable because this workstation request ended with an error.",
      ),
    ).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Active work" })).toBeNull();
    expect(currentSelection.queryByRole("heading", { name: "Execution details" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const SelectedWorkDispatchHistorySmoke = {
  parameters: selectedWorkDispatchHistoryStoryParameters(),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(await canvas.findByRole("button", { name: "Select Review workstation" }));
    await userEvent.click(
      within(currentSelectionCard(canvasElement)).getByRole("button", {
        name: "Select work item Active Story",
      }),
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const dispatchHistory = currentSelection.getByRole("region", {
      name: "Workstation dispatches",
    });

    await expect(currentSelection.getByRole("heading", { name: "Workstation dispatches" })).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Work session runs list" })).toBeNull();
    await expect(within(dispatchHistory).getByText("6 dispatches")).toBeVisible();
    [
      "dispatch-review-active",
      dashboardWorkstationRequestFixtures.errored.dispatch_id,
      dashboardWorkstationRequestFixtures.rejected.dispatch_id,
      dashboardWorkstationRequestFixtures.ready.dispatch_id,
      dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id,
      dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id,
    ].forEach((dispatchId) => {
      expect(dispatchHistoryCard(dispatchHistory, dispatchId)).toBeTruthy();
    });

    const activeCard = dispatchHistoryCard(dispatchHistory, "dispatch-review-active");
    await expect(within(activeCard).getByText("Current dispatch")).toBeVisible();
    await expect(
      within(activeCard).getByText("No response yet for this dispatch."),
    ).toBeVisible();
    await expect(
      within(activeCard).getByRole("button", { name: "Select work item Active Story" }),
    ).toBeVisible();

    const erroredCard = dispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.errored.dispatch_id,
    );
    await expect(
      within(erroredCard).getByText("Provider rate limit exceeded while reviewing the story."),
    ).toBeVisible();
    expect(within(erroredCard).queryByText("Current dispatch")).toBeNull();

    const traceLink = within(erroredCard).getByRole("link", {
      name: /^trace-active-story/,
    });
    await expect(traceLink).toBeVisible();
    expect(traceLink.getAttribute("href")).toBe("#trace");

    const scriptSuccessCard = dispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id,
    );
    await expect(within(scriptSuccessCard).getByText("script-tool")).toBeVisible();
    await expect(within(scriptSuccessCard).getByText("script success stdout")).toBeVisible();
    expect(within(scriptSuccessCard).queryByText("Current dispatch")).toBeNull();

    const scriptFailedCard = dispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id,
    );
    expect(within(scriptFailedCard).getAllByText("TIMEOUT").length).toBeGreaterThan(0);
    await expect(within(scriptFailedCard).getByText("script timed out")).toBeVisible();
    expect(within(scriptFailedCard).queryByText("Current dispatch")).toBeNull();

    await expect(canvas.getByRole("article", { name: "Trace drill-down" })).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptPending = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.scriptPending),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.scriptPending);

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getAllByText("request-script-pending-story").length).toBeGreaterThan(0);
    await expect(currentSelection.getByText("script-tool")).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Script response details are not available for this workstation request yet.",
      ),
    ).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptSuccess = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.scriptSuccess),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.scriptSuccess);

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getAllByText("request-script-success-story").length).toBeGreaterThan(0);
    await expect(currentSelection.getByText("script success stdout")).toBeVisible();
    await expect(currentSelection.getAllByText("SUCCEEDED").length).toBeGreaterThan(0);
    await expect(currentSelection.getAllByText("222ms").length).toBeGreaterThan(0);
    expect(currentSelection.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptFailed = {
  parameters: workstationRequestStoryParameters(dashboardWorkstationRequestFixtures.scriptFailed),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(canvasElement, dashboardWorkstationRequestFixtures.scriptFailed);

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getAllByText("request-script-failed-story").length).toBeGreaterThan(0);
    await expect(currentSelection.getByText("script_timeout")).toBeVisible();
    await expect(currentSelection.getByText("TIMEOUT")).toBeVisible();
    await expect(currentSelection.getByText("script timed out")).toBeVisible();
    expect(currentSelection.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkChartTimelineVerification = {
  parameters: {
    dashboardApi: {
      timelineSnapshots: [historicalWorkOutcomeSnapshot, liveWorkOutcomeSnapshot],
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
      within(outcomeChart).getByRole("img", { name: "Work outcome chart for Session" }),
    ).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);

    const slider = await canvas.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    fireEvent.change(slider, { target: { value: "2" } });

    await expect(await canvas.findByText("Tick 2 of 5")).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);

    await userEvent.click(await canvas.findByRole("button", { name: "Current" }));

    await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);
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
      await canvas.findByRole("button", { name: "Select story:implemented state" }),
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
      currentSelection.getByText("No work is recorded for this place at the selected tick."),
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

    await userEvent.click((await canvas.findAllByRole("button", { name: /Active Story/ }))[0]);

    const currentSelection = within(currentSelectionCard(canvasElement));
    expect(currentSelection.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    await expect(currentSelection.getByRole("heading", { name: "Workstation dispatches" }))
      .toBeVisible();
    await expect(currentSelection.getByText("Current dispatch")).toBeVisible();
    expect(currentSelection.getAllByText(/codex/).length).toBeGreaterThan(0);
    expect(currentSelection.getAllByText(/factory-renderer/).length).toBeGreaterThan(0);
    expect(currentSelection.getAllByText(/sha256:system-runtime/).length).toBeGreaterThan(0);
    expect(
      currentSelection.queryByText(/Model details are not available for this selected run/),
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

    await userEvent.click(await canvas.findByRole("button", { name: "Failed Story" }));

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(currentSelection.getByText("Failed Story")).toBeVisible();
    await expect(currentSelection.getByText("Failure reason")).toBeVisible();
    await expect(currentSelection.getByText("provider_rate_limit")).toBeVisible();
    await expect(currentSelection.getByText("Failure message")).toBeVisible();
    await expect(
      currentSelection.getByText("Provider rate limit exceeded while generating the repair."),
    ).toBeVisible();
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

    const slider = await canvas.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    expect(slider.value).toBe("4");
    await expect(await canvas.findByText("Tick 4 of 4")).toBeVisible();

    await userEvent.click(await canvas.findByRole("button", { name: "Blocked Analysis Story" }));

    const failedSelection = within(currentSelectionCard(canvasElement));
    await expect(failedSelection.getByText("Failure reason")).toBeVisible();
    expect(failedSelection.getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    await expect(failedSelection.getByText("Failure message")).toBeVisible();
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

    await userEvent.click(await canvas.findByRole("button", { name: "Select story:new state" }));

    const positionSelection = within(currentSelectionCard(canvasElement));
    await expect(positionSelection.getByText("Current work")).toBeVisible();
    await expect(positionSelection.getByText("Queued Analysis Story")).toBeVisible();
    await expect(positionSelection.getByText("work-queued-analysis")).toBeVisible();
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

    const slider = await canvas.findByRole<HTMLInputElement>("slider", { name: "Timeline tick" });
    await expect(await canvas.findByText("Tick 4 of 4")).toBeVisible();
    await expect(await canvas.findByLabelText("2 resource tokens")).toBeVisible();

    fireEvent.change(slider, { target: { value: "3" } });

    await expect(await canvas.findByText("Tick 3 of 4")).toBeVisible();
    await expect(await canvas.findByLabelText("1 resource tokens")).toBeVisible();

    fireEvent.change(slider, { target: { value: "1" } });

    await expect(await canvas.findByText("Tick 1 of 4")).toBeVisible();
    await expect(await canvas.findByLabelText("2 resource tokens")).toBeVisible();
  },
};

export const TypographyRegression = {
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
