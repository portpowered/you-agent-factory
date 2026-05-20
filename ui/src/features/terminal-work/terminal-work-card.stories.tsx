import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import type { DashboardProviderSessionAttempt } from "../../api/dashboard/types";
import { getTerminalWorkMessages } from "./messages";
import type {
  TerminalWorkItem,
  TerminalWorkStatus,
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

function SelectableTerminalWorkStory() {
  const [selectedItem, setSelectedItem] = useState<{
    label: string;
    status: TerminalWorkStatus;
  } | null>({ label: "Failed Story", status: "failed" });

  return (
    <CompletedFailedWorkstationCard
      completedItems={completedItems}
      failedItems={failedItems}
      onSelectItem={(status, item) =>
        setSelectedItem({ label: item.label, status })
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
  render: () => <SelectableTerminalWorkStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getTerminalWorkMessages("en");
    const terminalWork = await canvas.findByLabelText(messages.cardTitle);
    const terminalScope = within(terminalWork);

    await expect(
      await terminalScope.findByRole("button", { name: "Failed Story" }),
    ).toBeVisible();

    const completedToggle = (
      await terminalScope.findAllByRole("button", {
        name: messages.disclosureLabel(true),
      })
    )[0];
    await userEvent.click(completedToggle);
    await expect(completedToggle).toHaveAttribute("aria-expanded", "false");
    expect(
      terminalScope.queryByRole("button", { name: "Done Story" }),
    ).toBeNull();

    await userEvent.click(completedToggle);
    const doneStory = await terminalScope.findByRole("button", {
      name: "Done Story",
    });
    await expect(doneStory).toBeVisible();
    await userEvent.click(doneStory);
    await expect(doneStory).toHaveAttribute("data-selected", "true");
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
