import { fireEvent, render, screen, within } from "@testing-library/react";
import type { DashboardProviderSessionAttempt } from "../../../api/dashboard/types";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { getTerminalWorkMessages } from "../messages/terminal-work";
import { CompletedFailedWorkstationCard } from "./terminal-work-card";

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
    const messages = getTerminalWorkMessages("en");

    render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[
          {
            attempts: [failedAttempt],
            label: "Failed Story",
            traceWorkID: "work-failed-story",
            workstationName: "setup-workspace",
          },
        ]}
        onSelectItem={onSelectItem}
      />,
    );

    const rows = document.querySelectorAll("[data-terminal-work-status]");
    expect(rows).toHaveLength(2);
    const completedHeading = screen.getByRole("heading", {
      name: messages.rowTitle("completed"),
    });
    const failedHeading = screen.getByRole("heading", {
      name: messages.rowTitle("failed"),
    });
    expect(completedHeading.className).toContain(
      DASHBOARD_SECTION_HEADING_CLASS,
    );
    expect(failedHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);
    const completedTitle = completedHeading.closest(
      "[data-terminal-work-title]",
    );
    const failedTitle = failedHeading.closest("[data-terminal-work-title]");
    expect(completedTitle?.className).toContain("flex");
    expect(failedTitle?.className).toContain("flex");
    const completedRow = completedHeading.closest("section");
    const failedRow = failedHeading.closest("section");
    expect(completedRow).toBeTruthy();
    expect(completedRow?.getAttribute("data-terminal-work-status")).toBe(
      "completed",
    );
    expect(failedRow?.getAttribute("data-terminal-work-status")).toBe("failed");
    expect(completedRow?.getAttribute("aria-labelledby")).toBe(
      completedHeading.id,
    );
    expect(failedRow?.getAttribute("aria-labelledby")).toBe(failedHeading.id);
    expect(
      within(completedTitle as HTMLElement)
        .getByRole("img", { name: messages.iconLabel("completed") })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("terminal");
    expect(
      within(completedTitle as HTMLElement)
        .getByRole("img", {
          name: messages.iconLabel("completed"),
        })
        .getAttribute("class"),
    ).toContain("text-af-on-info");
    expect(
      within(failedTitle as HTMLElement)
        .getByRole("img", { name: messages.iconLabel("failed") })
        .getAttribute("data-graph-semantic-icon"),
    ).toBe("failed");
    expect(
      within(failedTitle as HTMLElement)
        .getByRole("img", {
          name: messages.iconLabel("failed"),
        })
        .getAttribute("class"),
    ).toContain("text-af-on-danger");
    const doneStoryButton = screen.getByRole("button", { name: /Done Story/ });
    expect(doneStoryButton.getAttribute("aria-label")).toBe("Done Story");
    expect(doneStoryButton.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    expect(doneStoryButton.className).toContain("text-af-on-info");
    expect(doneStoryButton).toBeTruthy();
    const failedStoryButton = screen.getByRole("button", {
      name: /Failed Story/,
    });
    expect(failedStoryButton.getAttribute("aria-label")).toBe("Failed Story");
    expect(failedStoryButton.className).toContain("text-af-on-danger");
    expect(failedStoryButton).toBeTruthy();
    const failedMeta = screen.getByText("Failed at setup-workspace");
    expect(failedMeta.className).toContain(DASHBOARD_SUPPORTING_TEXT_CLASS);
    expect(failedMeta).toBeTruthy();
    expect(
      within(completedRow as HTMLElement).getByText(messages.itemCountLabel(1))
        .className,
    ).toContain(DASHBOARD_SUPPORTING_TEXT_CLASS);

    fireEvent.click(doneStoryButton);
    expect(onSelectItem).toHaveBeenCalledWith(
      "completed",
      expect.objectContaining({
        label: "Done Story",
        traceWorkID: "work-done-story",
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /Failed Story/ }));
    expect(onSelectItem).toHaveBeenCalledWith(
      "failed",
      expect.objectContaining({
        label: "Failed Story",
        traceWorkID: "work-failed-story",
      }),
    );
  });

  it("formats fallback terminal summaries as state plus workstation without provider-session text", () => {
    render(
      <CompletedFailedWorkstationCard
        completedItems={[]}
        failedItems={[
          {
            attempts: [failedAttempt],
            label: "Failed Story",
            traceWorkID: "work-failed-story",
          },
        ]}
        onSelectItem={vi.fn()}
      />,
    );

    expect(screen.getByText("Failed at Repair")).toBeTruthy();
    expect(screen.queryByText(/session_id/i)).toBeNull();
    expect(screen.queryByText(/codex/i)).toBeNull();
  });

  it("collapses each row independently without hiding the other row heading", () => {
    const messages = getTerminalWorkMessages("en");

    render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[
          { label: "Failed Story", traceWorkID: "work-failed-story" },
        ]}
        onSelectItem={vi.fn()}
      />,
    );

    const completedRow = screen
      .getByRole("heading", { name: messages.rowTitle("completed") })
      .closest("section");
    const failedRow = screen
      .getByRole("heading", { name: messages.rowTitle("failed") })
      .closest("section");
    if (!completedRow || !failedRow) {
      throw new Error("expected completed and failed rows");
    }

    fireEvent.click(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(true),
      }),
    );

    expect(
      within(completedRow).queryByRole("button", { name: /Done Story/ }),
    ).toBeNull();
    expect(
      screen.getByRole("heading", { name: messages.rowTitle("failed") }),
    ).toBeTruthy();
    expect(
      within(failedRow).getByRole("button", { name: /Failed Story/ }),
    ).toBeTruthy();

    fireEvent.click(
      within(failedRow).getByRole("button", {
        name: messages.disclosureLabel(true),
      }),
    );

    expect(
      screen.getByRole("heading", { name: messages.rowTitle("completed") }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: messages.rowTitle("failed") }),
    ).toBeTruthy();
    expect(
      within(failedRow).queryByRole("button", { name: /Failed Story/ }),
    ).toBeNull();

    fireEvent.click(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(false),
      }),
    );
    expect(
      within(completedRow).getByRole("button", { name: /Done Story/ }),
    ).toBeTruthy();
  });

  it("keeps disclosure ids and labels distinct across duplicate terminal-work cards", () => {
    const messages = getTerminalWorkMessages("en");

    render(
      <>
        <CompletedFailedWorkstationCard
          completedItems={[
            { label: "Done Story", traceWorkID: "work-done-story" },
          ]}
          failedItems={[
            { label: "Failed Story", traceWorkID: "work-failed-story" },
          ]}
          onSelectItem={vi.fn()}
          widgetId="terminal-work::one"
        />
        <CompletedFailedWorkstationCard
          completedItems={[
            { label: "Done Story 2", traceWorkID: "work-done-story-2" },
          ]}
          failedItems={[
            { label: "Failed Story 2", traceWorkID: "work-failed-story-2" },
          ]}
          onSelectItem={vi.fn()}
          widgetId="terminal-work::two"
        />
      </>,
    );

    const completedToggles = screen.getAllByRole("button", {
      name: messages.disclosureLabel(true),
    });
    expect(completedToggles).toHaveLength(4);

    expect(completedToggles[0]?.getAttribute("aria-controls")).toBe(
      "terminal-work::one-completed-items",
    );
    expect(completedToggles[2]?.getAttribute("aria-controls")).toBe(
      "terminal-work::two-completed-items",
    );

    const firstCompletedRegion = document.getElementById(
      "terminal-work::one-completed-items",
    );
    const secondCompletedRegion = document.getElementById(
      "terminal-work::two-completed-items",
    );
    expect(firstCompletedRegion).toBeTruthy();
    expect(secondCompletedRegion).toBeTruthy();
    expect(firstCompletedRegion?.id).not.toBe(secondCompletedRegion?.id);

    const firstCompletedHeading = screen.getAllByRole("heading", {
      level: 4,
      name: messages.rowTitle("completed"),
    })[0];
    const secondCompletedHeading = screen.getAllByRole("heading", {
      level: 4,
      name: messages.rowTitle("completed"),
    })[1];
    expect(firstCompletedHeading?.id).toBe(
      "terminal-work::one-completed-items-heading",
    );
    expect(secondCompletedHeading?.id).toBe(
      "terminal-work::two-completed-items-heading",
    );
    expect(firstCompletedHeading?.id).not.toBe(secondCompletedHeading?.id);
  });

  it("marks the selected outcome item through the shared button state", () => {
    render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[
          { label: "Failed Story", traceWorkID: "work-failed-story" },
        ]}
        onSelectItem={vi.fn()}
        selectedItem={{ label: "Failed Story", status: "failed" }}
      />,
    );

    expect(
      screen
        .getByRole("button", { name: /Failed Story/ })
        .getAttribute("data-selected"),
    ).toBe("true");
    expect(
      screen.getByRole("button", { name: /Failed Story/ }).className,
    ).toContain("text-af-text");
    expect(
      screen.getByRole("button", { name: /Failed Story/ }).className,
    ).toContain("border-af-accent-border");
    expect(
      screen
        .getByRole("button", { name: /Done Story/ })
        .getAttribute("data-selected"),
    ).toBeNull();
  });

  it("keeps selection styling controlled by props across rerenders", () => {
    const { rerender } = render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[
          { label: "Failed Story", traceWorkID: "work-failed-story" },
        ]}
        onSelectItem={vi.fn()}
        selectedItem={{ label: "Done Story", status: "completed" }}
      />,
    );

    expect(
      screen
        .getByRole("button", { name: /Done Story/ })
        .getAttribute("data-selected"),
    ).toBe("true");

    rerender(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[
          { label: "Failed Story", traceWorkID: "work-failed-story" },
        ]}
        onSelectItem={vi.fn()}
        selectedItem={{ label: "Failed Story", status: "failed" }}
      />,
    );

    expect(
      screen
        .getByRole("button", { name: /Done Story/ })
        .getAttribute("data-selected"),
    ).toBeNull();
    expect(
      screen
        .getByRole("button", { name: /Failed Story/ })
        .getAttribute("data-selected"),
    ).toBe("true");
  });

  it("renders explicit empty messages for each outcome row", () => {
    const messages = getTerminalWorkMessages("en");

    render(
      <CompletedFailedWorkstationCard
        completedItems={[]}
        failedItems={[]}
        onSelectItem={vi.fn()}
      />,
    );

    expect(screen.getByText(messages.emptyState("completed"))).toBeTruthy();
    expect(screen.getByText(messages.emptyState("failed"))).toBeTruthy();
  });

  it("falls back to default English copy when given an unsupported locale", () => {
    const messages = getTerminalWorkMessages("en");

    render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[]}
        locale="fr"
        onSelectItem={vi.fn()}
      />,
    );

    const terminalWork = screen.getByLabelText(messages.cardTitle);
    const completedRow = screen
      .getByRole("heading", { name: messages.rowTitle("completed") })
      .closest("section");
    if (!completedRow) {
      throw new Error("expected completed row");
    }
    expect(screen.getByText(messages.cardTitle)).toBeTruthy();
    expect(
      within(terminalWork).getByRole("heading", {
        name: messages.rowTitle("completed"),
      }),
    ).toBeTruthy();
    expect(
      within(terminalWork).getByRole("img", {
        name: messages.iconLabel("completed"),
      }),
    ).toBeTruthy();
    expect(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(true),
      }),
    ).toBeTruthy();
    expect(
      within(terminalWork).getByText(
        messages.sessionSummaryFallback("completed"),
      ),
    ).toBeTruthy();
    expect(
      within(terminalWork).getByText(messages.emptyState("failed")),
    ).toBeTruthy();
  });

  it("renders translated terminal-work copy for non-default locales", () => {
    const messages = getTerminalWorkMessages("ja");

    render(
      <CompletedFailedWorkstationCard
        completedItems={[
          { label: "Done Story", traceWorkID: "work-done-story" },
        ]}
        failedItems={[]}
        locale="ja"
        onSelectItem={vi.fn()}
      />,
    );

    const terminalWork = screen.getByLabelText(messages.cardTitle);
    expect(screen.getByText(messages.cardTitle)).toBeTruthy();
    expect(
      within(terminalWork).getByRole("heading", {
        name: messages.rowTitle("completed"),
      }),
    ).toBeTruthy();
    const completedRow = screen
      .getByRole("heading", { name: messages.rowTitle("completed") })
      .closest("section");
    if (!completedRow) {
      throw new Error("expected completed row");
    }
    expect(
      within(terminalWork).getByRole("img", {
        name: messages.iconLabel("completed"),
      }),
    ).toBeTruthy();
    expect(
      within(terminalWork).getByText(
        messages.sessionSummaryFallback("completed"),
      ),
    ).toBeTruthy();
    expect(
      within(terminalWork).getByText(messages.emptyState("failed")),
    ).toBeTruthy();

    fireEvent.click(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(true),
      }),
    );

    expect(
      within(completedRow)
        .getByRole("button", {
          name: messages.disclosureLabel(false),
        })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });
});
