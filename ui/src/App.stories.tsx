// biome-ignore-all lint/nursery/noExcessiveLinesPerFile: dashboard story coverage is intentionally consolidated until the follow-up story split lands.
import { expect, userEvent, waitFor, within } from "storybook/test";

import { App } from "./App";
import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "./api/dashboard";
import type { FactoryValue } from "./api/named-factory";
import { dashboardWorkstationRequestFixtures } from "./components/dashboard/fixtures";
import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";
import { formatTimeOfDay } from "./components/ui/formatters";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./components/ui/dashboard-typography";
import { DashboardScreen } from "./features/dashboard";
import { AppLocaleProvider, useAppLocale } from "./i18n";
import {
  buttonVisibleStyle,
  expectGraphWorkstation,
  fillSubmitWorkCard,
} from "./stories/dashboardStoryTestUtils";

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
const _inferenceDetailsSnapshot = withInferenceDetails(
  semanticWorkflowDashboardSnapshot,
);
const _markdownReadyWorkstationRequest: DashboardWorkstationRequest = {
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
const editableConfigurationFactoryDefinition =
  buildEditableConfigurationFactoryDefinition();
const editableConfigurationDocument = buildEditableConfigurationDocument();
const promptTemplateContractResponse = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
    {
      category: "INPUT",
      description: "Payload for the first authored input.",
      example: "{{ (index .Inputs 0).Payload }}",
      path: ".Inputs[0].Payload",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [
    {
      example: "{{ (index .Inputs 1).Payload }}",
      path: ".Inputs[1].Payload",
      reason: "Only input 0 is available for this workstation.",
    },
  ],
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
        provider_sessions: (source.runtime.session.provider_sessions ?? []).map(
          (attempt) =>
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

const _failedStoryTrace: DashboardTrace = {
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
      failure_message:
        "Provider rate limit exceeded while generating the repair.",
      failure_reason: "provider_rate_limit",
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [],
      output_mutations: [],
    },
  ],
};

function expectCurrentSelectionCardID(canvasElement: HTMLElement): void {
  const canvas = within(canvasElement);
  const currentSelection = canvas.getByRole("article", {
    name: "Current selection",
  });
  expect(
    currentSelection.closest<HTMLElement>("[data-bento-card-id]")?.dataset
      .bentoCardId,
  ).toBe("current-selection");
}

function currentSelectionCard(canvasElement: HTMLElement): HTMLElement {
  return within(canvasElement).getByRole("article", {
    name: "Current selection",
  });
}

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function buildEditableConfigurationFactoryDefinition(
  overrides: { prompt?: string; workerName?: string } = {},
): FactoryValue {
  return {
    name: "Current Factory",
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [],
    workstations: [
      {
        body:
          overrides.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides.workerName ?? "reviewer",
      },
    ],
  };
}

function buildEditableConfigurationDocument(
  factoryDefinition: FactoryValue = editableConfigurationFactoryDefinition,
) {
  return {
    factoryDefinition,
    version: {
      logical: 7,
      physical: "2026-05-20T10:00:00Z",
    },
  };
}

function submittedFactoryDefinitionBody(init?: RequestInit): FactoryValue {
  if (typeof init?.body !== "string") {
    return buildEditableConfigurationFactoryDefinition({
      prompt: "Browser verified prompt update.",
      workerName: "planner",
    });
  }

  const requestBody = JSON.parse(init.body) as {
    factoryDefinition?: FactoryValue;
  };

  return (
    requestBody.factoryDefinition ??
    buildEditableConfigurationFactoryDefinition({
      prompt: "Browser verified prompt update.",
      workerName: "planner",
    })
  );
}

function promptTemplateValidationResponse(init?: RequestInit) {
  if (typeof init?.body !== "string") {
    return {
      diagnostics: [],
      valid: true,
    };
  }

  const payload = JSON.parse(init.body) as { prompt?: string };
  if (payload.prompt === "Use {{ .WorkID }}.") {
    return {
      diagnostics: [],
      valid: true,
    };
  }

  return {
    diagnostics: [
      {
        endOffset: 33,
        kind: "UNAVAILABLE_VARIABLE",
        message: "Only input 0 is available.",
        path: ".Inputs[1]",
        sourceText: "(index .Inputs 1)",
        startOffset: 7,
      },
    ],
    valid: false,
  };
}

function validPromptTemplateValidationResponse() {
  return {
    diagnostics: [],
    valid: true,
  };
}

function editableConfigurationSection(
  currentSelection: HTMLElement,
): HTMLElement {
  const section = within(currentSelection)
    .getByRole("heading", { name: "Configuration" })
    .closest("section");

  if (!(section instanceof HTMLElement)) {
    throw new Error("expected editable configuration section");
  }

  return section;
}

async function expectEditableConfigurationBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);

  await userEvent.click(
    await canvas.findByRole("button", { name: "Select Review workstation" }),
  );

  const currentSelection = currentSelectionCard(canvasElement);
  const section = editableConfigurationSection(currentSelection);
  const sectionScope = within(section);
  const expandButton = sectionScope.getByRole("button", {
    name: "Expand editable configuration",
  });

  await expect(expandButton).toHaveAttribute("aria-expanded", "false");
  expect(sectionScope.queryByLabelText("Worker")).toBeNull();
  expect(sectionScope.queryByLabelText("Prompt")).toBeNull();

  await userEvent.click(expandButton);

  const workerField = await sectionScope.findByRole("combobox", {
    name: "Worker",
  });
  const promptField = await sectionScope.findByRole("textbox", {
    name: "Prompt",
  });

  await expect(expandButton).toHaveAttribute("aria-expanded", "true");
  await expect(workerField).toHaveValue("reviewer");
  await expect(promptField).toHaveValue(
    "Review the latest story changes before approval.",
  );
  expect(sectionScope.queryByLabelText("Model")).toBeNull();
  expect(sectionScope.queryByLabelText("Template")).toBeNull();

  await userEvent.selectOptions(workerField, "planner");
  await userEvent.clear(promptField);
  await userEvent.type(promptField, "Browser verified prompt update.");

  await expect(workerField).toHaveValue("planner");
  await expect(promptField).toHaveValue("Browser verified prompt update.");

  const currentSelectionScope = within(currentSelection);
  await userEvent.click(
    currentSelectionScope.getByRole("button", { name: "Save changes" }),
  );

  await expect(
    await canvas.findByRole("heading", {
      name: "Overwrite the running factory definition?",
    }),
  ).toBeVisible();
  await userEvent.click(
    canvas.getByRole("button", { name: "Overwrite factory" }),
  );

  await expect(
    await sectionScope.findByText(
      "Running factory saved. The editable workstation values were refreshed to the saved definition.",
    ),
  ).toBeVisible();
  await expect(workerField).toHaveValue("planner");
  await expect(promptField).toHaveValue("Browser verified prompt update.");
  await expect(
    currentSelectionScope.getByRole("button", { name: "Save changes" }),
  ).toBeDisabled();
}

async function expectFactoryGraphHeaderBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const graphCard = await canvas.findByRole("article", {
    name: "Factory graph",
  });
  const graphHeader = graphCard.querySelector("header");

  if (!(graphHeader instanceof HTMLElement)) {
    throw new Error("expected factory graph card header");
  }

  const headerScope = within(graphHeader);
  await expect(headerScope.getByText("Observe mode")).toBeVisible();
  await userEvent.click(
    headerScope.getByRole("button", {
      name: "Enter factory graph editor",
    }),
  );
  await expect(headerScope.getByText("Editor mode active")).toBeVisible();
  await expect(
    headerScope.getByRole("button", {
      name: "Leave factory graph editor",
    }),
  ).toBeVisible();
  await expect(
    within(graphCard).getByRole("region", {
      name: "Factory graph editor tools",
    }),
  ).toBeVisible();
  await userEvent.click(
    headerScope.getByRole("button", {
      name: "Leave factory graph editor",
    }),
  );
  await expect(headerScope.getByText("Observe mode")).toBeVisible();
}

async function expectPromptHintBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);

  await userEvent.click(
    await canvas.findByRole("button", { name: "Select Review workstation" }),
  );

  const currentSelection = currentSelectionCard(canvasElement);
  const section = editableConfigurationSection(currentSelection);
  const sectionScope = within(section);

  await userEvent.click(
    sectionScope.getByRole("button", {
      name: "Expand editable configuration",
    }),
  );

  const promptField = await sectionScope.findByRole("textbox", {
    name: "Prompt",
  });
  const helpButton = sectionScope.getByRole("button", {
    name: "Open prompt variable help",
  });
  const saveButton = within(currentSelection).getByRole("button", {
    name: "Save changes",
  });

  helpButton.focus();
  await userEvent.keyboard("{Enter}");
  await expect(
    await sectionScope.findByText("This workstation exposes 1 authored input."),
  ).toBeVisible();
  await expect(sectionScope.getByText(".WorkID")).toBeVisible();
  await expect(sectionScope.getByText("{{ .WorkID }}")).toBeVisible();
  await expect(sectionScope.getByText(".Inputs[1].Payload")).toBeVisible();

  promptField.focus();
  await userEvent.clear(promptField);
  await userEvent.type(promptField, "Use {{ (index .Inputs 1).Payload }}.");

  await expect(
    await sectionScope.findByText("Prompt diagnostics"),
  ).toBeVisible();
  await expect(sectionScope.getByText(".Inputs[1]")).toBeVisible();
  await expect(sectionScope.getAllByText("(index .Inputs 1)")[0]).toBeVisible();
  await expect(saveButton).toBeDisabled();

  const diagnosticUnderline = section.querySelector(
    "mark.decoration-wavy",
  ) as HTMLElement | null;
  expect(diagnosticUnderline?.textContent).toContain("(index .Inputs 1)");
}

