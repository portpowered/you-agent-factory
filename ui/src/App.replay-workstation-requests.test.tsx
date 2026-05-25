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
import { useFactoryTimelineStore } from "./features/timeline/state";
import {
  registerAppDashboardTestLifecycle,
  renderApp,
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
  const reviewWorkstationButton = screen.queryByRole("button", {
    name: "Select Review workstation",
  });
  if (reviewWorkstationButton) {
    fireEvent.click(reviewWorkstationButton);
  }

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
      name: new RegExp(
        `\\(${dispatchID.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\)$`,
      ),
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

describe("App replay workstation request flows", () => {
  registerAppDashboardTestLifecycle();

  it("smoke tests workstation-request runtime details against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: runtimeDetailsTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("11");
    expect(screen.getByText("11/11")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
      }),
    ).toBeTruthy();
    expect(
      useFactoryTimelineStore.getState().worldViewCache[11]?.runtime
        .workstation_requests_by_dispatch_id,
    ).toMatchObject(runtimeDetailsBackendWorkstationRequestsByDispatchID);

    fireEvent.click(
      screen.getByRole("button", {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
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
      within(completedSelection).getAllByText(
        runtimeDetailsFixtureIDs.completedDispatchID,
      ).length,
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
        name: runtimeDetailsFixtureIDs.failedWorkLabel,
      }),
    );

    const failedSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(failedSelection).getAllByText(
        runtimeDetailsFixtureIDs.failedFailureReason,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(failedSelection).getAllByText(
        runtimeDetailsFixtureIDs.failedFailureMessage,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(failedSelection).queryByRole("heading", {
        name: "Request counts",
      }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Select Review workstation" }),
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
      within(pendingSelection).getAllByText(
        runtimeDetailsFixtureIDs.activeDispatchID,
      ).length,
    ).toBeGreaterThan(0);
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
  }, 30_000);

  it("smoke tests mixed script and inference workstation-request history against backend expectations", async () => {
    renderApp({
      snapshot: historicalTimelineSnapshot,
      timelineEvents: scriptDashboardIntegrationTimelineEvents,
    });

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
    expect(
      within(scriptFailedSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureReason,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelection).getAllByText(
        scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelection).getAllByText("TIMEOUT").length,
    ).toBeGreaterThan(0);
    expect(
      within(scriptFailedSelection).getByText("script timed out"),
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
  });
});
