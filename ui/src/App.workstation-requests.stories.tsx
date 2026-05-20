import { expect, userEvent, within } from "storybook/test";

import { App } from "./App";
import { dashboardWorkstationRequestFixtures } from "./components/dashboard/fixtures";
import {
  currentSelectionCard,
  dispatchHistoryCard,
  expectCurrentSelectionCardID,
  markdownReadyWorkstationRequest,
  selectWorkstationRequest,
  selectedWorkDispatchHistoryStoryParameters,
  workstationRequestStoryParameters,
} from "./stories/dashboardStorySupport";

export default {
  title: "Infinite You/Workflow Dashboard",
  component: App,
};

export const WorkstationRequestSelection = {
  parameters: workstationRequestStoryParameters(
    markdownReadyWorkstationRequest,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      markdownReadyWorkstationRequest,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const inferenceAttempts = within(
      currentSelection.getByRole("region", { name: "Inference attempts" }),
    );
    const formattedAttempt = within(
      inferenceAttempts.getByRole("article", { name: "Inference attempt 2" }),
    );
    const requestBody = within(
      formattedAttempt.getByRole("region", { name: "Request body" }),
    );
    const responseBody = within(
      formattedAttempt.getByRole("region", { name: "Response body" }),
    );

    await expect(
      currentSelection.getByRole("heading", { name: "Request counts" }),
    ).toBeVisible();
    await expect(
      currentSelection.getByRole("heading", { name: "Response details" }),
    ).toBeVisible();
    await expect(
      currentSelection.getAllByText(markdownReadyWorkstationRequest.dispatch_id)
        .length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getAllByText("request-ready-story").length,
    ).toBeGreaterThan(0);
    await expect(
      requestBody.getByRole("heading", { level: 2, name: "Review checklist" }),
    ).toBeVisible();
    await expect(requestBody.getByRole("list")).toBeVisible();
    await expect(requestBody.getByText("Check the latest diff")).toBeVisible();
    expect(requestBody.queryByText("## Review checklist")).toBeNull();
    expect(requestBody.queryByText("```text")).toBeNull();
    await expect(
      responseBody.getByRole("heading", {
        level: 3,
        name: "Reviewer response",
      }),
    ).toBeVisible();
    await expect(responseBody.getByRole("list")).toBeVisible();
    await expect(
      responseBody.getByText("Confirm the diff is limited"),
    ).toBeVisible();
    expect(responseBody.queryByText("### Reviewer response")).toBeNull();
    expect(responseBody.queryByText("```text")).toBeNull();
    expect(
      currentSelection.queryByRole("heading", { name: "Active work" }),
    ).toBeNull();
    expect(
      currentSelection.queryByRole("heading", { name: "Execution details" }),
    ).toBeNull();
    expect(
      currentSelection.queryByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionNoResponse = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.noResponse,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.noResponse,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getByRole("heading", { name: "Request counts" }),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "No inference events are available for this selected work item.",
      ),
    ).toBeVisible();
    await expect(
      currentSelection.getByRole("heading", { name: "Response details" }),
    ).toBeVisible();
    expect(
      currentSelection.queryByRole("heading", { name: "Execution details" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionRejected = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.rejected,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.rejected,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const inferenceAttempts = within(
      currentSelection.getByRole("region", { name: "Inference attempts" }),
    );
    const responseDetails = within(
      currentSelection.getByRole("region", { name: "Response details" }),
    );

    expect(
      currentSelection.getAllByText("request-rejected-story").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeVisible();
    await expect(
      inferenceAttempts.getByText(
        "The active story needs revision before it can continue.",
      ),
    ).toBeVisible();
    expect(
      responseDetails.queryByText(/Inference attempts when available/),
    ).toBeNull();
    await expect(
      currentSelection.getByRole("heading", { name: "Response details" }),
    ).toBeVisible();
    expect(
      currentSelection.queryByRole("heading", { name: "Active work" }),
    ).toBeNull();
    expect(
      currentSelection.queryByRole("heading", { name: "Execution details" }),
    ).toBeNull();
    expect(
      currentSelection.queryByRole("heading", { name: "Error details" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionErrored = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.errored,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.errored,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const errorDetails = within(
      currentSelection.getByRole("region", { name: "Error details" }),
    );
    const outcomeRow = currentSelection
      .getAllByText("Outcome")
      .map((label) => label.closest("div"))
      .find((row) => row?.textContent?.includes("provider_rate_limit"));

    expect(
      currentSelection.getAllByText("request-error-story").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getByRole("heading", { name: "Inference attempts" }),
    ).toBeVisible();
    await expect(
      currentSelection.getByText(
        "Review the blocked story and explain the failure.",
      ),
    ).toBeVisible();
    expect(
      currentSelection.getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    expect(outcomeRow?.textContent).toContain("FAILED");
    expect(outcomeRow?.textContent).toContain(
      "Failure reason: provider_rate_limit",
    );
    expect(outcomeRow?.textContent).toContain(
      "Failure message: Provider rate limit exceeded while reviewing the story.",
    );
    expect(currentSelection.queryByText("Transition ID")).toBeNull();
    await expect(
      errorDetails.getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
    ).toBeVisible();
    expect(
      currentSelection.queryByRole("heading", { name: "Active work" }),
    ).toBeNull();
    expect(
      currentSelection.queryByRole("heading", { name: "Execution details" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const SelectedWorkDispatchHistorySmoke = {
  parameters: selectedWorkDispatchHistoryStoryParameters(),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: "Select Review workstation" }),
    );
    await userEvent.click(
      within(currentSelectionCard(canvasElement)).getByRole("button", {
        name: "Select work item Active Story",
      }),
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    const dispatchHistory = currentSelection.getByRole("region", {
      name: "Workstation dispatches",
    });

    await expect(
      currentSelection.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeVisible();
    expect(
      currentSelection.queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
    await expect(
      within(dispatchHistory).getByText("6 dispatches"),
    ).toBeVisible();
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

    const activeCard = dispatchHistoryCard(
      dispatchHistory,
      "dispatch-review-active",
    );
    await expect(
      within(activeCard).getByText("Current dispatch"),
    ).toBeVisible();
    await expect(
      within(activeCard).getByText("Active Story"),
    ).toBeVisible();
    await expect(
      within(activeCard).getByText("Started at"),
    ).toBeVisible();
    expect(within(activeCard).queryByText("Dispatched")).toBeNull();
    expect(within(activeCard).queryByText("Responded")).toBeNull();
    expect(within(activeCard).queryByText("Errored")).toBeNull();
    const inferenceAttemptsToggle = within(activeCard).getByRole("button", {
      name: "Expand",
    });
    expect(inferenceAttemptsToggle.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(activeCard).queryByText(
        "No inference attempt details have been recorded for this dispatch yet.",
      ),
    ).toBeNull();
    await userEvent.click(inferenceAttemptsToggle);
    expect(inferenceAttemptsToggle.getAttribute("aria-expanded")).toBe("true");
    await expect(
      within(activeCard).getByText(
        "No inference attempt details have been recorded for this dispatch yet.",
      ),
    ).toBeVisible();
    await expect(
      within(activeCard).getByRole("button", {
        name: "Select work item Active Story",
      }),
    ).toBeVisible();

    const erroredCard = dispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.errored.dispatch_id,
    );
    await expect(
      within(erroredCard).getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
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
    await expect(
      within(scriptSuccessCard).getByText("Script attempts"),
    ).toBeVisible();
    const scriptAttemptsToggle = within(scriptSuccessCard).getByRole("button", {
      name: "Expand",
    });
    expect(scriptAttemptsToggle.getAttribute("aria-expanded")).toBe("false");
    expect(within(scriptSuccessCard).queryByText("script success stdout")).toBeNull();
    await userEvent.click(scriptAttemptsToggle);
    expect(scriptAttemptsToggle.getAttribute("aria-expanded")).toBe("true");
    await expect(
      within(scriptSuccessCard).getAllByText("script-tool").length,
    ).toBeGreaterThan(0);
    await expect(
      within(scriptSuccessCard).getAllByText("script success stdout").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptSuccessCard).queryByText("Current dispatch"),
    ).toBeNull();

    const scriptFailedCard = dispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id,
    );
    expect(
      within(scriptFailedCard).getAllByText("TIMEOUT").length,
    ).toBeGreaterThan(0);
    await expect(
      within(scriptFailedCard).getByText("Script timed out."),
    ).toBeVisible();
    await expect(
      within(scriptFailedCard).getByText("Started at"),
    ).toBeVisible();
    expect(within(scriptFailedCard).queryByText("Current dispatch")).toBeNull();

    await expect(
      canvas.getByRole("article", { name: "Trace drill-down" }),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptPending = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.scriptPending,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.scriptPending,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getAllByText("request-script-pending-story").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getAllByText("script-tool").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getByText(
        "Script response details are not available for this workstation request yet.",
      ),
    ).toBeVisible();
    expect(
      currentSelection.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptSuccess = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.scriptSuccess,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.scriptSuccess,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getAllByText("request-script-success-story").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getAllByText("script success stdout").length,
    ).toBeGreaterThan(0);
    await expect(
      currentSelection.getAllByText("SUCCEEDED").length,
    ).toBeGreaterThan(0);
    await expect(currentSelection.getAllByText("222ms").length).toBeGreaterThan(
      0,
    );
    expect(
      currentSelection.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};

export const WorkstationRequestSelectionScriptFailed = {
  parameters: workstationRequestStoryParameters(
    dashboardWorkstationRequestFixtures.scriptFailed,
  ),
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await selectWorkstationRequest(
      canvasElement,
      dashboardWorkstationRequestFixtures.scriptFailed,
    );

    const currentSelection = within(currentSelectionCard(canvasElement));
    await expect(
      currentSelection.getAllByText("request-script-failed-story").length,
    ).toBeGreaterThan(0);
    await expect(currentSelection.getByText("script_timeout")).toBeVisible();
    await expect(currentSelection.getByText("TIMEOUT")).toBeVisible();
    await expect(
      currentSelection.getAllByText("script timed out").length,
    ).toBeGreaterThan(0);
    expect(
      currentSelection.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    expectCurrentSelectionCardID(canvasElement);
  },
};
