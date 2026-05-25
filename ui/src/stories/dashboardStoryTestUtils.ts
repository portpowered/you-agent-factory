import { expect, userEvent, within } from "storybook/test";

import type { DashboardWorkstationRequest } from "../api/dashboard";
import { dashboardWorkstationRequestFixtures } from "../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../components/dashboard/test-fixtures";
import {
  findSubmitWorkCard,
  getSubmitWorkCardControls,
  submitWorkCardQueryContract,
} from "../testing/submit-work-card-queries";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
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
  requestNameField: HTMLInputElement;
  requestField: HTMLTextAreaElement;
  scope: ReturnType<typeof within>;
  submitButton: HTMLButtonElement;
  workTypeField: HTMLSelectElement;
}> {
  const canvas = within(canvasElement);
  const dashboardGrid = await canvas.findByRole("region", {
    name: submitWorkCardQueryContract.dashboardRegionName,
  });
  const submitWorkCard = await findSubmitWorkCard(within(dashboardGrid));
  const submitWorkScope = within(submitWorkCard);
  const { requestName, requestText, submitButton, workType } =
    getSubmitWorkCardControls(submitWorkScope);

  return {
    requestField: requestText,
    requestNameField: requestName,
    scope: submitWorkScope,
    submitButton,
    workTypeField: workType,
  };
}

export async function fillSubmitWorkCard(
  canvasElement: HTMLElement,
  requestName: string,
  requestText: string,
): Promise<{
  requestNameField: HTMLInputElement;
  requestField: HTMLTextAreaElement;
  scope: ReturnType<typeof within>;
  submitButton: HTMLButtonElement;
  workTypeField: HTMLSelectElement;
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
  const heading = await canvas.findByRole("heading", { name: "U" });
  const toolbar = canvas.getByRole("region", { name: "dashboard summary" });
  const streamStatus = canvas.getByRole("status", {
    name: /Event stream (connecting|live)/,
  });

  expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
  expect(heading.textContent).toBe("U");
  expect(streamStatus.className).toContain("inline-flex");
  expect(streamStatus.className).toContain("rounded-full");
  expect(streamStatus.className).not.toContain("sr-only");
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
  const heading = within(toolbar).getByRole("heading", { name: "U" });
  const streamStatus = within(toolbar).getByRole("status", {
    name: /Event stream (connecting|live)/,
  });
  const activeTab = within(toolbar).getByRole("tab", { name: "root" });
  const slider = within(toolbar).getByRole<HTMLInputElement>("slider", {
    name: "Timeline tick",
  });
  const progressText = within(toolbar).getByText(/^\d+\/\d+$/);
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
  const primaryRow = requireValue(
    heading.closest<HTMLElement>("div"),
    "expected primary dashboard toolbar row",
  );
  const secondaryRow = requireValue(
    sliderShell.parentElement,
    "expected secondary dashboard toolbar row",
  );
  const sliderMetaGroup = requireValue(
    progressText.parentElement,
    "expected timeline meta group in dashboard toolbar",
  );
  const headingRect = heading.getBoundingClientRect();
  const activeTabRect = activeTab.getBoundingClientRect();
  const actionsGroupRect = actionsGroup.getBoundingClientRect();
  const primaryRowRect = primaryRow.getBoundingClientRect();
  const secondaryRowRect = secondaryRow.getBoundingClientRect();
  const sliderRect = sliderShell.getBoundingClientRect();
  const sliderInputRect = slider.getBoundingClientRect();
  const progressTextRect = progressText.getBoundingClientRect();
  const languageButtonRect = languageButton.getBoundingClientRect();
  const streamStatusRect = streamStatus.getBoundingClientRect();
  const exportButtonRect = exportButton.getBoundingClientRect();
  const headerControls = Array.from(
    toolbar.querySelectorAll(
      '[aria-label="Timeline tick"], [aria-label="Change language"], [aria-label="Export PNG"], [role="status"]',
    ),
  );

  expect(sliderShell.className).toContain("gap-1.5");
  expect(sliderShell.className).toContain("px-2.5");
  expect(streamStatus.className).toContain("inline-flex");
  expect(streamStatus.className).toContain("rounded-full");
  expect(streamStatus.className).not.toContain("sr-only");
  expect(sliderMetaGroup.contains(progressText)).toBe(true);
  expect(
    within(toolbar).queryByRole("button", { name: "Return to current tick" }),
  ).toBeNull();
  expect(headerControls).toHaveLength(4);
  expect(headerControls[0]).toBe(streamStatus);
  expect(headerControls[1]).toBe(exportButton);
  expect(headerControls[2]).toBe(languageButton);
  expect(headerControls[3]).toBe(slider);
  expect(primaryRowRect.top).toBeLessThan(secondaryRowRect.top);
  expect(sliderRect.top).toBeGreaterThanOrEqual(headingRect.bottom - 1);
  expect(sliderRect.top).toBeGreaterThanOrEqual(activeTabRect.bottom - 1);
  expect(sliderRect.top).toBeGreaterThanOrEqual(actionsGroupRect.bottom - 1);
  expect(progressTextRect.width).toBeGreaterThan(0);
  expect(progressTextRect.height).toBeGreaterThan(0);
  expect(progressTextRect.top).toBeGreaterThanOrEqual(sliderInputRect.top - 1);
  expect(exportButtonRect.left).toBeGreaterThanOrEqual(actionsGroupRect.left - 1);
  expect(languageButtonRect.width).toBeGreaterThan(0);
  expect(languageButtonRect.height).toBeGreaterThan(0);
  expect(streamStatusRect.width).toBeGreaterThan(1);
  expect(streamStatusRect.height).toBeGreaterThan(1);
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
  const chart = outcomeChart.matches("[data-work-chart-ready='true']")
    ? outcomeChart
    : outcomeChart.querySelector<HTMLElement>("[data-work-chart-ready='true']");

  expect(chart).not.toBeNull();
  expect(chart?.getAttribute("data-work-chart-ready")).toBe("true");
  expect(chart?.getAttribute("data-work-chart-visible-ticks")).toBeTruthy();
  expect(
    chart?.querySelector("[data-work-chart-overlay='true']"),
  ).not.toBeNull();
}
