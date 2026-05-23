import { expect, userEvent, within } from "storybook/test";

import type { DashboardWorkstationRequest } from "../api/dashboard";
import { dashboardWorkstationRequestFixtures } from "../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../components/dashboard/test-fixtures";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../components/ui/dashboard-typography";
import { activeStoryTrace } from "./dashboardStoryFixtures";

export async function expectGraphWorkstation(
  canvasElement: HTMLElement,
  workstationName: string,
): Promise<HTMLElement> {
  const canvas = within(canvasElement);

  await expect(
    await canvas.findByRole("region", { name: "Work graph viewport" }),
  ).toBeVisible();

  const workstation = await canvas.findByRole("button", {
    name: workstationName,
  });
  await expect(workstation).toBeVisible();

  return workstation;
}

export function expectCurrentSelectionCardID(canvasElement: HTMLElement): void {
  const canvas = within(canvasElement);
  const currentSelection = canvas.getByRole("article", {
    name: "Current selection",
  });
  expect(
    currentSelection.closest<HTMLElement>("[data-bento-card-id]")?.dataset
      .bentoCardId,
  ).toBe("current-selection");
}

export function currentSelectionCard(canvasElement: HTMLElement): HTMLElement {
  return within(canvasElement).getByRole("article", {
    name: "Current selection",
  });
}

export function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

export function expectNoPageHorizontalOverflow(canvasElement: HTMLElement): void {
  const documentElement = canvasElement.ownerDocument.documentElement;
  const overflowTolerance = 1;

  expect(
    documentElement.scrollWidth <=
      documentElement.clientWidth + overflowTolerance,
  ).toBe(true);
}

export function buttonVisibleStyle(button: HTMLElement): {
  backgroundColor: string;
  borderColor: string;
  color: string;
} {
  const view = button.ownerDocument.defaultView;
  if (!view) {
    throw new Error("expected a defaultView when reading button styles");
  }

  const styles = view.getComputedStyle(button);
  return {
    backgroundColor: styles.backgroundColor,
    borderColor: styles.borderTopColor,
    color: styles.color,
  };
}

export async function submitWorkCardControls(canvasElement: HTMLElement): Promise<{
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

export async function fillSubmitWorkCard(
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

export async function expectTypographyRegressionSurface(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const heading = await canvas.findByRole("heading", { name: "you-agent-factory" });
  const hiddenWordmark = within(heading).getByText("you-agent-factory");
  const toolbar = canvas.getByRole("region", { name: "dashboard summary" });
  const streamStatus = canvas.getByRole("status", {
    name: /you-agent-factory event stream (connecting|live)/,
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

export async function expectTimelineToolbarAlignment(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const toolbar = await canvas.findByRole("region", {
    name: "dashboard summary",
  });
  const heading = within(toolbar).getByRole("heading", { name: "you-agent-factory" });
  const activeTab = within(toolbar).getByRole("tab", { name: "root" });
  const slider = within(toolbar).getByRole<HTMLInputElement>("slider", {
    name: "Timeline tick",
  });
  const progressText = within(toolbar).getByText("5/5");
  const languageButton = within(toolbar).getByRole("button", {
    name: "Change language",
  });
  const actionsGroup = within(toolbar).getByRole("group", {
    name: "Dashboard actions",
  });
  const exportButton = within(toolbar).getByRole("button", { name: "Export PNG" });
  const sliderShell = requireValue(
    slider.closest<HTMLElement>("div"),
    "expected slider shell in dashboard toolbar",
  );
  const sliderMetaGroup = requireValue(
    progressText.parentElement,
    "expected timeline meta group in dashboard toolbar",
  );
  const headingRect = heading.getBoundingClientRect();
  const activeTabRect = activeTab.getBoundingClientRect();
  const actionsGroupRect = actionsGroup.getBoundingClientRect();
  const sliderRect = sliderShell.getBoundingClientRect();
  const sliderInputRect = slider.getBoundingClientRect();
  const progressTextRect = progressText.getBoundingClientRect();
  const languageButtonRect = languageButton.getBoundingClientRect();
  const exportButtonRect = exportButton.getBoundingClientRect();

  expect(sliderShell.className).toContain("gap-1.5");
  expect(sliderShell.className).toContain("px-2.5");
  expect(sliderMetaGroup.contains(progressText)).toBe(true);
  expect(
    within(toolbar).queryByRole("button", { name: "Return to current tick" }),
  ).toBeNull();
  expect(sliderRect.top).toBeGreaterThanOrEqual(headingRect.bottom - 1);
  expect(sliderRect.top).toBeGreaterThanOrEqual(activeTabRect.bottom - 1);
  expect(sliderRect.top).toBeGreaterThanOrEqual(actionsGroupRect.bottom - 1);
  expect(progressTextRect.left).toBeGreaterThanOrEqual(sliderInputRect.right - 1);
  expect(exportButtonRect.left).toBeGreaterThanOrEqual(
    actionsGroupRect.left - 1,
  );
  expect(languageButtonRect.left).toBeGreaterThanOrEqual(
    exportButtonRect.right - 1,
  );
}

export async function selectWorkstationRequest(
  canvasElement: HTMLElement,
  request: DashboardWorkstationRequest,
): Promise<void> {
  await selectWorkstationRequestByDispatchID(
    canvasElement,
    request.dispatch_id,
  );
}

export function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export async function selectWorkstationRequestByDispatchID(
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

export function workstationRequestStoryParameters(
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

export function workstationRequestWithStartedAt(
  request: DashboardWorkstationRequest,
  startedAt: string,
): DashboardWorkstationRequest {
  return {
    ...request,
    request_view: request.request_view
      ? {
          ...request.request_view,
          started_at: startedAt,
        }
      : request.request_view,
    started_at: startedAt,
  };
}

export function selectedWorkDispatchHistoryStoryParameters() {
  const active = workstationRequestWithStartedAt(
    {
      ...dashboardWorkstationRequestFixtures.noResponse,
      dispatch_id: "dispatch-review-active",
      request_id: "request-active-story",
      request_view: {
        ...dashboardWorkstationRequestFixtures.noResponse.request_view,
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

export function dispatchHistoryCard(
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

export function expectWorkOutcomeSeries(outcomeChart: HTMLElement): void {
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
