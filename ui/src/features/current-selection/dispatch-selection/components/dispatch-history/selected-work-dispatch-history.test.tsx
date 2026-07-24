import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { workstationRequest } from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

describe("SelectedWorkDispatchHistorySection", () => {
  it("uses the unified operations heading when operation history is provided", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          fallbackProviderSessions={[]}
          operationHistory={[]}
          requests={[]}
          selectedWorkID="work-ops"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByRole("heading", { name: "Work operations" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "No move or workstation operations have been recorded yet for this work item.",
      ),
    ).toBeTruthy();
  });

  it("keeps the workstation-only heading when operation history is omitted", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          fallbackProviderSessions={[]}
          requests={[
            workstationRequest("dispatch-legacy", {
              workstation_name: "Review",
            }),
          ]}
          selectedWorkID="work-legacy"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
  });

  it("renders dispatch history without the outer outlined content container", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          currentDispatchID="dispatch-flush"
          fallbackProviderSessions={[]}
          requests={[
            workstationRequest("dispatch-flush", {
              workstation_name: "Review",
            }),
          ]}
          selectedWorkID="work-flush"
        />
      </CurrentSelectionLocaleProvider>,
    );

    const contentWrapper = document.getElementById(
      "current-selection-work-item-dispatches-content",
    );

    expect(contentWrapper?.className).toBe("grid");
  });

  it("uses larger headline-sized headers, keeps the active pill on the right, and exposes expandable request details", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          currentDispatchID="dispatch-card"
          fallbackProviderSessions={[]}
          requests={[
            workstationRequest("dispatch-card", {
              workstation_name: "Review",
            }),
          ]}
          selectedWorkID="work-card"
        />
      </CurrentSelectionLocaleProvider>,
    );

    const historyCard = screen.getByRole("article", {
      name: /Active Story.*dispatch-card/i,
    });
    const title = within(historyCard).getByText("Active Story");

    expect(title.className).toContain("type-headline-large");
    expect(within(historyCard).getByText("Current dispatch")).toBeTruthy();
    expect(within(historyCard).queryByText("Workstation")).toBeNull();
    expect(
      within(historyCard).getByRole("heading", { name: "Summary" }),
    ).toBeTruthy();
    const header = title.closest("div");
    expect(header?.parentElement?.className).toContain("justify-between");

    const requestDetailsSection = within(historyCard)
      .getByRole("heading", { name: "Summary" })
      .closest("section");
    if (!requestDetailsSection) {
      throw new Error("expected request details section");
    }

    fireEvent.click(
      within(requestDetailsSection).getByRole("button", { name: "Expand" }),
    );

    expect(within(historyCard).getAllByText("dispatch-card").length).toBe(1);
    expect(within(historyCard).getByText("Request details")).toBeTruthy();
  });
});
