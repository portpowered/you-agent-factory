import { fireEvent, render, screen, within } from "@testing-library/react";

import { CompletedFailedWorkstationCard } from "./terminal-work-card";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./typography";
import type { DashboardProviderSessionAttempt } from "../../api/dashboard/types";

const failedAttempt: DashboardProviderSessionAttempt = {
  dispatch_id: "dispatch-repair-failed",
  outcome: "FAILED",
  provider_session: {
    id: "sess-failed-story",
    kind: "session_id",
    provider: "codex",
  },
  transition_id: "repair",
  workstation_name: "Repair",
  work_items: [{ display_name: "Failed Story", work_id: "work-failed-story" }],
};

describe("CompletedFailedWorkstationCard", () => {
  it("renders exactly two expandable outcome rows with completed and failed items", () => {
    const onSelectItem = vi.fn();

    render(
      <CompletedFailedWorkstationCard
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[
          { attempts: [failedAttempt], label: "Failed Story", traceWorkID: "work-failed-story" },
        ]}
        onSelectItem={onSelectItem}
      />,
    );

    const rows = document.querySelectorAll("[data-terminal-work-status]");
    expect(rows).toHaveLength(2);
    const completedHeading = screen.getByRole("heading", { name: "Completed" });
    const failedHeading = screen.getByRole("heading", { name: "Failed" });
    expect(completedHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);
    expect(failedHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);
    const completedTitle = screen.getByRole("heading", { name: "Completed" }).closest("[data-terminal-work-title]");
    const failedTitle = screen.getByRole("heading", { name: "Failed" }).closest("[data-terminal-work-title]");
    expect(completedTitle?.className).toContain("flex");
    expect(failedTitle?.className).toContain("flex");
    const completedRow = screen.getByRole("heading", { name: "Completed" }).closest("section");
    expect(completedRow).toBeTruthy();
    expect(
      within(completedTitle as HTMLElement)
        .getByRole("img", { name: "Completed work" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("terminal");
    expect(
      within(failedTitle as HTMLElement)
        .getByRole("img", { name: "Failed work" })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("failed");
    const doneStoryButton = screen.getByRole("button", { name: /Done Story/ });
    expect(doneStoryButton.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    expect(doneStoryButton).toBeTruthy();
    expect(screen.getByRole("button", { name: /Failed Story/ })).toBeTruthy();
    const failedMeta = screen.getByText(/Failed at Repair; codex \/ session_id \/ sess-failed-story/);
    expect(failedMeta.className).toContain(DASHBOARD_SUPPORTING_TEXT_CLASS);
    expect(failedMeta).toBeTruthy();
    expect(within(completedRow as HTMLElement).getByText("1 item").className).toContain(
      DASHBOARD_SUPPORTING_TEXT_CLASS,
    );

    fireEvent.click(doneStoryButton);
    expect(onSelectItem).toHaveBeenCalledWith(
      "completed",
      expect.objectContaining({ label: "Done Story", traceWorkID: "work-done-story" }),
    );

    fireEvent.click(screen.getByRole("button", { name: /Failed Story/ }));
    expect(onSelectItem).toHaveBeenCalledWith(
      "failed",
      expect.objectContaining({ label: "Failed Story", traceWorkID: "work-failed-story" }),
    );
  });

  it("collapses each row independently without hiding the other row heading", () => {
    render(
      <CompletedFailedWorkstationCard
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[{ label: "Failed Story", traceWorkID: "work-failed-story" }]}
        onSelectItem={vi.fn()}
      />,
    );

    const completedRow = screen.getByRole("heading", { name: "Completed" }).closest("section");
    const failedRow = screen.getByRole("heading", { name: "Failed" }).closest("section");
    if (!completedRow || !failedRow) {
      throw new Error("expected completed and failed rows");
    }

    fireEvent.click(within(completedRow).getByRole("button", { name: "Collapse" }));

    expect(within(completedRow).queryByRole("button", { name: /Done Story/ })).toBeNull();
    expect(screen.getByRole("heading", { name: "Failed" })).toBeTruthy();
    expect(within(failedRow).getByRole("button", { name: /Failed Story/ })).toBeTruthy();

    fireEvent.click(within(failedRow).getByRole("button", { name: "Collapse" }));

    expect(screen.getByRole("heading", { name: "Completed" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Failed" })).toBeTruthy();
    expect(within(failedRow).queryByRole("button", { name: /Failed Story/ })).toBeNull();

    fireEvent.click(within(completedRow).getByRole("button", { name: "Expand" }));
    expect(within(completedRow).getByRole("button", { name: /Done Story/ })).toBeTruthy();
  });

  it("marks the selected outcome item through the shared button state", () => {
    render(
      <CompletedFailedWorkstationCard
        completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
        failedItems={[{ label: "Failed Story", traceWorkID: "work-failed-story" }]}
        onSelectItem={vi.fn()}
        selectedItem={{ label: "Failed Story", status: "failed" }}
      />,
    );

    expect(screen.getByRole("button", { name: /Failed Story/ }).getAttribute("data-selected")).toBe(
      "true",
    );
    expect(screen.getByRole("button", { name: /Done Story/ }).getAttribute("data-selected")).toBeNull();
  });

  it("renders explicit empty messages for each outcome row", () => {
    render(
      <CompletedFailedWorkstationCard
        completedItems={[]}
        failedItems={[]}
        onSelectItem={vi.fn()}
      />,
    );

    expect(screen.getByText("No completed work recorded yet.")).toBeTruthy();
    expect(screen.getByText("No failed work recorded yet.")).toBeTruthy();
  });
});

