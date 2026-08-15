import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import type { DashboardProviderSessionAttempt } from "../../../api/dashboard/types";
import { getTerminalWorkMessages } from "../messages/terminal-work";
import type {
  TerminalWorkItem,
  TerminalWorkSelection,
} from "./terminal-work-card";
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

const completedAttempt: DashboardProviderSessionAttempt = {
  dispatch_id: "dispatch-complete",
  outcome: "ACCEPTED",
  provider_session: {
    id: "sess-done-story",
    kind: "session_id",
    provider: "codex",
  },
  transition_id: "complete",
  workstation_name: "Complete",
  work_items: [{ display_name: "Done Story", work_id: "work-done-story" }],
};

const completedItems: TerminalWorkItem[] = [
  {
    attempts: [completedAttempt],
    label: "Done Story",
    traceWorkID: "work-done-story",
  },
  { label: "Release Notes", traceWorkID: "work-release-notes" },
];

const failedItems: TerminalWorkItem[] = [
  {
    attempts: [failedAttempt],
    label: "Failed Story",
    traceWorkID: "work-failed-story",
  },
];

const ACCENT_SELECTED_TOKENS = [
  "bg-primary",
  "bg-primary-container",
  "border-primary",
  "shadow-af-accent-selected",
] as const;

function expectNoAccentSelectedTreatment(className: string) {
  for (const token of ACCENT_SELECTED_TOKENS) {
    expect(className).not.toContain(token);
  }
}

function SelectableTerminalWorkStory() {
  const [selectedItem, setSelectedItem] =
    useState<TerminalWorkSelection | null>({
      status: "failed",
      traceWorkID: "work-failed-story",
    });

  return (
    <CompletedFailedWorkstationCard
      completedItems={completedItems}
      failedItems={failedItems}
      onSelectItem={(status, item) =>
        setSelectedItem({
          dispatchID: item.dispatchID,
          status,
          traceWorkID: item.traceWorkID,
          workItem: item.workItem,
        })
      }
      selectedItem={selectedItem}
      widgetId="terminal-work-story"
    />
  );
}

function LocalizedTerminalWorkStory({ locale }: { locale: string }) {
  const messages = getTerminalWorkMessages(locale);

  return (
    <CompletedFailedWorkstationCard
      completedItems={[{ label: "Done Story", traceWorkID: "work-done-story" }]}
      failedItems={[]}
      locale={locale}
      onSelectItem={() => {}}
      widgetId={`terminal-work-${locale}-story`}
      title={messages.cardTitle}
    />
  );
}

export default {
  title: "Agent Factory/Dashboard/Completed Failed Workstation Card",
  component: CompletedFailedWorkstationCard,
};

export const MixedOutcomes = {
  tags: ["test"],
  render: () => <SelectableTerminalWorkStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getTerminalWorkMessages("en");
    const terminalWork = await canvas.findByLabelText(messages.cardTitle);
    const terminalScope = within(terminalWork);

    await expect(
      await terminalScope.findByRole("button", {
        name: messages.selectWorkItemLabel("Failed Story"),
      }),
    ).toBeVisible();

    const completedToggle = (
      await terminalScope.findAllByRole("button", {
        name: messages.disclosureLabel(true),
      })
    )[0];
    await userEvent.click(completedToggle);
    await expect(completedToggle).toHaveAttribute("aria-expanded", "false");
    expect(
      terminalScope.queryByRole("button", {
        name: messages.selectWorkItemLabel("Done Story"),
      }),
    ).toBeNull();

    await userEvent.click(completedToggle);
    const doneStory = await terminalScope.findByRole("button", {
      name: messages.selectWorkItemLabel("Done Story"),
    });
    await expect(doneStory).toBeVisible();
    doneStory.focus();
    await userEvent.keyboard("{Enter}");
    await expect(doneStory).toHaveAttribute("aria-pressed", "true");
    await expect(doneStory).toHaveTextContent(messages.selectedWorkItemAction);
    expectNoAccentSelectedTreatment(doneStory.className);

    const failedStory = await terminalScope.findByRole("button", {
      name: messages.selectWorkItemLabel("Failed Story"),
    });
    await expect(failedStory).toHaveAttribute("aria-pressed", "false");
    expectNoAccentSelectedTreatment(failedStory.className);
  },
};

export const LocalizedJapanese = {
  render: () => <LocalizedTerminalWorkStory locale="ja" />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getTerminalWorkMessages("ja");
    const terminalWork = await canvas.findByLabelText(messages.cardTitle);
    const terminalScope = within(terminalWork);
    const completedRow = (
      await terminalScope.findByRole("heading", {
        name: messages.rowTitle("completed"),
      })
    ).closest("section");

    await expect(canvas.getByText(messages.cardTitle)).toBeVisible();
    await expect(
      terminalScope.getByRole("img", { name: messages.iconLabel("completed") }),
    ).toBeVisible();
    await expect(
      terminalScope.getByText(messages.sessionSummaryFallback("completed")),
    ).toBeVisible();
    await expect(
      terminalScope.getByText(messages.emptyState("failed")),
    ).toBeVisible();

    if (!completedRow) {
      throw new Error("expected completed row");
    }

    await userEvent.click(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(true),
      }),
    );
    await expect(
      within(completedRow).getByRole("button", {
        name: messages.disclosureLabel(false),
      }),
    ).toHaveAttribute("aria-expanded", "false");
  },
};

export const CompletedOnly = {
  args: {
    completedItems: completedItems.slice(0, 1),
    failedItems: [],
    onSelectItem: () => {},
    widgetId: "terminal-work-completed-story",
  },
};

export const FailedOnly = {
  args: {
    completedItems: [],
    failedItems,
    onSelectItem: () => {},
    widgetId: "terminal-work-failed-story",
  },
};

export const Empty = {
  args: {
    completedItems: [],
    failedItems: [],
    onSelectItem: () => {},
    widgetId: "terminal-work-empty-story",
  },
};
