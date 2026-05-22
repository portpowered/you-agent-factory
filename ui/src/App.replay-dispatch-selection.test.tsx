import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { dashboardWorkstationRequestFixtures } from "./components/dashboard/fixtures";
import {
  activeSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./testing/app-shell-test-utils";

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
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
  const resolvedRequestHistorySection = requireValue(
    requestHistorySection,
    "expected request history section for replay smoke",
  );

  fireEvent.click(
    within(resolvedRequestHistorySection).getByRole("button", {
      name: "Expand",
    }),
  );
  fireEvent.click(
    within(resolvedRequestHistorySection).getByRole("button", {
      name: new RegExp(`\\(${escapeRegExp(dispatchID)}\\)$`),
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

describe("App replay dispatch selection flows", () => {
  registerAppDashboardTestLifecycle();

  it.each([
    {
      label: "ready",
      requestProjection: dashboardWorkstationRequestFixtures.ready,
      verify: (currentSelection: HTMLElement) => {
        const readyAttempt = expandWorkstationRequestAttempt(
          currentSelection,
          2,
        );
        const responseBody = expandAttemptBodySection(
          readyAttempt,
          "Response body",
        );
        expect(
          within(currentSelection).getAllByText("request-ready-story").length,
        ).toBeGreaterThan(0);
        expect(
          within(currentSelection).queryByRole("heading", {
            name: "Request counts",
          }),
        ).toBeNull();
        expect(
          within(responseBody).getByText("Ready for the next workstation."),
        ).toBeTruthy();
      },
    },
    {
      label: "no-response",
      requestProjection: dashboardWorkstationRequestFixtures.noResponse,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getByText(
            "No inference events are available for this selected work item.",
          ),
        ).toBeTruthy();
      },
    },
    {
      label: "rejected",
      requestProjection: dashboardWorkstationRequestFixtures.rejected,
      verify: (currentSelection: HTMLElement) => {
        const rejectedAttempt = expandWorkstationRequestAttempt(
          currentSelection,
          1,
        );
        const responseBody = expandAttemptBodySection(
          rejectedAttempt,
          "Response body",
        );
        expect(
          within(responseBody).getByText(
            "The active story needs revision before it can continue.",
          ),
        ).toBeTruthy();
      },
    },
    {
      label: "errored",
      requestProjection: dashboardWorkstationRequestFixtures.errored,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getByRole("heading", {
            name: "Error details",
          }),
        ).toBeTruthy();
        expect(
          within(currentSelection).getAllByText("provider_rate_limit").length,
        ).toBeGreaterThan(0);
      },
    },
    {
      label: "script-success",
      requestProjection: dashboardWorkstationRequestFixtures.scriptSuccess,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getAllByText("request-script-success-story")
            .length,
        ).toBeGreaterThan(0);
        expect(
          within(currentSelection).queryByRole("heading", {
            name: "Request counts",
          }),
        ).toBeNull();
        expect(
          within(currentSelection).queryByRole("heading", {
            name: "Execution details",
          }),
        ).toBeNull();
      },
    },
    {
      label: "script-failed",
      requestProjection: dashboardWorkstationRequestFixtures.scriptFailed,
      verify: (currentSelection: HTMLElement) => {
        expect(
          within(currentSelection).getAllByText("request-script-failed-story")
            .length,
        ).toBeGreaterThan(0);
        expect(
          within(currentSelection).getByRole("heading", {
            name: "Error details",
          }),
        ).toBeTruthy();
        expect(
          within(currentSelection).getByText("script_timeout"),
        ).toBeTruthy();
      },
    },
  ])("selects a workstation dispatch and routes $label request context through work-item details", async ({
    requestProjection,
    verify,
  }) => {
    renderApp({
      snapshot: activeSnapshot,
      workstationRequestsByDispatchID: {
        [requestProjection.dispatch_id]: requestProjection,
      },
    });

    await selectReviewRequest(requestProjection.dispatch_id);

    await waitFor(() => {
      const currentSelection = screen.getByRole("article", {
        name: "Current selection",
      });
      expect(
        within(currentSelection).getAllByText(requestProjection.dispatch_id)
          .length,
      ).toBeGreaterThan(0);
      expect(
        within(currentSelection).queryByRole("heading", {
          name: "Active work",
        }),
      ).toBeNull();
      verify(currentSelection);
    });
  });
});