function expectNoPageHorizontalOverflow(canvasElement: HTMLElement): void {
  const documentElement = canvasElement.ownerDocument.documentElement;
  const overflowTolerance = 1;

  expect(
    documentElement.scrollWidth <=
      documentElement.clientWidth + overflowTolerance,
  ).toBe(true);
}

async function submitWorkCardControls(canvasElement: HTMLElement): Promise<{
  requestNameField: HTMLElement;
  requestField: HTMLElement;
  scope: ReturnType<typeof within>;
  submitButton: HTMLElement;
  workTypeField: HTMLElement;
}> {
  const canvas = within(canvasElement);
  const submitWorkCard = await canvas.findByRole("article", {
    name: "Submit work",
  });
  const submitWorkScope = within(submitWorkCard);
  const workTypeField = submitWorkScope.getByRole("combobox", {
    name: "Work type",
  });
  const requestNameField = submitWorkScope.getByRole("textbox", {
    name: "Request name",
  });
  const requestField = submitWorkScope.getByRole("textbox", {
    name: "Request",
  });

  return {
    requestNameField,
    requestField,
    scope: submitWorkScope,
    submitButton: submitWorkScope.getByRole("button", { name: "Submit work" }),
    workTypeField,
  };
}

async function _expectTypographyRegressionSurface(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const heading = await canvas.findByRole("heading", { name: "Infinite You" });
  const hiddenWordmark = within(heading).getByText("Infinite You");
  const toolbar = canvas.getByRole("region", { name: "dashboard summary" });
  const streamStatus = canvas.getByRole("status", {
    name: /Infinite You event stream (connecting|live)/,
  });

  expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
  expect(hiddenWordmark.className).toContain("sr-only");
  expect(heading.textContent).toContain("∞");
  expect(heading.textContent).toContain("U");
  expect(streamStatus.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
  expect(streamStatus.className).toContain(DASHBOARD_SUPPORTING_LABELS_CLASS);
  expect(within(toolbar).queryByText("Factory state")).toBeNull();
  expect(
    within(toolbar).queryByText(
      String(semanticWorkflowDashboardSnapshot.factory_state),
    ),
  ).toBeNull();
  expect(within(toolbar).queryByText("Stream")).toBeNull();
  expect(within(toolbar).queryByText("Loading factory events...")).toBeNull();
  expect(within(toolbar).queryByText("Export PNG")).toBeNull();

  await userEvent.click(
    await canvas.findByRole("button", { name: "Select Review workstation" }),
  );

  const currentSelection = currentSelectionCard(canvasElement);
  const currentSelectionScope = within(currentSelection);
  const activeWorkHeading = currentSelectionScope.getByRole("heading", {
    name: "Active work",
  });
  const activeWorkCard = currentSelectionScope
    .getByText("Active Story")
    .closest("li");
  const runHistorySection = currentSelectionScope
    .getByRole("heading", { name: "Run history" })
    .closest("section");

  if (!(runHistorySection instanceof HTMLElement)) {
    throw new Error("expected current-selection run history section");
  }

  expect(activeWorkHeading.className).toContain(
    DASHBOARD_SECTION_HEADING_CLASS,
  );
  expect(activeWorkCard?.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
  expect(within(runHistorySection).getByText("2 runs").className).toContain(
    DASHBOARD_SUPPORTING_TEXT_CLASS,
  );
}

async function _expectTimelineToolbarAlignment(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const toolbar = await canvas.findByRole("region", {
    name: "dashboard summary",
  });
  const heading = within(toolbar).getByRole("heading", {
    name: "Infinite You",
  });
  const slider = within(toolbar).getByRole<HTMLInputElement>("slider", {
    name: "Timeline tick",
  });
  const streamStatus = within(toolbar).getByRole("status", {
    name: /Infinite You event stream (connecting|live)/,
  });
  const exportButton = within(toolbar).getByRole("button", {
    name: "Export PNG",
  });
  const sliderShell = requireValue(
    slider.closest<HTMLElement>("div"),
    "expected slider shell in dashboard toolbar",
  );
  const headingRect = heading.getBoundingClientRect();
  const sliderRect = sliderShell.getBoundingClientRect();
  const streamStatusRect = streamStatus.getBoundingClientRect();
  const exportButtonRect = exportButton.getBoundingClientRect();

  expect(sliderRect.left).toBeGreaterThanOrEqual(headingRect.right - 1);
  expect(streamStatusRect.left).toBeGreaterThanOrEqual(sliderRect.right - 1);
  expect(exportButtonRect.left).toBeGreaterThanOrEqual(
    streamStatusRect.right - 1,
  );
}

async function _selectWorkstationRequest(
  canvasElement: HTMLElement,
  request: DashboardWorkstationRequest,
): Promise<void> {
  await selectWorkstationRequestByDispatchID(
    canvasElement,
    request.dispatch_id,
  );
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
    const collapsedButton = requestHistoryScope.queryByRole("button", {
      name: "Expand",
    });
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
    const collapsedButton = runHistoryScope.queryByRole("button", {
      name: "Expand",
    });
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

  throw new Error(
    `unable to find workstation request controls for ${dispatchID}`,
  );
}

function _workstationRequestStoryParameters(
  request: DashboardWorkstationRequest,
) {
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

function _selectedWorkDispatchHistoryStoryParameters() {
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

function _dispatchHistoryCard(
  container: HTMLElement,
  dispatchId: string,
): HTMLElement {
  const dispatchBadge = within(container).getAllByText(dispatchId)[0];
  const card = dispatchBadge.closest("article");

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchId}`);
  }

  return card;
}

function expectWorkOutcomeSeries(outcomeChart: HTMLElement): void {
  expect(
    outcomeChart.querySelector('[data-chart-series="queued"]'),
  ).not.toBeNull();
  expect(
    outcomeChart.querySelector('[data-chart-series="inFlight"]'),
  ).not.toBeNull();
  expect(
    outcomeChart.querySelector('[data-chart-series="completed"]'),
  ).not.toBeNull();
  expect(
    outcomeChart.querySelector('[data-chart-series="failed"]'),
  ).not.toBeNull();
}

function LocalePropagationStory() {
  return (
    <AppLocaleProvider initialLocale="en">
      <LocalePropagationControls />
      <div style={{ maxWidth: "100%", width: "1280px" }}>
        <DashboardScreen />
      </div>
    </AppLocaleProvider>
  );
}

function LocalePropagationControls() {
  const { locale, setLocale } = useAppLocale();

  return (
    <fieldset style={{ display: "flex", gap: "0.75rem", marginBottom: "1rem" }}>
      <legend>Locale verification controls</legend>
      <span>Current locale: {locale}</span>
      <button onClick={() => setLocale("en")} type="button">
        Switch to English
      </button>
      <button onClick={() => setLocale("zh-CN")} type="button">
        Switch to zh-CN
      </button>
    </fieldset>
  );
}

export default {
  title: "Infinite You/Workflow Dashboard",
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
    const graphCard = await canvas.findByRole("article", {
      name: "Factory graph",
    });

    await expectGraphWorkstation(canvasElement, "Select Review workstation");
    expect(canvas.queryByText("Operator View")).toBeNull();
    expect(
      within(graphCard).queryByRole("heading", { name: "Current activity" }),
    ).toBeNull();
    expect(
      (await canvas.findAllByText("dispatch-review-active")).length,
    ).toBeGreaterThan(0);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    const runHistorySection = within(currentSelectionCard(canvasElement))
      .getByRole("heading", { name: "Run history" })
      .closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected implement workstation run history section",
    );
    await userEvent.click(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }),
    );
    await expect(
      within(resolvedRunHistorySection).getByText("Retry Story"),
    ).toBeVisible();
    const failedStoryButton = await canvas.findByRole("button", {
      name: "Failed Story",
    });
    await expect(failedStoryButton).toBeVisible();
    await expect(within(failedStoryButton).getByText("Failed at Repair")).toBeVisible();
    expect(within(failedStoryButton).queryByText(/session_id/i)).toBeNull();
    expect(within(failedStoryButton).queryByText(/codex/i)).toBeNull();
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
      await canvas.findByRole("button", {
        name: "Select Document workstation",
      }),
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
    const viewport = await canvas.findByRole("region", {
      name: "Work graph viewport",
    });
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
    await expect(
      canvas.getByRole("article", { name: "Current selection" }),
    ).toBeVisible();
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

    const graphCard = await canvas.findByRole("article", {
      name: "Factory graph",
    });
    const submitWorkCard = await canvas.findByRole("article", {
      name: "Submit work",
    });
    await expect(graphCard).toBeVisible();
    await expect(submitWorkCard).toBeVisible();
    expect(within(graphCard).queryByText("Operator View")).toBeNull();
    expect(
      within(graphCard).queryByRole("heading", { name: "Current activity" }),
    ).toBeNull();
    await expect(
      within(submitWorkCard).getByRole("combobox", { name: "Work type" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request name" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
    expect(
      within(submitWorkCard).queryByText(
        "Ready to submit. Request details are optional.",
      ),
    ).toBeNull();
    expect(
      within(submitWorkCard).queryByText(
        "Optional. Leave this blank to submit an empty request.",
      ),
    ).toBeNull();
    await expect(
      await canvas.findByRole("button", { name: "Move Work totals" }),
    ).toBeVisible();
    expect(canvas.queryByRole("button", { name: "Move" })).toBeNull();

    const workTotalsItem = canvasElement.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-e"),
    ).not.toBeNull();
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-s"),
    ).not.toBeNull();
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-se"),
    ).not.toBeNull();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Implement"),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: /Active Story/ }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText(
        "work-active-story",
      ),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select story:implemented state",
      }),
    );
    const currentSelection = within(currentSelectionCard(canvasElement));
    const summaryDetails = currentSelection.getByText("Count").closest("dl");
    await expect(
      currentSelection.getByText("Current work"),
    ).toBeVisible();
    await expect(currentSelection.getByText("story: implemented")).toBeVisible();
    await expect(currentSelection.getByText("Active Story")).toBeVisible();
    await expect(
      currentSelection.getByText(
        `Started at ${formatTimeOfDay("2026-04-08T12:00:01Z")}`,
      ),
    ).toBeVisible();
    expect(summaryDetails).not.toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("Work type")).toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("State")).toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("State node ID")).toBeNull();
    expect(currentSelection.queryByText("work-active-story")).toBeNull();
    expect(currentSelection.queryByText("trace-active-story")).toBeNull();
    const traceDrilldownCard = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(
      within(traceDrilldownCard).queryByText(
        "Resolves from selected-tick factory event history.",
      ),
    ).toBeNull();
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

    const outcomeChart = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });
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

    await expect(
      await canvas.findByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await userEvent.click(
      (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = await canvas.findByRole("region", {
      name: "Infinite You bento board",
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
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(360);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const CurrentSelectionEditableConfigurationDesktopVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: {
            body: validPromptTemplateValidationResponse(),
            status: 200,
          },
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryDefinitionBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectFactoryGraphHeaderBrowserFlow(canvasElement);
    await expectEditableConfigurationBrowserFlow(canvasElement);
  },
};

export const CurrentSelectionEditableConfigurationNarrowVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: {
            body: validPromptTemplateValidationResponse(),
            status: 200,
          },
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryDefinitionBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectFactoryGraphHeaderBrowserFlow(canvasElement);
    await expectEditableConfigurationBrowserFlow(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const CurrentSelectionPromptHintVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "GET",
          path: "/factory/~current/workstations/Review/prompt-template-contract",
          response: {
            body: promptTemplateContractResponse,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
            status: 200,
          }),
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectPromptHintBrowserFlow(canvasElement);
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
              traceId: "trace-submit-story",
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
    const {
      requestField,
      requestNameField,
      scope,
      submitButton,
      workTypeField,
    } = await submitWorkCardControls(canvasElement);
    const disabledSubmitStyle = buttonVisibleStyle(submitButton);

    expect(
      Array.from(
        (workTypeField as HTMLSelectElement).options,
        (option) => option.value,
      ),
    ).toContain("story");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(requestNameField, "Dashboard smoke request");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(
      requestField,
      "Review the failed dashboard submission smoke.",
    );
    await expect(submitButton).toBeDisabled();
    await userEvent.selectOptions(workTypeField, "story");
    await expect(submitButton).toBeEnabled();
    await waitFor(() => {
      expect(buttonVisibleStyle(submitButton)).not.toEqual(disabledSubmitStyle);
    });
    await userEvent.click(submitButton);
    await expect(
      await scope.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeVisible();
    await expect(requestNameField).toHaveValue("");
    await expect(requestField).toHaveValue("");
    await expect(workTypeField).toHaveValue("story");
    await expect(submitButton).toBeDisabled();
    await waitFor(() => {
      expect(buttonVisibleStyle(submitButton)).toEqual(disabledSubmitStyle);
    });
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
    const { requestField, requestNameField, scope, workTypeField } =
      await fillSubmitWorkCard(canvasElement, requestName, requestText);

    await userEvent.click(scope.getByRole("button", { name: "Submit work" }));
    await expect(
      await scope.findByText("work_type_name is required"),
    ).toBeVisible();
    await expect(workTypeField).toHaveValue("story");
    await expect(requestNameField).toHaveValue(requestName);
    await expect(requestField).toHaveValue(requestText);
  },
};

export const HeaderLocalizationVerification = {
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
      <App initialLocale="en" />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const englishToolbar = await canvas.findByRole("region", {
      name: "dashboard summary",
    });
    const languageButton = within(englishToolbar).getByRole("button", {
      name: "Change language",
    });

    await expect(
      within(englishToolbar).getByRole("heading", { name: "Infinite You" }),
    ).toBeVisible();
    await expect(
      within(englishToolbar).getByRole("button", { name: "Export PNG" }),
    ).toBeVisible();
    await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();

    await userEvent.tab();
    await expect(languageButton).toHaveFocus();
    await userEvent.click(languageButton);
    await userEvent.click(
      within(canvasElement.ownerDocument.body).getByRole("menuitemradio", {
        name: "简体中文",
      }),
    );

    const localizedToolbar = await canvas.findByRole("region", {
      name: "仪表板概览",
    });
    await expect(
      within(localizedToolbar).getByRole("button", { name: "切换语言" }),
    ).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("slider", { name: "时间线刻度" }),
    ).toBeVisible();
    await expect(await canvas.findByText("第 5 个刻度，共 5 个")).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("status", {
        name: /Infinite You 事件流(正在连接|在线)/,
      }),
    ).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("button", { name: "返回当前刻度" }),
    ).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    ).toBeVisible();

    await userEvent.click(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    );
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      { name: "导出工厂" },
    );
    await expect(
      within(dialog).getByRole("button", { name: "取消" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("button", { name: "导出 PNG" }),
    ).toBeVisible();
    await userEvent.click(within(dialog).getByRole("button", { name: "取消" }));

    const localizedLanguageButton = within(localizedToolbar).getByRole(
      "button",
      {
        name: "切换语言",
      },
    );
    localizedLanguageButton.focus();
    await expect(localizedLanguageButton).toHaveFocus();
    await userEvent.click(localizedLanguageButton);
    await userEvent.click(
      within(canvasElement.ownerDocument.body).getByRole("menuitemradio", {
        name: "English",
      }),
    );

    const restoredToolbar = await canvas.findByRole("region", {
      name: "dashboard summary",
    });
    await expect(
      within(restoredToolbar).getByRole("button", { name: "Change language" }),
    ).toBeVisible();
    await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();
    await expect(
      within(restoredToolbar).getByRole("button", { name: "Export PNG" }),
    ).toBeVisible();
  },
};

export const LocalePropagationVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => <LocalePropagationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const controls = await canvas.findByRole("group", {
      name: "Locale verification controls",
    });
    const englishToolbar = await canvas.findByRole("region", {
      name: "dashboard summary",
    });

    await expect(
      within(controls).getByText("Current locale: en"),
    ).toBeVisible();
    await expect(
      within(englishToolbar).getByRole("button", {
        name: "Return to current tick",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("Tick 5 of 5")).toBeVisible();

    await userEvent.click(
      within(controls).getByRole("button", {
        name: "Switch to zh-CN",
      }),
    );

    const mandarinToolbar = await canvas.findByRole("region", {
      name: "仪表板概览",
    });
    await expect(
      within(controls).getByText("Current locale: zh-CN"),
    ).toBeVisible();
    await expect(
      within(mandarinToolbar).getByRole("button", {
        name: "返回当前刻度",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("第 5 个刻度，共 5 个")).toBeVisible();
  },
};
