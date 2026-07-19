import { fireEvent, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  runtimeDetailsBackendWorkstationRequestsByDispatchID,
  runtimeDetailsFixtureIDs,
  runtimeDetailsTimelineEvents,
  scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID,
  scriptDashboardIntegrationFixtureIDs,
  scriptDashboardIntegrationTimelineEvents,
} from "./components/dashboard/fixtures";
import { useFactoryTimelineStore } from "./features/timeline/state/factoryTimelineStore";
import {
  registerAppDashboardTestLifecycle,
  renderApp,
  settleAppShellDashboardEffects,
} from "./testing/app-shell-test-utils";
import { historicalTimelineSnapshot } from "./testing/app-shell-timeline-test-utils";

function expectDefinitionValue(
  section: HTMLElement,
  label: string,
  expectedValue: string,
): void {
  const term = within(section).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected definition row for ${label}`);
  }

  expect(within(row).getByText(expectedValue)).toBeTruthy();
}

async function selectReviewRequest(dispatchID: string): Promise<void> {
  fireEvent.click(
    await screen.findByLabelText("Select Review workstation"),
  );

  const workstationSelection = await screen.findByRole("article", {
    name: "Current selection",
  });
  const requestHistorySection = within(workstationSelection)
    .getByRole("heading", { name: "Request history" })
    .closest("section");
  if (!(requestHistorySection instanceof HTMLElement)) {
    throw new Error("expected request history section for replay smoke");
  }

  fireEvent.click(
    within(requestHistorySection).getByRole("button", {
      name: "Expand",
    }),
  );
  fireEvent.click(
    within(requestHistorySection).getByRole("button", {
      name: `Select workstation request ${dispatchID}`,
    }),
  );
}

function expandWorkstationRequestAttempt(
  currentSelection: HTMLElement,
  attemptNumber: number,
): HTMLElement {
  const inferenceAttempts = within(currentSelection).getByRole("region", {
    name: "Inference attempts",
  });
  const attemptCard = within(inferenceAttempts).getByRole("article", {
    name: `Inference attempt ${attemptNumber}`,
  });
  const toggle = within(attemptCard).getByRole("button", {
    name: `Expand attempt ${attemptNumber}`,
  });

  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");

  return attemptCard;
}

function expandAttemptBodySection(
  attemptCard: HTMLElement,
  bodyLabel: "Request body" | "Response body",
): HTMLElement {
  const toggle = within(attemptCard).getByRole("button", {
    name: `Expand ${bodyLabel.toLowerCase()}`,
  });

  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");

  return within(attemptCard).getByRole("region", { name: bodyLabel });
}

function expandCurrentSelectionSection(
  currentSelection: HTMLElement,
  sectionTitle: string,
): HTMLElement {
  const section = within(currentSelection).getByRole("region", {
    name: sectionTitle,
  });
  const toggle =
    within(section).queryByRole("button", { name: "Expand" }) ??
    within(section).getByRole("button", { name: "Collapse" });

  if (toggle.getAttribute("aria-expanded") === "false") {
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  } else {
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  }

  return section;
}

function getDispatchCard(
  currentSelection: HTMLElement,
  dispatchID: string,
): HTMLElement {
  return within(currentSelection).getByRole("article", {
    name: new RegExp(dispatchID),
  });
}

describe("App replay workstation request flows", () => {
  registerAppDashboardTestLifecycle();

  it("smoke tests workstation-request runtime details against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: runtimeDetailsTimelineEvents,
    });
    await settleAppShellDashboardEffects();

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("11");
    expect(screen.getByText("11/11")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: `Select work item ${runtimeDetailsFixtureIDs.completedWorkLabel}`,
      }),
    ).toBeTruthy();
    expect(
      useFactoryTimelineStore.getState().worldViewCache[11]?.runtime
        .workstation_requests_by_dispatch_id,
    ).toMatchObject(runtimeDetailsBackendWorkstationRequestsByDispatchID);

    fireEvent.click(
      screen.getByRole("button", {
        name: `Select work item ${runtimeDetailsFixtureIDs.completedWorkLabel}`,
      }),
    );

    const completedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(completedSelection).queryByRole("heading", {
        name: "Request counts",
      }),
    ).toBeNull();
    expect(
      getDispatchCard(
        completedSelection,
        runtimeDetailsFixtureIDs.completedDispatchID,
      ),
    ).toBeTruthy();
    expect(
      within(completedSelection).getByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeTruthy();
    expect(
      within(completedSelection).queryByRole("link", { name: "Open trace" }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", {
        name: `Select work item ${runtimeDetailsFixtureIDs.failedWorkLabel}`,
      }),
    );

    const failedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    const failedSelectionDetails = expandCurrentSelectionSection(
      failedSelection,
      "Failure details",
    );
    expect(
      within(failedSelectionDetails).getAllByText(
        runtimeDetailsFixtureIDs.failedFailureReason,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(failedSelectionDetails).getAllByText(
        runtimeDetailsFixtureIDs.failedFailureMessage,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(failedSelection).queryByRole("heading", {
        name: "Request counts",
      }),
    ).toBeNull();

    fireEvent.click(
      await screen.findByLabelText("Select Review workstation"),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: `Select work item ${runtimeDetailsFixtureIDs.activeWorkLabel}`,
      }),
    );

    const pendingSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(pendingSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      getDispatchCard(pendingSelection, runtimeDetailsFixtureIDs.activeDispatchID),
    ).toBeTruthy();
    expectDefinitionValue(pendingSelection, "Workstation dispatches", "1");
    expect(
      within(pendingSelection).queryByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeNull();
    expect(
      within(pendingSelection).queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      screen.queryByText(runtimeDetailsFixtureIDs.unsafeSystemPromptBody),
    ).toBeNull();
    expect(
      screen.queryByText(runtimeDetailsFixtureIDs.unsafeUserMessageBody),
    ).toBeNull();
  }, 90_000);

  it("smoke tests mixed script and inference workstation-request history against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: scriptDashboardIntegrationTimelineEvents,
    });
    await settleAppShellDashboardEffects();

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("14");
    expect(screen.getByText("14/14")).toBeTruthy();
    expect(
      useFactoryTimelineStore.getState().worldViewCache[14]?.runtime
        .workstation_requests_by_dispatch_id,
    ).toMatchObject(
      scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID,
    );

    await selectReviewRequest(
      scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
    );

    const scriptSuccessSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(scriptSuccessSelection).queryByRole("heading", {
        name: "Request counts",
      }),
    ).toBeNull();
    expect(
      within(scriptSuccessSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptSuccessSelection).getByText("script success stdout"),
    ).toBeTruthy();
    expect(
      within(scriptSuccessSelection).getAllByText("SUCCEEDED").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptSuccessSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();

    await selectReviewRequest(
      scriptDashboardIntegrationFixtureIDs.failedDispatchID,
    );

    const scriptFailedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    const scriptFailedSelectionDetails = expandCurrentSelectionSection(
      scriptFailedSelection,
      "Error details",
    );
    expect(
      within(scriptFailedSelectionDetails).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureReason,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelectionDetails).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelectionDetails).getByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      ),
    ).toBeTruthy();
    expect(
      within(scriptFailedSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();

    await selectReviewRequest(
      scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
    );

    const inferenceSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    const inferenceAttempt = expandWorkstationRequestAttempt(
      inferenceSelection,
      1,
    );
    const inferenceResponseBody = expandAttemptBodySection(
      inferenceAttempt,
      "Response body",
    );
    expect(
      within(inferenceSelection).getByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeTruthy();
    expect(
      within(inferenceResponseBody).getAllByText(
        scriptDashboardIntegrationFixtureIDs.inferenceResponseText,
      ).length,
    ).toBeGreaterThan(0);
  }, 90_000);
});
